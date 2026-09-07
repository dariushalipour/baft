import { spawn } from "child_process";
import * as vscode from "vscode";
import {
  CompatibilityReport,
  RestyleColorPalette,
  binaryPath,
  isScanned,
  runCheck,
  runRestyle,
  verifyCompatibility,
} from "./baft";
import { publish } from "./diagnostics";

const DEBOUNCE_MS = 750;
const BAFT_DOCUMENT_SELECTOR: vscode.DocumentSelector = [
  { scheme: "file", pattern: "**/BAFT.md" },
];

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Baft");
  context.subscriptions.push(output);
  const pluginVersion = String(context.extension.packageJSON.version ?? "0.0.1");
  const integrationId = vscode.env.appName.includes("Insiders")
    ? "vscode-insiders"
    : "vscode";

  const collections = new Map<string, vscode.DiagnosticCollection>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const runs = new Map<string, number>();
  let compatible = false;
  let pendingCompatibility: Promise<boolean> | undefined;
  let lastError = "";

  // Every failure path funnels through here so a repeated failure is reported once.
  function notifyFailure(message: string): void {
    if (message === lastError) return;
    lastError = message;
    output.appendLine(`Baft: ${message}`);
    vscode.window.showErrorMessage(message);
  }

  function ensureCompatibility(): Promise<boolean> {
    if (compatible) return Promise.resolve(true);
    if (!pendingCompatibility) {
      pendingCompatibility = checkCompatibility().finally(() => {
        pendingCompatibility = undefined;
      });
    }
    return pendingCompatibility;
  }

  async function checkCompatibility(): Promise<boolean> {
    try {
      const report = await verifyCompatibility(integrationId, pluginVersion);
      if (report.compatible) {
        compatible = true;
        lastError = "";
        return true;
      }
      if (report.code === "version_mismatch") {
        showVersionMismatch(report);
      } else {
        notifyFailure(report.message || "Baft compatibility check failed");
      }
    } catch (err: unknown) {
      notifyFailure(failureMessage(err));
    }
    return false;
  }

  function showVersionMismatch(mismatch: CompatibilityReport): void {
    const detail = mismatch.expected_version
      ? `Installed: ${mismatch.plugin_version}, Expected: ${mismatch.expected_version}`
      : mismatch.message;
    const message = `Baft plugin version mismatch\n${detail}`;
    if (message === lastError) return;
    lastError = message;
    output.appendLine(`Baft: ${message}`);
    vscode.window
      .showErrorMessage(message, "Reinstall")
      .then((selection) => {
        if (selection === "Reinstall") {
          runReinstall();
        }
      });
  }

  function runReinstall(): void {
    const proc = spawn(
      binaryPath(),
      ["integrate", `--integration=${integrationId}`, "--yes"],
      { stdio: ["ignore", "pipe", "pipe"] }
    );

    let stderr = "";
    proc.stderr?.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });

    proc.on("close", (code: number) => {
      if (code !== 0) {
        vscode.window.showErrorMessage(
          `Baft reinstall failed: ${stderr.trim() || "Reinstall failed"}`
        );
        return;
      }
      vscode.window
        .showInformationMessage(
          "Baft extension reinstalled successfully. Please reload VS Code to activate it.",
          "Reload Window"
        )
        .then((selection) => {
          if (selection === "Reload Window") {
            vscode.commands.executeCommand("workbench.action.reloadWindow");
          }
        });
    });

    proc.on("error", (err: unknown) => {
      vscode.window.showErrorMessage(failureMessage(err));
    });
  }

  function getCollection(root: string): vscode.DiagnosticCollection {
    let c = collections.get(root);
    if (!c) {
      c = vscode.languages.createDiagnosticCollection(`baft:${root}`);
      collections.set(root, c);
      context.subscriptions.push(c);
    }
    return c;
  }

  // One check per workspace root; diagnostics for every file come from that run.
  // A failed run keeps the diagnostics of the last good one instead of clearing them.
  async function checkFolder(root: string): Promise<void> {
    const runId = (runs.get(root) ?? 0) + 1;
    runs.set(root, runId);

    try {
      if (!(await ensureCompatibility())) {
        return;
      }
      const { violations, errors } = await runCheck(root, output);
      if (runs.get(root) !== runId) return;
      if (errors.length > 0) {
        notifyFailure(errors.join("\n"));
        return;
      }
      lastError = "";
      publish(getCollection(root), violations);
    } catch (err: unknown) {
      if (runs.get(root) !== runId) return;
      notifyFailure(failureMessage(err));
    }
  }

  function scheduleCheck(root: string): void {
    const t = timers.get(root);
    if (t !== undefined) clearTimeout(t);
    timers.set(
      root,
      setTimeout(() => {
        timers.delete(root);
        checkFolder(root);
      }, DEBOUNCE_MS)
    );
  }

  function rootToCheck(document: vscode.TextDocument): string | undefined {
    if (!isScanned(document.uri.fsPath)) return undefined;
    return vscode.workspace.getWorkspaceFolder(document.uri)?.uri.fsPath;
  }

  function formatPaletteFor(document: vscode.TextDocument): RestyleColorPalette {
    return vscode.workspace
      .getConfiguration("baft", document.uri)
      .get<RestyleColorPalette>("format.colorPalette", "vibrant");
  }

  function formatOnSaveEnabledFor(document: vscode.TextDocument): boolean {
    return vscode.workspace
      .getConfiguration("baft", document.uri)
      .get<boolean>("format.onSave", false);
  }

  async function provideFormattingEdits(
    document: vscode.TextDocument
  ): Promise<vscode.TextEdit[]> {
    try {
      if (!(await ensureCompatibility())) {
        return [];
      }
      const restyled = await runRestyle(
        document.uri.fsPath,
        document.getText(),
        formatPaletteFor(document),
        output
      );
      if (restyled === document.getText()) {
        return [];
      }

      const fullRange = new vscode.Range(
        document.positionAt(0),
        document.positionAt(document.getText().length)
      );
      return [vscode.TextEdit.replace(fullRange, restyled)];
    } catch (err: unknown) {
      notifyFailure(failureMessage(err));
      return [];
    }
  }

  async function restyleDocument(
    document: vscode.TextDocument
  ): Promise<boolean> {
    const edits = await provideFormattingEdits(document);
    if (edits.length === 0) {
      return false;
    }

    const workspaceEdit = new vscode.WorkspaceEdit();
    workspaceEdit.set(document.uri, edits);
    return vscode.workspace.applyEdit(workspaceEdit);
  }

  context.subscriptions.push(
    vscode.languages.registerDocumentFormattingEditProvider(
      BAFT_DOCUMENT_SELECTOR,
      { provideDocumentFormattingEdits: provideFormattingEdits }
    ),
    vscode.commands.registerCommand("baft.restyleContract", async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showErrorMessage("Baft: open a contract file to restyle it");
        return;
      }

      if (vscode.languages.match(BAFT_DOCUMENT_SELECTOR, editor.document) === 0) {
        vscode.window.showErrorMessage("Baft: the active editor is not a contract file");
        return;
      }

      const applied = await restyleDocument(editor.document);
      if (!applied) {
        vscode.window.showInformationMessage("Baft: contract is already restyled");
      }
    }),
    vscode.workspace.onWillSaveTextDocument((event) => {
      if (vscode.languages.match(BAFT_DOCUMENT_SELECTOR, event.document) === 0) {
        return;
      }
      if (!formatOnSaveEnabledFor(event.document)) {
        return;
      }
      event.waitUntil(provideFormattingEdits(event.document));
    }),
    vscode.workspace.onDidSaveTextDocument((doc) => {
      const root = rootToCheck(doc);
      if (!root) return;
      const t = timers.get(root);
      if (t !== undefined) {
        clearTimeout(t);
        timers.delete(root);
      }
      checkFolder(root);
    }),
    vscode.workspace.onDidChangeTextDocument((e) => {
      const root = rootToCheck(e.document);
      if (root) scheduleCheck(root);
    }),
    vscode.workspace.onDidCloseTextDocument((doc) => {
      const root = rootToCheck(doc);
      if (root) scheduleCheck(root);
    })
  );

  for (const folder of vscode.workspace.workspaceFolders ?? []) {
    checkFolder(folder.uri.fsPath);
  }
}

export function deactivate(): void {}

function failureMessage(err: unknown): string {
  if (
    typeof err === "object" &&
    err !== null &&
    (err as NodeJS.ErrnoException).code === "ENOENT"
  ) {
    return `Baft: '${binaryPath()}' not found. Install the CLI or set baft.binaryPath.`;
  }
  if (err instanceof Error && err.message.trim() !== "") {
    return err.message;
  }
  return "Baft: command failed";
}
