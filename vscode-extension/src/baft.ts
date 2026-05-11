import { spawn, ChildProcess } from "child_process";
import * as vscode from "vscode";

export interface Violation {
  rule: string;
  severity: string;
  source: string;
  message: string;
  file: string;
  line: number;
  column: number;
  columnEnd?: number;
}

interface OverlayFile {
  path: string;
  content: string;
}

interface OverlayPayload {
  files: OverlayFile[];
}

const running = new Map<string, ChildProcess>();

export function runCheck(
  cwd: string,
  output: vscode.OutputChannel
): Promise<Violation[]> {
  const overlay = collectOverlay(cwd);

  running.get(cwd)?.kill();
  running.delete(cwd);

  return new Promise((resolve, reject) => {
    const args = ["check", "--reporter=vsce"];
    if (overlay !== undefined) {
      args.push("--overlay-stdin");
    }
    args.push(".");

    const proc = spawn("baft", args, {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
    });

    running.set(cwd, proc);

    proc.stdin?.end(overlay);

    let stdout = "";

    proc.stdout?.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    proc.stderr?.on("data", (chunk: Buffer) => {
      output.appendLine(chunk.toString().trimEnd());
    });

    proc.on("error", (err: NodeJS.ErrnoException) => {
      running.delete(cwd);
      reject(err);
    });

    proc.on("close", (_code, signal) => {
      running.delete(cwd);
      if (signal !== null) {
        resolve([]);
        return;
      }
      try {
        resolve(JSON.parse(stdout.trim()));
      } catch {
        output.appendLine(`BAFT: failed to parse output:\n${stdout}`);
        resolve([]);
      }
    });
  });
}

function collectOverlay(cwd: string): string | undefined {
  const files = vscode.workspace.textDocuments
    .filter(
      (doc) =>
        doc.isDirty &&
        doc.uri.scheme === "file" &&
        vscode.workspace.getWorkspaceFolder(doc.uri)?.uri.fsPath === cwd
    )
    .map<OverlayFile>((doc) => ({
      path: doc.uri.fsPath,
      content: doc.getText(),
    }))
    .sort((left, right) => left.path.localeCompare(right.path));

  if (files.length === 0) {
    return undefined;
  }

  const payload: OverlayPayload = { files };
  return JSON.stringify(payload);
}
