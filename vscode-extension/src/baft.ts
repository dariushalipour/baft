import { spawn, ChildProcess } from "child_process";
import * as vscode from "vscode";

const PROTOCOL_VERSION = 3;

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

interface CompatibilityReport {
  compatible: boolean;
  message: string;
  warning?: string;
}

export type RestyleColorPalette = "vibrant" | "muted" | "mono" | "none";

const running = new Map<string, ChildProcess>();

export function verifyCompatibility(
  integrationId: string,
  pluginVersion: string,
  output: vscode.OutputChannel
): Promise<void> {
  return new Promise((resolve, reject) => {
    const proc = spawn(
      "baft",
      [
        "integrate",
        "--verify-compatible",
        `--integration=${integrationId}`,
        `--plugin-version=${pluginVersion}`,
        `--protocol=${PROTOCOL_VERSION}`,
      ],
      { stdio: ["ignore", "pipe", "pipe"] }
    );

    let stdout = "";
    let stderr = "";

    proc.stdout?.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    proc.stderr?.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });

    proc.on("error", (err: NodeJS.ErrnoException) => {
      reject(err);
    });

    proc.on("close", (code, signal) => {
      if (signal !== null) {
        reject(new Error("BAFT compatibility check was interrupted"));
        return;
      }

      let report: CompatibilityReport | undefined;
      try {
        report = JSON.parse(stdout.trim()) as CompatibilityReport;
      } catch {
        report = undefined;
      }

      if (report?.warning) {
        output.appendLine(`BAFT: ${report.warning}`);
      }

      if (code === 0 && report?.compatible) {
        resolve();
        return;
      }

      const message = report?.message || stderr.trim() || "BAFT compatibility check failed";
      reject(new Error(message));
    });
  });
}

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

export function runRestyle(
  filePath: string,
  content: string,
  colorPalette: RestyleColorPalette,
  output: vscode.OutputChannel
): Promise<string> {
  return new Promise((resolve, reject) => {
    const proc = spawn(
      "baft",
      [
        "restyle",
        "--stdin",
        `--path=${filePath}`,
        `--color-palette=${colorPalette}`,
      ],
      { stdio: ["pipe", "pipe", "pipe"] }
    );

    proc.stdin?.end(content);

    let stdout = "";
    let stderr = "";

    proc.stdout?.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    proc.stderr?.on("data", (chunk: Buffer) => {
      const text = chunk.toString();
      stderr += text;
      output.appendLine(text.trimEnd());
    });

    proc.on("error", (err: NodeJS.ErrnoException) => {
      reject(err);
    });

    proc.on("close", (code, signal) => {
      if (signal !== null) {
        reject(new Error("BAFT restyle was interrupted"));
        return;
      }
      if (code !== 0) {
        reject(new Error(stderr.trim() || "BAFT restyle failed"));
        return;
      }
      resolve(stdout);
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
