# Capsules

A **Capsule** is the fundamental self-contained code boundary in a polyglot
dependency graph. It is the unit that owns a manifest, defines a dependency
graph, carries its own architecture rules, and supports independently addressable
tooling operations.

A Capsule is an abstraction. It is not a filesystem layout, not a directory
convention, and not a language construct. It is the normalized concept that
tooling operates on when it needs to reason about code boundaries across
languages.

---

## The terminology problem

Every major ecosystem has a term for its primary code boundary:

| Ecosystem                   | Term used                   |
| --------------------------- | --------------------------- |
| Go                          | module                      |
| JavaScript/TypeScript (npm) | package                     |
| Rust                        | crate / package / workspace |
| Dart                        | package                     |
| Kotlin/Java (Gradle)        | module / project            |

These terms are not interchangeable. They carry language-specific baggage:

- **Module** means different things. In Go, a module contains packages. In
  JavaScript, a module is a single file. In Gradle, a module is a dependency
  unit.
- **Package** means different things. In npm, a package is a dependency unit
  with a `package.json`. In Java, a package is a namespace inside a JAR.
- **Crate** is a Rust-specific concept — a compilation unit. It overlaps with
  "package" in Cargo, and both coexist with "workspaces" for multi-crate
  projects.
- **Workspace** means a collection of packages in Cargo and npm. In other
  ecosystems, the equivalent concept is called "multi-module project" or
  "monorepo."
- **Project** is used by Gradle, Maven, and IDEs, but means something
  different from a Gradle module (a project may contain multiple modules).

The tooling system is polyglot by design. It walks trees, resolves imports,
builds dependency graphs, and enforces architecture rules across all of these
ecosystems simultaneously. Using any single ecosystem's terminology for the
underlying concept would introduce bias, confusion, and mental overhead.

---

## Definition

A **Capsule** is a language-agnostic abstraction representing a self-contained
code boundary defined by:

1. **A manifest** — a file that declares identity, version, and dependencies
   (e.g. `go.mod`, `package.json`, `Cargo.toml`, `pubspec.yaml`,
   `build.gradle.kts`).
2. **A dependency graph** — the set of internal imports and external
   dependencies that the manifest and source files collectively define.
3. **Architecture rules** — BAFT.md defines nodes, edges, and constraints that track internal imports, along with other declarations that shape how the code boundary is interpreted.
4. **A lifecycle** — the ability to be discovered, parsed, validated, and
   addressed as a unit by tooling.
5. **Independently addressable operations** — tooling commands (check, dump)
   operate on Capsules without needing to know the underlying language.

A Capsule maps to exactly one manifest file. One manifest, one Capsule.

---

## What a Capsule is not

A Capsule is not:

- **A filesystem path.** A Capsule has a directory (the one containing its
  manifest), but the Capsule itself is the boundary concept, not the
  directory.
- **A package, module, crate, or namespace.** Those are language-specific
  constructs. A Capsule may _contain_ them, or _be_ them, but it is not
  any one of them.
- **A directory-level convention.** Capsules are identified by manifest
  presence, not by directory structure. Two Capsules may live in the same
  directory tree without any special layout.
- **A compilation unit.** A Rust crate is a compilation unit. A Go module is
  not. A Capsule is neither — it is the normalized boundary that tooling
  uses regardless of compilation model.
- **A scope for architecture rules.** Architecture rules (BAFT.md) apply
  per-Capsule, but the Capsule itself does not define the rules. It is the
  container that holds them.

---

## What a Capsule is

A Capsule is:

- **A manifest boundary.** Find the manifest, find the Capsule. The manifest
  declares identity (module path, package name, crate name) and external
  dependencies.
- **A graph node in tooling.** Tooling treats a Capsule as an addressable
  unit. Commands resolve to Capsules, report against Capsules, and validate
  Capsules independently.
- **A scoping boundary for architecture rules.** Each Capsule may contain
  its own `BAFT.md`. Rules inside a Capsule track only imports within that
  Capsule. Cross-Capsule imports are tracked by the parent Capsule or by
  external dependency declarations.
- **A discovery target.** Capsule discovery walks the filesystem, locates
  manifest files, parses their contents, and produces Capsule structs with
  resolved directory, identity, and contract path.

---

## Why not reuse an existing term

**Module** is the closest candidate. It is used by Go and Gradle.
But in JavaScript, "module" means a single file. Using "module" for a
Capsule would require constant clarification: "this module is not that
module."

**Package** is used by npm and Dart. But in Java, "package" is a
namespace, not a dependency unit. In Rust, "package" and "crate" overlap
confusingly.

**Crate** is Rust-only.

**Project** is IDE and build-tool oriented. It does not map cleanly to
dependency boundaries.

**Unit** is too generic. **Boundary** is too vague. **Context** carries
DDD connotations that are misleading here.

**Capsule** was chosen because it:

- Does not inherit assumptions from any existing ecosystem.
- Is short, pronounceable, and unambiguous.
- Conveys the idea of a self-contained boundary without implying filesystem
  layout, compilation model, or language semantics.
- Does not collide with any existing term in Go, JavaScript, Rust, Dart,
  or Kotlin.
- Works as a noun in commands, APIs, and documentation without requiring
  qualification ("Capsule module," "Capsule package" — neither is needed).

---

## Design principles

1. **Manifest-driven.** A Capsule exists where a manifest exists. No
   manifest, no Capsule. The manifest is the source of truth for identity
   and dependencies.

2. **One manifest, one Capsule.** Each manifest file defines exactly one
   Capsule. A workspace or monorepo contains multiple Capsules, each with
   its own manifest.

3. **Language-agnostic.** The Capsule concept does not encode language
   semantics. Language-specific behavior lives in language adapters. The
   core sees only manifest boundaries, directory paths, and resolved imports.

4. **Composable.** Capsules nest through directory structure. A monorepo
   contains Capsules. A Capsule may contain sub-packages, modules, or
   namespaces — those are internal to the Capsule, not separate Capsules.

5. **Addressable.** Tooling commands resolve to Capsules by directory.
   Operations (check, dump) are scoped to a Capsule's boundary.
   Cross-Capsule references are explicit edges in the dependency graph.

6. **Not a filesystem concept.** Capsules are identified by manifest presence,
   not by directory naming conventions or layout patterns. The filesystem is
   the discovery mechanism, not the definition.

---

## Examples

### A Go module as a Capsule

```
my-service/
  go.mod          ← Capsule manifest (identity: my-service)
  main.go
  internal/
    auth/
      auth.go
    billing/
      billing.go
```

The `go.mod` file defines one Capsule. The `internal/auth` and `internal/billing`
directories are Go packages — they are _inside_ the Capsule but are not
themselves Capsules (they have no `go.mod`).

### An npm package as a Capsule

```
web-app/
  package.json    ← Capsule manifest (identity: web-app)
  src/
    index.ts
    lib/
      utils.ts
```

The `package.json` defines one Capsule. The TypeScript files and directories
are modules and namespaces inside the Capsule.

### A Cargo workspace as multiple Capsules

```
rust-project/
  Cargo.toml          ← workspace manifest (not a Capsule itself)
  crates/
    core/
      Cargo.toml      ← Capsule 1 (identity: rust-project-core)
      src/
    cli/
      Cargo.toml      ← Capsule 2 (identity: rust-project-cli)
      src/
    sdk/
      Cargo.toml      ← Capsule 3 (identity: rust-project-sdk)
      src/
```

The workspace root's `Cargo.toml` declares a collection of packages. Each
package with its own `Cargo.toml` is a separate Capsule. The workspace itself
is not a Capsule — it is a container for Capsules.

### A monorepo containing multiple Capsules

```
monorepo/
  package.json        ← Capsule 1 (workspace root, may or may not be a Capsule
                       depending on whether it declares dependencies)
  packages/
    shared/
      package.json    ← Capsule 2
    web/
      package.json    ← Capsule 3
    api/
      package.json    ← Capsule 4
```

Each `package.json` defines a Capsule. The monorepo root is a Capsule if it
has its own manifest with dependency declarations. The workspace or monorepo
structure is metadata _around_ the Capsules, not a Capsule itself.

### Nested source namespaces inside a Capsule

```
go-service/
  go.mod          ← Capsule
  internal/
    auth/         ← Go package (inside Capsule, not a Capsule)
      handler.go
    billing/      ← Go package (inside Capsule, not a Capsule)
      handler.go
  pkg/            ← Go package (inside Capsule, not a Capsule)
    client/       ← Go package (inside Capsule, not a Capsule)
      client.go
```

Go packages, Java packages, Dart libraries, TypeScript modules, and Rust
modules are all internal organizational units _within_ a Capsule. They do not
become Capsules unless they carry their own manifest.

---

## Capsules in the tooling system

The tooling system operates on Capsules as its primary unit of work:

**Discovery.** `CapsuleDiscovery` walks the filesystem, locates manifest files
by name (`go.mod`, `package.json`, `Cargo.toml`, etc.), parses each manifest
to extract the module identifier, and produces a Capsule struct with resolved
`Dir` and `CapsuleID`.

**Graph building.** Each Capsule contributes nodes (internal source
directories/files) and edges (internal imports) to the dependency graph.
Cross-Capsule imports are external edges.

**Check.** The check command evaluates each Capsule independently. A
Capsule's `BAFT.md` tracks only imports within that Capsule's directory.
Cross-Capsule imports are validated by the parent Capsule's rules or by
external dependency declarations.

**Dump.** The dump command generates `BAFT.md` files per Capsule by
scanning the Capsule's source files, resolving imports, and mapping them to
graph nodes.

**Language adapters.** Language adapters implement `port.Language` and handle
everything language-specific: which files are tracked, how imports are
parsed, how internal targets are resolved. The core sees only the resulting
paths and booleans — it operates on Capsules uniformly regardless of
language.

---

## Mapping summary

| Ecosystem      | Manifest                 | Capsule             | Internal units                             |
| -------------- | ------------------------ | ------------------- | ------------------------------------------ |
| Go             | `go.mod`                 | Go module           | Go packages (`internal/`, `pkg/`)          |
| npm/TypeScript | `package.json`           | npm package         | TypeScript modules (files), namespaces     |
| Rust           | `Cargo.toml` (per crate) | Cargo crate/package | Rust modules (`mod`, `crate::`)            |
| Dart           | `pubspec.yaml`           | Dart package        | Dart libraries (`lib/`)                    |
| Gradle/Kotlin  | `build.gradle.kts`       | Gradle module       | Kotlin packages (dot-separated namespaces) |
