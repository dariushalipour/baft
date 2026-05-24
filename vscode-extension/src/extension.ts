import * as vscode from "vscode";
import {
  RestyleColorPalette,
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

  const collections = new Map<string, vscode.DiagnosticCollection>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const runs = new Map<string, number>();
  let compatibilityVerified = false;
  let lastCompatibilityError = "";

  async function ensureCompatibility(): Promise<boolean> {
    if (compatibilityVerified) {
      return true;
    }

    try {
      const report = await verifyCompatibility("vscode", pluginVersion, output);
      if (report?.compatible) {
        compatibilityVerified = true;
        lastCompatibilityError = "";
        return true;
      }

      const message = report?.message || "Baft compatibility check failed";
      if (message.includes("version mismatch") && report) {
        handleVersionMismatch(message, report, output);
      } else if (message !== lastCompatibilityError) {
        lastCompatibilityError = message;
        output.appendLine(`Baft: ${message}`);
        vscode.window.showErrorMessage(message);
      }
      return false;
    } catch (err: unknown) {
      const message = errorMessage(err);
      if (message !== lastCompatibilityError) {
        lastCompatibilityError = message;
        output.appendLine(`Baft: ${message}`);
        vscode.window.showErrorMessage(message);
      }
      return false;
    }
  }

  function handleVersionMismatch(
    message: string,
    report: { expected_version?: string; plugin_version?: string },
    _output: vscode.OutputChannel
  ): void {
    const expected = report.expected_version;
    const detail = expected
      ? `Installed: ${report.plugin_version}, Expected: ${expected}`
      : message;

    vscode.window
      .showErrorMessage(`Baft plugin version mismatch\n${detail}`, "Reinstall")
      .then((selection) => {
        if (selection === "Reinstall") {
          runReinstall();
        }
      });
  }

  function runReinstall(): void {
    const { spawn } = require("child_process");
    const proc = spawn("baft", ["integrate", "--integration=vscode", "--yes"], {
      stdio: ["ignore", "pipe", "pipe"],
    });

    let stdout = "";
    let stderr = "";

    proc.stdout?.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    proc.stderr?.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });

    proc.on("close", (code: number) => {
      if (code === 0) {
        vscode.window.showInformationMessage(
          "Baft extension reinstalled successfully. Please reload VS Code to activate it.",
          "Reload Window"
        ).then((selection) => {
          if (selection === "Reload Window") {
            vscode.commands.executeCommand("workbench.action.reloadWindow");
          }
        });
      } else {
        const error = stderr.trim() || "Reinstall failed";
        vscode.window.showErrorMessage(
          `Baft reinstall failed: ${error}`
        );
      }
    });

    proc.on("error", () => {
      vscode.window.showErrorMessage(
        "Baft: Could not run reinstall. Make sure 'baft' is in your PATH."
      );
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

  async function checkFolder(root: string): Promise<void> {
    const runId = (runs.get(root) ?? 0) + 1;
    runs.set(root, runId);

    try {
      if (!(await ensureCompatibility())) {
        return;
      }
      const violations = await runCheck(root, output);
      if (runs.get(root) !== runId) return;
      publish(getCollection(root), violations);
    } catch (err: unknown) {
      if (runs.get(root) !== runId) return;
      if (isEnoent(err)) {
        vscode.window.showErrorMessage("Baft: binary not found in PATH");
      }
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

  function rootOf(uri: vscode.Uri): string | undefined {
    return vscode.workspace.getWorkspaceFolder(uri)?.uri.fsPath;
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
      if (isEnoent(err)) {
        vscode.window.showErrorMessage("Baft: binary not found in PATH");
        return [];
      }
      const message = errorMessage(err);
      output.appendLine(`Baft: ${message}`);
      vscode.window.showErrorMessage(message);
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
      const root = rootOf(doc.uri);
      if (!root) return;
      const t = timers.get(root);
      if (t !== undefined) {
        clearTimeout(t);
        timers.delete(root);
      }
      checkFolder(root);
    }),
    vscode.workspace.onDidChangeTextDocument((e) => {
      const root = rootOf(e.document.uri);
      if (root) scheduleCheck(root);
    }),
    vscode.workspace.onDidCloseTextDocument((doc) => {
      const root = rootOf(doc.uri);
      if (root) scheduleCheck(root);
    })
  );

  for (const folder of vscode.workspace.workspaceFolders ?? []) {
    checkFolder(folder.uri.fsPath);
  }
}

export function deactivate(): void {}

function isEnoent(err: unknown): boolean {
  return (
    typeof err === "object" &&
    err !== null &&
    (err as NodeJS.ErrnoException).code === "ENOENT"
  );
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim() !== "") {
    return err.message;
  }
  return "Baft compatibility check failed";
}
