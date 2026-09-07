import { spawn, ChildProcess } from "child_process";
import * as vscode from "vscode";

const PROTOCOL_VERSION = 4;

// Everything that can change a check: the sources baft scans, the contract,
// the capsule manifests that delimit them, and the files that decide what is
// scanned at all. Keep in sync with isScannedByBaft in the IntelliJ plugin.
const SCANNED =
  /(\.(go|ts|tsx|py|pyi|rs|java|kt|cs|csproj|dart)|[\\/](BAFT\.md|go\.mod|package\.json|pom\.xml|build\.gradle(\.kts)?|Cargo\.toml|pyproject\.toml|setup\.py|pubspec\.yaml|tsconfig[^\\/]*\.json|\.gitignore|\.baftignore))$/;

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

export interface CheckOutput {
  violations: Violation[];
  errors: string[];
}

export interface CompatibilityReport {
  compatible: boolean;
  code?: string;
  message: string;
  expected_version?: string;
  plugin_version?: string;
}

interface OverlayFile {
  path: string;
  content: string;
}

export type RestyleColorPalette = "vibrant" | "muted" | "mono" | "none";

const running = new Map<string, ChildProcess>();

export function isScanned(fsPath: string): boolean {
  return SCANNED.test(fsPath);
}

// Machine-scoped setting, so a workspace can never point Baft at a binary it ships.
export function binaryPath(): string {
  const configured = vscode.workspace
    .getConfiguration("baft")
    .get<string>("binaryPath", "")
    .trim();
  return configured || "baft";
}

export function verifyCompatibility(
  integrationId: string,
  pluginVersion: string
): Promise<CompatibilityReport> {
  return new Promise((resolve, reject) => {
    const proc = spawn(
      binaryPath(),
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

    proc.on("close", (_code, signal) => {
      if (signal !== null) {
        reject(new Error("Baft compatibility check was interrupted"));
        return;
      }
      try {
        resolve(JSON.parse(stdout.trim()) as CompatibilityReport);
      } catch {
        reject(new Error(stderr.trim() || "Baft compatibility check failed"));
      }
    });
  });
}

export function runCheck(
  cwd: string,
  output: vscode.OutputChannel
): Promise<CheckOutput> {
  const overlay = collectOverlay(cwd);

  running.get(cwd)?.kill();
  running.delete(cwd);

  return new Promise((resolve, reject) => {
    const args = ["check", "--reporter=vsce"];
    if (overlay !== undefined) {
      args.push("--overlay-stdin");
    }
    args.push(".");

    const proc = spawn(binaryPath(), args, {
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
        reject(new Error("Baft check was interrupted"));
        return;
      }
      try {
        const parsed = JSON.parse(stdout.trim()) as Partial<CheckOutput>;
        resolve({
          violations: parsed.violations ?? [],
          errors: parsed.errors ?? [],
        });
      } catch {
        output.appendLine(`Baft: failed to parse output:\n${stdout}`);
        reject(new Error("Baft: could not parse check output"));
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
      binaryPath(),
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
        reject(new Error("Baft restyle was interrupted"));
        return;
      }
      if (code !== 0) {
        reject(new Error(stderr.trim() || "Baft restyle failed"));
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
        isScanned(doc.uri.fsPath) &&
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

  return JSON.stringify({ files });
}
