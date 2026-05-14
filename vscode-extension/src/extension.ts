import * as vscode from "vscode";
import { runCheck, verifyCompatibility } from "./baft";
import { publish } from "./diagnostics";

const DEBOUNCE_MS = 750;

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("BAFT");
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
      await verifyCompatibility("vscode", pluginVersion, output);
      compatibilityVerified = true;
      lastCompatibilityError = "";
      return true;
    } catch (err: unknown) {
      const message = errorMessage(err);
      if (message !== lastCompatibilityError) {
        lastCompatibilityError = message;
        output.appendLine(`BAFT: ${message}`);
        vscode.window.showErrorMessage(message);
      }
      return false;
    }
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
        vscode.window.showErrorMessage("BAFT: binary not found in PATH");
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

  context.subscriptions.push(
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
  return "BAFT compatibility check failed";
}
