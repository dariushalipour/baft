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

const running = new Map<string, ChildProcess>();

export function runCheck(
  cwd: string,
  output: vscode.OutputChannel
): Promise<Violation[]> {
  running.get(cwd)?.kill();
  running.delete(cwd);

  return new Promise((resolve, reject) => {
    const proc = spawn("strata", ["check", "--reporter=vsce", "."], {
      cwd,
      stdio: ["ignore", "pipe", "pipe"],
    });

    running.set(cwd, proc);

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
        output.appendLine(`STRATA: failed to parse output:\n${stdout}`);
        resolve([]);
      }
    });
  });
}
