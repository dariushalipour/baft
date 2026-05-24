import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { spawn } from "child_process";

const mockSpawn = vi.fn();
vi.mock("child_process", () => ({
  spawn: (...args: unknown[]) => mockSpawn(...args),
}));

vi.mock("vscode", () => ({
  window: {
    createOutputChannel: () => ({
      appendLine: () => {},
    }),
  },
  workspace: {
    textDocuments: [],
    getWorkspaceFolder: () => undefined,
  },
}));

import { verifyCompatibility, runCheck, runRestyle } from "./baft";

function createMockProcess({
  exitCode = 0,
  stdout = "",
  stderr = "",
  signal = null,
}: {
  exitCode?: number;
  stdout?: string;
  stderr?: string;
  signal?: string | null;
} = {}) {
  const EventEmitter = require("events");
  const ee = new EventEmitter();

  ee.stdout = new EventEmitter();
  ee.stderr = new EventEmitter();

  process.nextTick(() => {
    if (stdout) {
      ee.stdout.emit("data", Buffer.from(stdout));
    }
    if (stderr) {
      ee.stderr.emit("data", Buffer.from(stderr));
    }
    ee.emit("close", exitCode, signal);
  });

  return ee;
}

describe("verifyCompatibility", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockSpawn.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("resolves with compatible report when versions match", async () => {
    const report = {
      compatible: true,
      message: "compatible",
      integration_id: "vscode",
      protocol: 3,
    };
    mockSpawn.mockReturnValue(createMockProcess({ stdout: JSON.stringify(report) }));

    const outputChannel = { appendLine: vi.fn() };
    const result = await verifyCompatibility("vscode", "0.2.1", outputChannel as any);

    expect(result).toBeDefined();
    expect(result?.compatible).toBe(true);
    expect(mockSpawn).toHaveBeenCalledWith(
      "baft",
      expect.arrayContaining(["--verify-compatible", "--integration=vscode", "--plugin-version=0.2.1", "--protocol=3"]),
      expect.any(Object)
    );
  });

  it("resolves with incompatible report on version mismatch", async () => {
    const report = {
      compatible: false,
      message: "Baft plugin version mismatch: expected 0.2.1, got 0.1.2",
      expected_version: "0.2.1",
      plugin_version: "0.1.2",
    };
    mockSpawn.mockReturnValue(
      createMockProcess({ exitCode: 1, stdout: JSON.stringify(report) })
    );

    const outputChannel = { appendLine: vi.fn() };
    const result = await verifyCompatibility("vscode", "0.1.2", outputChannel as any);

    expect(result).toBeDefined();
    expect(result?.compatible).toBe(false);
    expect(result?.expected_version).toBe("0.2.1");
    expect(result?.plugin_version).toBe("0.1.2");
    expect(result?.message).toContain("version mismatch");
  });

  it("resolves with report containing warning", async () => {
    const report = {
      compatible: true,
      message: "compatible",
      warning: "CLI version is a development build; semantic version checks were skipped",
    };
    mockSpawn.mockReturnValue(createMockProcess({ stdout: JSON.stringify(report) }));

    const appendLine = vi.fn();
    const outputChannel = { appendLine };
    const result = await verifyCompatibility("vscode", "0.2.1", outputChannel as any);

    expect(result?.compatible).toBe(true);
    expect(appendLine).toHaveBeenCalledWith("Baft: CLI version is a development build; semantic version checks were skipped");
  });

  it("rejects when spawn fails with ENOENT", async () => {
    const EventEmitter = require("events");
    const ee = new EventEmitter();
    process.nextTick(() => {
      ee.emit("error", { code: "ENOENT", message: "baft not found" });
    });
    mockSpawn.mockReturnValue(ee);

    const outputChannel = { appendLine: vi.fn() };
    await expect(
      verifyCompatibility("vscode", "0.2.1", outputChannel as any)
    ).rejects.toThrow();
  });

  it("rejects when process is killed by signal", async () => {
    const EventEmitter = require("events");
    const ee = new EventEmitter();
    process.nextTick(() => {
      ee.emit("close", null, "SIGKILL");
    });
    ee.stdout = new EventEmitter();
    ee.stderr = new EventEmitter();
    mockSpawn.mockReturnValue(ee);

    const outputChannel = { appendLine: vi.fn() };
    await expect(
      verifyCompatibility("vscode", "0.2.1", outputChannel as any)
    ).rejects.toThrow("Baft compatibility check was interrupted");
  });

  it("resolves with fallback message when JSON is invalid", async () => {
    mockSpawn.mockReturnValue(
      createMockProcess({ exitCode: 1, stderr: "some error output" })
    );

    const outputChannel = { appendLine: vi.fn() };
    const result = await verifyCompatibility("vscode", "0.2.1", outputChannel as any);

    expect(result).toBeUndefined();
  });
});

describe("runCheck", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockSpawn.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns parsed violations array", async () => {
    const violations = [
      {
        rule: "capsule-purity",
        severity: "error",
        source: "src/foo.go",
        message: "violation",
        file: "src/foo.go",
        line: 10,
        column: 5,
      },
    ];
    const mockProc = createMockProcess({ stdout: JSON.stringify(violations) });
    mockProc.stdin = { end: vi.fn() };
    mockSpawn.mockReturnValue(mockProc);

    const outputChannel = { appendLine: vi.fn() };
    const result = await runCheck("/project/root", outputChannel as any);

    expect(result).toEqual(violations);
    expect(mockSpawn).toHaveBeenCalledWith(
      "baft",
      ["check", "--reporter=vsce", "."],
      expect.objectContaining({ cwd: "/project/root" })
    );
  });

  it("returns empty array when output is not valid JSON", async () => {
    const appendLine = vi.fn();
    const outputChannel = { appendLine };
    const mockProc = createMockProcess({ stdout: "not json" });
    mockProc.stdin = { end: vi.fn() };
    mockSpawn.mockReturnValue(mockProc);

    const result = await runCheck("/project/root", outputChannel as any);

    expect(result).toEqual([]);
    expect(appendLine).toHaveBeenCalled();
  });

  it("rejects when spawn fails", async () => {
    const EventEmitter = require("events");
    const ee = new EventEmitter();
    process.nextTick(() => {
      ee.emit("error", { code: "ENOENT" });
    });
    mockSpawn.mockReturnValue(ee);

    const outputChannel = { appendLine: vi.fn() };
    await expect(
      runCheck("/project/root", outputChannel as any)
    ).rejects.toThrow();
  });
});

describe("runRestyle", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockSpawn.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns restyled content on success", async () => {
    const restyled = "# Restyled contract content";
    const mockProc = createMockProcess({ stdout: restyled });
    mockProc.stdin = { end: vi.fn() };
    mockSpawn.mockReturnValue(mockProc);

    const outputChannel = { appendLine: vi.fn() };
    const result = await runRestyle(
      "BAFT.md",
      "# Original",
      "vibrant",
      outputChannel as any
    );

    expect(result).toBe(restyled);
    expect(mockSpawn).toHaveBeenCalledWith(
      "baft",
      ["restyle", "--stdin", "--path=BAFT.md", "--color-palette=vibrant"],
      expect.any(Object)
    );
  });

  it("rejects when restyle exits with non-zero code", async () => {
    const mockProc = createMockProcess({ exitCode: 1, stderr: "restyle error" });
    mockProc.stdin = { end: vi.fn() };
    mockSpawn.mockReturnValue(mockProc);

    const outputChannel = { appendLine: vi.fn() };
    await expect(
      runRestyle("BAFT.md", "# Original", "vibrant", outputChannel as any)
    ).rejects.toThrow("restyle error");
  });

  it("rejects when process is killed by signal", async () => {
    const EventEmitter = require("events");
    const ee = new EventEmitter();
    ee.stdin = { end: vi.fn() };
    process.nextTick(() => {
      ee.emit("close", null, "SIGTERM");
    });
    mockSpawn.mockReturnValue(ee);

    const outputChannel = { appendLine: vi.fn() };
    await expect(
      runRestyle("BAFT.md", "# Original", "vibrant", outputChannel as any)
    ).rejects.toThrow("Baft restyle was interrupted");
  });
});
