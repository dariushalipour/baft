import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const runCheck = vi.fn();
const verifyCompatibility = vi.fn();
const publish = vi.fn();
const showErrorMessage = vi.fn(() => Promise.resolve(undefined));
const handlers: Record<string, (arg: unknown) => void> = {};
const register = (name: string, fn: (arg: unknown) => void) => {
  handlers[name] = fn;
  return { dispose: () => {} };
};

vi.mock("./baft", () => ({
  binaryPath: () => "baft",
  isScanned: () => true,
  runCheck: (...args: unknown[]) => runCheck(...args),
  runRestyle: vi.fn(),
  verifyCompatibility: (...args: unknown[]) => verifyCompatibility(...args),
}));

vi.mock("./diagnostics", () => ({
  publish: (...args: unknown[]) => publish(...args),
}));

vi.mock("vscode", () => ({
  window: {
    createOutputChannel: () => ({ appendLine: () => {}, dispose: () => {} }),
    showErrorMessage: (...args: unknown[]) => showErrorMessage(...(args as [])),
    showInformationMessage: () => Promise.resolve(undefined),
    activeTextEditor: undefined,
  },
  languages: {
    createDiagnosticCollection: () => ({ clear: () => {}, set: () => {}, dispose: () => {} }),
    registerDocumentFormattingEditProvider: () => ({ dispose: () => {} }),
    match: () => 1,
  },
  commands: {
    registerCommand: () => ({ dispose: () => {} }),
    executeCommand: () => {},
  },
  env: { appName: "Visual Studio Code" },
  workspace: {
    workspaceFolders: [],
    textDocuments: [],
    getWorkspaceFolder: () => ({ uri: { fsPath: "/repo" } }),
    getConfiguration: () => ({ get: (_key: string, fallback: unknown) => fallback }),
    onWillSaveTextDocument: (fn: (arg: unknown) => void) => register("willSave", fn),
    onDidSaveTextDocument: (fn: (arg: unknown) => void) => register("save", fn),
    onDidChangeTextDocument: (fn: (arg: unknown) => void) => register("change", fn),
    onDidCloseTextDocument: (fn: (arg: unknown) => void) => register("close", fn),
    onDidChangeConfiguration: (fn: (arg: unknown) => void) => register("config", fn),
    applyEdit: () => Promise.resolve(true),
  },
}));

import { activate } from "./extension";

const doc = { uri: { fsPath: "/repo/a.go", scheme: "file" }, getText: () => "" };

const compatible = { compatible: true, message: "compatible" };
const mismatch = {
  compatible: false,
  code: "version_mismatch",
  message: "Baft plugin version mismatch",
  expected_version: "0.4.0",
  plugin_version: "0.3.1",
};

function start(): void {
  activate({
    subscriptions: [],
    extension: { packageJSON: { version: "0.3.1" } },
  } as never);
}

async function flush(): Promise<void> {
  for (let i = 0; i < 20; i++) await Promise.resolve();
}

async function save(): Promise<void> {
  handlers.save(doc);
  await flush();
}

describe("activate", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    runCheck.mockReset();
    verifyCompatibility.mockReset();
    publish.mockReset();
    showErrorMessage.mockClear();
    verifyCompatibility.mockResolvedValue(compatible);
    runCheck.mockResolvedValue({ violations: [], errors: [] });
    start();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows the version mismatch toast once, however often the check reruns", async () => {
    verifyCompatibility.mockResolvedValue(mismatch);

    await save();
    await save();

    expect(verifyCompatibility).toHaveBeenCalledTimes(2);
    expect(showErrorMessage).toHaveBeenCalledTimes(1);
    expect(showErrorMessage.mock.calls[0]).toEqual([
      "Baft plugin version mismatch\nInstalled: 0.3.1, Expected: 0.4.0",
      "Reinstall",
    ]);
    expect(runCheck).not.toHaveBeenCalled();
  });

  it("reports a repeated failure once and a new one again", async () => {
    verifyCompatibility.mockRejectedValue(new Error("boom"));
    await save();
    await save();
    expect(showErrorMessage).toHaveBeenCalledTimes(1);

    verifyCompatibility.mockRejectedValue(new Error("other boom"));
    await save();
    expect(showErrorMessage).toHaveBeenCalledTimes(2);
  });

  it("keeps the last good diagnostics when a run aborts", async () => {
    const violations = [{ rule: "no-node", file: "/repo/a.go" }];
    runCheck.mockResolvedValue({ violations, errors: [] });
    await save();
    expect(publish.mock.calls[0][1]).toEqual(violations);

    runCheck.mockResolvedValue({ violations: [], errors: ["discovery: boom"] });
    await save();

    expect(publish).toHaveBeenCalledTimes(1);
    expect(showErrorMessage).toHaveBeenCalledWith("discovery: boom");
  });

  it("re-verifies against a newly configured binary", async () => {
    await save();
    expect(verifyCompatibility).toHaveBeenCalledTimes(1);

    handlers.config({ affectsConfiguration: (key: string) => key === "baft.binaryPath" });
    await flush();
    await save();

    expect(verifyCompatibility).toHaveBeenCalledTimes(2);
  });

  it("runs one check per burst of edits", async () => {
    handlers.change({ document: doc });
    handlers.change({ document: doc });
    handlers.change({ document: doc });
    await vi.advanceTimersByTimeAsync(750);
    await flush();

    expect(runCheck).toHaveBeenCalledTimes(1);
  });

  it("publishes only the newest run when two overlap", async () => {
    const pending: Array<(value: unknown) => void> = [];
    runCheck.mockImplementation(() => new Promise((resolve) => pending.push(resolve)));

    handlers.save(doc);
    handlers.save(doc);
    await flush();
    expect(pending).toHaveLength(2);

    pending[0]({ violations: [{ rule: "stale" }], errors: [] });
    await flush();
    expect(publish).not.toHaveBeenCalled();

    pending[1]({ violations: [{ rule: "fresh" }], errors: [] });
    await flush();
    expect(publish).toHaveBeenCalledTimes(1);
    expect(publish.mock.calls[0][1]).toEqual([{ rule: "fresh" }]);
  });
});
