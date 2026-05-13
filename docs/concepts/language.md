# Language Modules

Each language module encapsulates everything that is specific to a programming
language. The core (graph engine, check use case, draft use case) is
completely language-agnostic — it knows nothing about Go, Dart, TypeScript,
Kotlin, or Rust. It only knows about a `Language` interface and a `Graph`
domain.

A language module is a self-contained capsule under
`internal/adapter/languages/<name>/` that implements the `port.Language`
interface defined in `internal/port/language.go`.

## The `Language` interface

```go
type Language interface {
    Name() string
    IsScannableFile(rel string) bool
    ParseImports(fileSystem FileSystem, absPath string) ([]ImportSpec, error)
    ResolveInternalTarget(fileSystem FileSystem, spec ImportSpec, c Capsule, fileRel string) (targetDir string, internal bool)
    SupportsFileGlobs() bool
    Register(d CapsuleDiscovery)
}
```

Every method on this interface is a language responsibility. None of them can
be meaningfully shared across languages.

---

## 1. `Name()`

Returns a short identifier used in diagnostics and CLI flags (e.g. `"go"`,
`"dart"`, `"kotlin"`).

---

## 2. `IsScannableFile(rel string) bool`

Returns `true` if a file should be scanned for imports. This encodes the
language's conventions for which files are source code worth analyzing:

| Language   | Scannable files                     | Excluded                                        |
| ---------- | --------------------------------- | ----------------------------------------------- |
| Go         | `*.go` (all Go files)             | none                                            |
| Dart       | `*.dart` under `lib/`             | `_test.dart`, `.g.dart`, `.freezed.dart`        |
| TypeScript | `*.ts`, `*.tsx`                   | `.d.ts`, `.test.ts`, `.spec.ts`                 |
| Kotlin     | `*.kt` in 28+ source set prefixes | `Test.kt`, `/generated/`, `/ksp/`, `/buildSrc/` |
| Rust       | `*.rs` under `src/`               | `src/bin/`, `src/examples/`, `build.rs`         |

The core uses this filter during file walking (`service.WalkCapsule`,
`service.WalkAllFiles`) to skip files that should not be checked.

---

## 3. `ParseImports(fileSystem FileSystem, absPath string) ([]ImportSpec, error)`

Extracts import information from a source file and returns a slice of
`ImportSpec` structs, each containing:

- **Path** — the raw import specifier string
- **Line** — 1-indexed line number in the source file
- **Col** — 1-indexed column where the import path starts
- **ColEnd** — 1-indexed column where the import path ends

The format and mechanism are entirely language-specific:

| Language   | Mechanism                                    | Import format                                     |
| ---------- | -------------------------------------------- | ------------------------------------------------- |
| Go         | AST-based (`go/parser`, `go/token`)          | `"github.com/user/repo/path"`                     |
| Dart       | Regex on `import`/`export`/`part` directives | `"lib/path/to/file"`                              |
| TypeScript | Four regex patterns                          | `"./relative"`, `"@alias/path"`, `"package-name"` |
| Kotlin     | Regex on `import` statements                 | `"com.example.module.Class"`                      |
| Rust       | Regex on `use`/`mod`/`extern crate`          | `"crate::path::to::item"`                         |

Go uses the official parser for correctness. The others use carefully
constructed regex patterns. The output is always a slice of `ImportSpec`
structs with position info — the core never sees the parsing logic. The
position data (`Line`, `Col`, `ColEnd`) enables precise diagnostics and
error reporting in the check command.

---

## 4. `ResolveInternalTarget(fileSystem FileSystem, spec ImportSpec, c Capsule, fileRel string) (targetDir string, internal bool)`

This is the most complex method. It takes an `ImportSpec` (the output of
`ParseImports`, which includes the raw import path plus line/column position
info) and answers two questions:

1. **Is this an internal import?** — Does it refer to code within the same
   capsule/module, or is it an external/stdlib dependency?
2. **If internal, what is the capsule-relative path?** — A path that the
   core can use as a node key in the dependency graph.

The resolution semantics are language-specific:

| Language   | Internal check                                              | Path resolution                                        |
| ---------- | ----------------------------------------------------------- | ------------------------------------------------------ |
| Go         | Prefix match against `CapsuleID`                            | Strip `CapsuleID/` prefix                              |
| Dart       | `package:` URI name matches `CapsuleID`                     | Map `package:<name>/<path>` to `lib/<path>`            |
| TypeScript | `tsconfig.json` paths alias, `baseUrl`, package name match  | Resolve extensions (`.js` -> `.ts`), `index.ts`        |
| Kotlin     | Prefix match against base capsule (dot-separated)           | Convert dots to slashes, prepend source prefix         |
| Rust       | `crate::` prefix, `super::`/`self::` hops, crate name match | Resolve multi-hop `super::` paths, `crate::` from root |

Each language also handles its own special cases:

- TypeScript resolves `tsconfig.json` path aliases and `extends` chains
- Rust handles aliased imports (`use X as Y`) and visibility modifiers
- Dart handles `dart:` built-in imports (always external)

The core receives only the result: a path string and a boolean. It has no
knowledge of how that path was computed.

---

## 5. `SupportsFileGlobs() bool`

Returns `true` if the language's `BAFT.md` can use file-shaped node
definitions (e.g. `lib/main.dart` as a node). Only Dart and TypeScript
support this — Go, Kotlin, and Rust only support directory-level nodes.

This affects how the core builds node keys in the draft command
(`graph.NodeKey`) and how the check command validates file-to-node mapping
(`graph.NodeForPath`).

---

## 6. `Register(d CapsuleDiscovery)`

Registers this language with the capsule discovery service. The method
receives a `CapsuleDiscovery` interface and calls `d.Register()` with a
`ManifestInfo` struct containing:

- **Names** — the manifest file name(s) to look for (e.g. `go.mod`,
  `pubspec.yaml`, `package.json`)
- **ParseFunc** — a function that reads the manifest and extracts the capsule
  identifier (module name, package name, etc.)
Each language adapter implements its own manifest parser:

| Language   | Manifest file(s)   | Parser function    | Extracted value         |
| ---------- | ------------------ | ------------------ | ----------------------- |
| Go         | `go.mod`           | `readGoModulePath` | `module github.com/...` |
| Dart       | `pubspec.yaml`     | `readPubspecName`  | `name: my_package`      |
| TypeScript | `package.json`     | `readCapsuleName`  | `"name": "my-package"`  |
| Kotlin     | `build.gradle.kts` | `readGradleName`   | `namespace = "..."`     |
| Rust       | `Cargo.toml`       | `readCargoName`    | `[package] name = ...`  |

This method is called once during application startup so the discovery service
knows which files to look for and how to parse them.

---

## Capsule Discovery (moved out of Language)

Capsule discovery — finding capsules by locating manifest files, walking the
tree, parsing manifest data, and resolving contract paths — is no longer the
responsibility of the `Language` interface. It lives in the
`CapsuleDiscovery` service in `internal/application/service/`.

Each language registers with the discovery service by providing:

- **Manifest file names** — e.g. `["go.mod"]`, `["pubspec.yaml"]`,
  `["build.gradle.kts", "build.gradle"]`
- **Module ID parser** — a function that reads a manifest file and extracts
  the module identifier (e.g. the `module` line from `go.mod`)

The use cases (`check.Run`, `draft.Run`) call the discovery service
directly. The service returns `Capsule` structs with `Dir` and `CapsuleID`
resolved. The language adapter is then used only for
`IsScannableFile`, `ParseImports`, `ResolveInternalTarget`,
`SupportsFileGlobs`.

This separation means the language interface is lean — it contains only
semantics that are genuinely language-specific. The boilerplate of tree
walking, ancestor traversal, and contract path resolution is shared code that
no language adapter should duplicate.

---

## What language modules do NOT do

Language modules do not:

- **Discover capsules** — Capsule discovery is handled by the core
  `CapsuleDiscovery` service. Languages only register their manifest names
  and module ID parser.
- **Build the graph** — The core (`draft.Run`, `check.Run`) assembles
  nodes and edges from the paths that languages return.
- **Validate rules** — The core checks whether edges between nodes are allowed
  by the `BAFT.md` graph.
- **Parse BAFT.md** — The `mermaid.MermaidRepository` loads and saves the
  mermaid flowchart format.
- **Walk the file tree** — `service.WalkCapsule` and `service.WalkAllFiles`
  handle traversal; languages only provide the `IsScannableFile` filter.
- **Report output** — `Reporter` implementations (text, JSON) produce the
  final output.

The language module's job is strictly: **identify scannable files, extract
imports from those files, resolve import targets to capsule-relative paths**.
Everything else is the core's responsibility.
