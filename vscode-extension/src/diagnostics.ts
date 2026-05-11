import * as vscode from "vscode";
import type { Violation } from "./baft";

function toSeverity(s: string): vscode.DiagnosticSeverity {
  switch (s) {
    case "error":
      return vscode.DiagnosticSeverity.Error;
    case "warning":
      return vscode.DiagnosticSeverity.Warning;
    default:
      return vscode.DiagnosticSeverity.Information;
  }
}

export function publish(
  collection: vscode.DiagnosticCollection,
  violations: Violation[]
): void {
  collection.clear();

  const byFile = new Map<string, vscode.Diagnostic[]>();

  for (const v of violations) {
    const line = Math.max(0, v.line - 1);
    const col = Math.max(0, v.column - 1);
    const endCol = v.columnEnd ? v.columnEnd - 1 : Number.MAX_SAFE_INTEGER;
    const range = new vscode.Range(line, col, line, endCol);
    const diag = new vscode.Diagnostic(range, v.message, toSeverity(v.severity));
    diag.source = "baft";
    diag.code = v.rule;

    const existing = byFile.get(v.file) ?? [];
    existing.push(diag);
    byFile.set(v.file, existing);
  }

  for (const [file, diags] of byFile) {
    collection.set(vscode.Uri.file(file), diags);
  }
}
