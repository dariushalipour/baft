# .baftignore

A **.baftignore** file makes paths completely invisible to Baft. It operates at the filesystem layer, below capsule discovery, import parsing, and architecture validation. If a file matches a `.baftignore` pattern, the system treats it as if it does not exist.

---

## The problem

Every project has files that should never be part of an architectural analysis: generated code, build artifacts, temporary files, test fixtures, coverage reports. These files are real on disk but irrelevant to the dependency graph. They may or may not be gitignored — sometimes they need to be tracked in version control (committed snapshots of generated code, vendored dependencies, test data) but still should not be scanned for architecture design.

Without a way to exclude them, Baft would either:

- Scan them unnecessarily (wasting time and producing noise).
- Report their imports as real dependencies (producing false violations).
- Force developers to work around them in `BAFT.md` (polluting the architecture contract with implementation details).

Different ecosystems have different conventions for what to skip. Go uses `vendor/`. TypeScript uses `node_modules/`. Rust uses `target/`. But these are only the broadest categories. Each project has its own granular needs.

---

## Definition

A **.baftignore** file is a gitignore-compatible pattern file that tells Baft which files and directories to treat as invisible. It is not a build configuration, not a linting rule, and not an architecture constraint. It is a visibility filter.

When a file is ignored:

- It is never scanned for imports.
- It never appears as a node in the dependency graph.
- Its imports are treated as external (if encountered during resolution).
- It is never reported as a violation.

---

## What .baftignore is not

A .baftignore is not:

- **A BAFT.md rule.** It does not define nodes, edges, or allowed imports. It removes files from consideration entirely. A file that is `.baftignore`d does not need a node glob and cannot have a violation.
- **A language-specific skip list.** Language adapters provide built-in exclusions (`vendor/`, `node_modules/`, `target/`, etc.) via registration. `.baftignore` is for project-specific exclusions beyond those.
- **A BAFT.md exclusion mechanism.** You do not need to create an "ignored" node in `BAFT.md` and leave it with no edges. `.baftignore` removes the file before the graph is built.
- **A per-command flag.** `.baftignore` applies to all Baft operations — `check`, `draft`, `discover`. There is no way to run Baft while bypassing `.baftignore`.

---

## What .baftignore is

A .baftignore is:

- **A visibility filter.** It wraps the filesystem. When the wrapped filesystem encounters an ignored path, it returns `ErrNotExist` for files and filters them from directory listings. The core never sees them.
- **A gitignore-compatible format.** It uses the same syntax as `.gitignore`: glob patterns, negation with `!`, directory markers with trailing `/`, and hierarchical scoping.
- **A per-directory file.** Like `.gitignore`, `.baftignore` can exist at any directory level. Patterns in a subdirectory apply to that directory and all its children.
- **A layer on top of `.gitignore`.** At each directory level, both `.gitignore` and `.baftignore` are processed together. `.baftignore` is parsed after `.gitignore`, so it takes precedence. A `!` pattern in `.baftignore` can re-include a file that `.gitignore` excluded.

---

## Syntax

`.baftignore` uses standard gitignore syntax:

| Pattern          | Meaning                                                            |
| ---------------- | ------------------------------------------------------------------ |
| `generated/`     | Ignore the `generated` directory and everything inside it          |
| `*.pb.go`        | Ignore all files matching `*.pb.go` at any depth                   |
| `!keep.go`       | Re-include a file that was previously ignored                      |
| `src/generated/` | Ignore only `src/generated/` at this level, not `other/generated/` |
| `**/tmp/`        | Ignore `tmp` directories at any depth                              |

A pattern prefixed with `!` negates (re-includes) a previously ignored path. A trailing `/` matches only directories. A leading `/` anchors the pattern to the directory containing the `.baftignore` file.

---

## Precedence

The precedence hierarchy follows gitignore semantics. Later matches override earlier ones:

1. **Base ignore entries** (lowest priority). Hardcoded directories (`.git`, `.hg`, `.svn`, `.idea`, `.vscode`, `.vs`, `coverage/`, `coverage.lcov`) plus language-specific entries (`vendor/`, `node_modules/`, `target/`, `.dart_tool/`, `.pub/`, `build/`) registered during language setup.
2. **Ancestor patterns**. `.gitignore` and `.baftignore` from every directory between the repo root and the capsule root, processed bottom-up.
3. **Local patterns** (highest priority). `.gitignore` and `.baftignore` from the capsule root and all subdirectories, processed recursively.
4. **`.baftignore` over `.gitignore`**. At the same directory level, `.baftignore` is parsed after `.gitignore`, so its patterns take precedence.

This means a project can use `.gitignore` for general VCS exclusions and `.baftignore` for Baft-specific visibility — or use a single `.baftignore` to handle both, ensuring Baft-specific rules always win.

---

## Hierarchy

`.baftignore` files can exist at any level in the directory tree:

```
my-project/
  .baftignore          ← applies to entire project
  internal/
    .baftignore        ← applies to internal/ and its descendants
    api/
      handler.go       ← tracked
      handler_test.go  ← may be ignored by parent .baftignore
    generated/
      .baftignore      ← applies only to generated/ and its descendants
      code.pb.go       ← ignored by this file
```

A `.baftignore` in a subdirectory applies only to that directory and its children. It does not affect files in sibling directories or parent directories.

---

## Discovery and repo root

When `.baftignore` is loaded, Baft walks upward from the capsule root to find the repository root (a directory containing `.git`). All `.gitignore` and `.baftignore` files between the capsule root and the repo root are loaded as ancestor patterns.

If the repo root is unreachable (e.g., due to filesystem permissions), the wrapper is still created with a warning. Ancestor patterns are simply not loaded — base ignore entries and local patterns still apply.

---

## Interaction with BAFT.md

`.baftignore` and `BAFT.md` operate at different layers:

- `.baftignore` is a **filesystem filter**. It runs before any graph analysis. Ignored files are invisible to everything.
- `BAFT.md` is an **architecture contract**. It runs after file discovery. It defines which files belong to which nodes and which imports are allowed.

A file that is `.baftignore`d never reaches `BAFT.md`. It does not need a node glob, it cannot have a violation, and it cannot appear in any edge.

A file that is NOT `.baftignore`d but also does not match any node in `BAFT.md` is reported as a violation: `... is tracked by BAFT.md but matches no node`.

---

## Interaction with capsule discovery

Capsule discovery walks the filesystem looking for manifest files (`go.mod`, `package.json`, `Cargo.toml`, etc.). The walk respects `.baftignore` (and `.gitignore`). If a directory contains only ignored files, it is never entered.

This means a manifest inside an ignored directory is never discovered. A `go.mod` inside `vendor/` is skipped because `vendor` is a base ignore entry. A `package.json` inside a `.baftignore`d `third-party/` directory is also skipped.

---

## Interaction with import resolution

During import resolution, if a target file is ignored (either by `.gitignore` or `.baftignore`), the import is treated as external. The core receives `internal: false` and does not check it against `BAFT.md` rules.

This handles the common case where a project imports from a vendored or generated dependency that should not be part of the architectural graph.

---

## BAFT.md itself can be ignored

If `BAFT.md` matches a `.gitignore` or `.baftignore` pattern, the filesystem wrapper returns `ErrNotExist` when attempting to read it. The system treats the capsule as having no architecture contract — no violations are reported, and `draft` will create a new `BAFT.md`.

This is an edge case. Normally `BAFT.md` should not be ignored. But the behavior is consistent: ignored files are invisible, including the contract file itself.

---

## Examples

### Excluding generated code

```
# .baftignore
*.pb.go
*.gen.ts
*.freezed.dart
*.g.dart
```

All generated files are invisible to Baft regardless of their location in the tree.

### Excluding a specific directory

```
# .baftignore
third-party/
tools/
```

Entire directories are skipped. No files inside them are scanned, discovered, or reported.

### Re-including a specific file

```
# .baftignore
generated/
!generated/important.go
```

Everything in `generated/` is ignored except `important.go`, which is visible to Baft and must be covered by a node in `BAFT.md`.

### Excluding test fixtures

```
# .baftignore
testdata/
fixtures/
*_test.go
```

Test data directories and test files are invisible to Baft. Only production source files are analyzed.

---

## Why .baftignore and not just .gitignore

Projects already use `.gitignore` for version control. Why maintain a separate `.baftignore`?

1. **Different concerns.** `.gitignore` controls what goes into version control. `.baftignore` controls what Baft analyzes. These are not always the same. A file might be tracked in git but irrelevant to architecture (e.g., generated protobuf files that are committed as a snapshot).
2. **Precedence.** `.baftignore` takes precedence over `.gitignore` at the same level. This allows a project to override git exclusions for Baft's purposes without modifying `.gitignore`.
3. **Clarity.** A dedicated `.baftignore` makes it explicit which files are excluded for architectural analysis, separate from version control decisions.
4. **Safety.** Modifying `.gitignore` can have unintended consequences for the VCS. A `.baftignore` change only affects Baft.

Projects that want can use a single `.gitignore` and rely on Baft reading it. But `.baftignore` exists as an independent, higher-precedence layer for projects that need finer control.

---

## Mapping summary

| Concept                    | Baft equivalent                                                   |
| -------------------------- | ----------------------------------------------------------------- |
| `.gitignore`               | `.baftignore` (same syntax, higher precedence)                    |
| Git ignore rules           | Base entries + ancestor patterns + local patterns                 |
| Negation (`!`)             | Re-includes a previously ignored path                             |
| Directory traversal (`**`) | Matches zero or more directory segments                           |
| Directory-only (`/`)       | Matches only directories, not files                               |
| Ancestor walking           | Finds repo root, loads `.gitignore`/`.baftignore` from all levels |
| Filesystem wrapper         | `ignorefs.Wrap()` — intercepts all FS operations                  |
