# Language Modules

Each language module encapsulates everything that is specific to a programming
language. The core (graph engine, check use case, dump use case) is
completely language-agnostic — it knows nothing about Go, Dart, TypeScript,
Kotlin, Python, Rust, Java, or C#. It only knows about a `Language` interface and a `Graph`
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
    GetFileNamespace(fileSystem FileSystem, absPath string) (string, error)
    ResolveInternalTarget(fileSystem FileSystem, spec ImportSpec, c Capsule, fileRel string) (targetDir string, internal bool)
    SupportsFileGlobs() bool
    Register(d CapsuleDiscovery)
}
```

`internal/port/language.go` is the source of truth for that signature; the sections below describe what each method owes the core.

Every method on this interface is a language responsibility. None of them can
be meaningfully shared across languages.

---

## 1. `Name()`

Returns a short identifier used in diagnostics and CLI flags (e.g. `"go"`,
`"dart"`, `"jvm"`). `--lang jvm` is the only name for Java and Kotlin: one
Gradle/Maven capsule compiles both, so one adapter scans `.java` and `.kt`.

---

## 2. `IsScannableFile(rel string) bool`

Returns `true` if the file can be scanned for imports. The core uses this
filter during file walking to skip files it has no parser for. Each language
simply checks for its source file extension (e.g. `*.go`, `*.py`, `*.ts`).

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
- **Namespace** — the raw namespace string from the import (e.g., `MyApp.Domain` for C# `using`, `com.example.api` for Java/Kotlin). Empty for languages that don't declare namespaces.

The format and mechanism are entirely language-specific:

| Language   | Mechanism                                    | Import format                                     |
| ---------- | -------------------------------------------- | ------------------------------------------------- |
| Go         | AST-based (`go/parser`, `go/token`)          | `"github.com/user/repo/path"`                     |
| Dart       | Regex on `import`/`export`/`part` directives | `"lib/path/to/file"`                              |
| TypeScript | Four regex patterns                          | `"./relative"`, `"@alias/path"`, `"package-name"` |
| JVM        | Regex on `import` statements                 | `"com.example.module.Class"`                      |
| Python     | Regex on `import`/`from X import` statements | `"package.module.submodule"`                      |
| Rust       | Regex on `use`/`mod`/`extern crate`          | `"crate::path::to::item"`                         |
| C#         | Regex on `using` directives                  | `"MyApp.Domain.Entities"` (namespace), populates `Namespace` field |

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
| JVM        | Prefix match against base capsule (dot-separated)           | Convert dots to slashes, prepend the source set holding the target |
| Python     | Prefix match against base capsule (dot-separated)           | Convert dots to slashes, prepend source prefix         |
| Rust       | `crate::` prefix, `super::`/`self::` hops, crate name match | Resolve multi-hop `super::` paths, `crate::` from root |
| C#         | Prefix match against assembly/capsule name (dot-separated) | Convert dots to slashes, resolve from source root |

Each language also handles its own special cases:

- TypeScript resolves `tsconfig.json` path aliases and `extends` chains
- Rust handles aliased imports (`use X as Y`) and visibility modifiers
- Dart handles `dart:` built-in imports (always external)
- JVM resolves across source sets: a `src/main/java` file importing a class under `src/main/kotlin` targets the Kotlin path, and vice versa

The core receives only the result: a path string and a boolean. It has no
knowledge of how that path was computed.

---

## 5. `SupportsFileGlobs() bool`

Returns `true` if the language's contract file can use file-shaped node
definitions (e.g. `lib/main.dart` as a node). Only Dart and TypeScript
support this — Go, Java, Kotlin, Python, Rust, and C# only support directory-level nodes.

Directory-level nodes have two distinct meanings:

- `path/to/dir` matches files directly in that directory.
- `path/to/dir/**` matches the subtree rooted at that directory.

This affects how the core builds node keys in the dump command
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
- **BaseIgnoreEntries** — directory names and file globs that are invisible to
  Baft for this language (e.g. `vendor`, `*_test.go`, `*.generated.cs`)

Each language adapter implements its own manifest parser:

| Language   | Manifest file(s)                              | Extracted value                       |
| ---------- | --------------------------------------------- | ------------------------------------- |
| Go         | `go.mod`                                      | `module github.com/...`               |
| Dart       | `pubspec.yaml`                                | `name: my_package`                    |
| TypeScript | `package.json`                                | `"name": "my-package"`                |
| JVM        | `build.gradle.kts`, `build.gradle`, `pom.xml` | common package prefix from source     |
| Python     | `pyproject.toml`, `setup.py`                  | common package prefix from source     |
| Rust       | `Cargo.toml`                                  | `[package] name = ...`                |
| C#         | `*.csproj`                                    | `<RootNamespace>` or `<AssemblyName>` |

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

The use cases (`check.Run`, `dump.RunWithOptions`) call the discovery service
directly. The service returns `Capsule` structs with `Dir` and `CapsuleID`
resolved. The language adapter is then used only for
`IsScannableFile`, `ParseImports`, `ResolveInternalTarget`,
`SupportsFileGlobs`.

This separation means the language interface is lean — it contains only
semantics that are genuinely language-specific. The boilerplate of tree
walking, ancestor traversal, and contract path resolution is shared code that
no language adapter should duplicate.

---

## 7. `GetFileNamespace(fsys FileSystem, absPath string) (string, error)`

Returns the namespace declaration from a source file's header, or `("", nil)` if none exists.

| Language   | Implementation                                    |
| ---------- | ------------------------------------------------- |
| C#         | Regex on `namespace MyApp.Api` declaration        |
| JVM        | Regex on `package com.example.api` declaration    |
| Go         | `("", nil)` — no namespace concept                |
| TypeScript | `("", nil)` — no namespace concept                |
| Dart       | `("", nil)` — no namespace concept                |
| Python     | `("", nil)` — no namespace concept                |
| Rust       | `("", nil)` — no namespace concept                |

### Namespace Mode

A contract opts into namespace mode via `%% config namespaceMode "true"`. Import targets are then resolved by namespace string instead of filesystem path: the check builds a namespace index (file path → declared namespace) and matches each `using`/`import` against it. It is per-contract and applies to any language whose `GetFileNamespace` returns a value — C#, Java, and Kotlin today. See [contract.md](contract.md#namespace-mode).

Node patterns may use wildcards (`api["MyApp.Api.&ast;"]`); only a pattern containing `/` is file-shaped. Each `using` counts as one relation regardless of how many files share the target namespace. If no scanned file declares a namespace, the check reports `namespace-mode-no-namespaces` instead of silently reverting to path matching.

Language modules do not:

- **Discover capsules** — Capsule discovery is handled by the core
  `CapsuleDiscovery` service. Languages only register their manifest names
  and module ID parser.
- **Build the graph** — The core (`dump.RunWithOptions`, `check.Run`) assembles
  nodes and edges from the paths that languages return.
- **Validate rules** — The core checks whether edges between nodes are allowed
   by the contract file graph.
- **Parse contract file** — The `mermaid.MermaidRepository` loads and saves the
  mermaid flowchart format.
- **Walk the file tree** — `service.WalkCapsule` and `service.WalkAllFiles`
  handle traversal; languages only provide the `IsScannableFile` filter.
- **Report output** — `Reporter` implementations (text, JSON) produce the
  final output.

The language module's job is strictly: **identify scannable files, extract
imports from those files, resolve import targets to capsule-relative paths,
and report the file's namespace (if any)**. Everything else is the core's responsibility.
