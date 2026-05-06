# Manifest

A **Manifest** is the build-system file that a Capsule owns. It is the file
that declares identity, version, and dependencies — `go.mod`, `package.json`,
`Cargo.toml`, `pubspec.yaml`, `build.gradle.kts`.

A Manifest is both an ecosystem concept and a Strata concept. In each
ecosystem, it has its own semantics, format, and tooling. Strata reads
Manifests only to discover Capsules and extract module identifiers. It
does not execute build tools, resolve dependencies, or interpret build
logic.

Strata's view of a Manifest is narrow by design. It extracts exactly one
piece of data — the `CapsuleID` — and ignores everything else. The
ecosystem semantics are real and complete.

---

## The terminology problem

Every major ecosystem has a term for its build manifest:

| Ecosystem | Term used |
|-----------|-----------|
| Go        | module file |
| JavaScript/TypeScript (npm) | package manifest |
| Rust      | manifest |
| Dart      | pubspec |
| Kotlin/Java (Gradle) | build script |

These terms are not interchangeable. They carry language-specific baggage:

- **Module file** in Go refers specifically to `go.mod` and its `module`
  directive. It does not map to npm's `package.json` or Cargo's
  `Cargo.toml`.
- **Package manifest** in npm is a JSON file. In Python, "package" can
  refer to both the distribution unit (`setup.py`, `pyproject.toml`) and
  the import namespace.
- **Manifest** in Rust is the Cargo convention, but Rust also has
  `Cargo.lock` (a resolved dependency file) and `Cargo.toml` (the actual
  manifest). The term overlaps with "crate manifest" and "workspace
  manifest."
- **Pubspec** in Dart is a YAML file. The term is Dart-specific and
  carries no meaning outside the Dart ecosystem.
- **Build script** in Gradle is a Kotlin or Groovy file. It is both a
  manifest and a program, which is a fundamentally different model from
  the declarative manifests used by other ecosystems.

The tooling system is polyglot by design. It reads manifests to discover
Capsules and extract module identifiers. Using any single ecosystem's
terminology for the underlying concept would introduce bias and confusion.

---

## Definition

A **Manifest** is the language-agnostic name for the build-system file
that defines a Capsule. Each ecosystem has its own format and semantics.
Strata treats them all uniformly: walk the filesystem, locate files by
known names, parse the module identifier, produce a Capsule.

The Manifest is the source of truth for a Capsule's identity and
dependencies. The tooling only uses the identity (`CapsuleID`) —
dependencies, version, and build configuration are neither read nor
consumed by the check or draft commands.

One manifest, one Capsule.

---

## What a Manifest is not in Strata

A Manifest is not:

- **A build configuration in Strata.** The tooling does not execute build
  tools or interpret build logic. It reads manifests as raw text or JSON
  to extract identity. It does not run `go build`, `npm install`, `cargo
  build`, `dart pub get`, or `gradle build`.
- **A dependency resolver in Strata.** The tooling does not read lockfiles
  (`go.sum`, `package-lock.json`, `Cargo.lock`, `pubspec.lock`) or
  resolve external dependencies. External dependency information is not
  used by the check or draft commands.
- **A Strata Capsule by default.** Not every build manifest in an
  ecosystem becomes a Capsule in Strata. Cargo workspace roots with
  `[workspace]` but no `[package]` are valid manifests in Cargo but
  are not Capsules in Strata because they declare no module identity.
  Strata only treats manifests that declare a module identifier as
  Capsule manifests.
- **A STRATA.md.** The manifest defines identity and dependencies.
  `STRATA.md` defines architecture rules (nodes and allowed imports).
  They are separate files with separate purposes. A Capsule may have a
  manifest without a `STRATA.md` (meaning no architecture rules yet),
  but it cannot have a `STRATA.md` without a manifest.

---

## What a Manifest is in Strata

A Manifest is:

- **A Capsule owner.** A Capsule is defined by its manifest. Find the
  manifest, find the Capsule. The manifest declares identity and
  dependencies.
- **A discovery anchor.** The Capsule discovery service walks the
  filesystem looking for manifest files. When it finds one, it parses
  the file to extract the module identifier and produces a Capsule.
- **An identity provider.** The manifest provides the `CapsuleID` — the
  opaque identifier used by language adapters to resolve internal
  imports. This is the only piece of manifest data the tooling uses
  beyond discovery.
- **A normalized concept.** The Manifest is a language-agnostic view of
  ecosystem-specific files. The core tooling operates on the concept,
  not on any specific format.

---

## Discovery

Manifest discovery follows a two-step process: filename heuristic, then
semantic validation.

1. **Filename heuristic.** Each language registers known manifest file
   names (e.g. `go.mod` for Go, `package.json` for TypeScript). The
   discovery service walks the filesystem and checks each directory
   against all registered names.
2. **Semantic validation.** When a file matches a known name, the
   registered parser extracts the module identifier from its contents.
   If the result is non-empty, the file becomes a Capsule. If the
   result is empty, the file is skipped — it is not a Strata Capsule
   even though it may be a valid ecosystem file.

The discovery service also walks upward from the user-provided directory
to handle the common case where the user runs the tool from a subdirectory.
It climbs the tree to find the first ancestor containing a recognized
manifest, parses it, and treats that directory as the Capsule root.

Filename matching is the entry point. Module identity is the definition.

---

## Nested Capsules

A directory tree may contain multiple manifests, each defining a separate
Capsule. The discovery service finds all of them and produces a Capsule
for each one.

```
monorepo/
  package.json        ← Capsule 1 (workspace root)
  packages/
    shared/
      package.json    ← Capsule 2
    web/
      package.json    ← Capsule 3
    api/
      package.json    ← Capsule 4
```

Each `package.json` defines a separate Capsule. The monorepo structure
is metadata *around* the Capsules, not a Capsule itself.

A workspace root manifest that declares a collection of packages but does
not itself declare a module identifier is not a Capsule. It is a container
for Capsules, not a Capsule.

The manifest defines the canonical identity boundary used by the tooling,
not necessarily the operational ownership boundary. Real ecosystems violate
that equivalence constantly — Gradle multi-module projects, Cargo
workspaces, pnpm workspaces, nested Go modules, Bazel repos.

---

## Why not reuse an existing term

**Go module file** is too Go-specific. It refers to `go.mod` and its
`module` directive.

**Package.json** is npm-specific. It does not map to `Cargo.toml` or
`go.mod`.

**Build manifest** is used by Rust and .NET, but in .NET it refers to a
different file format entirely (`.deps.json`, `.runtimeconfig.json`).

**Module descriptor** carries Java/JPMS connotations that are misleading.

**Package manifest** overlaps with "package" in npm, Python, and Dart,
but those terms mean different things in different ecosystems.

**Manifest** was chosen because:

- It is short, pronounceable, and unambiguous.
- It is already used in Rust and .NET for the same concept.
- It does not inherit assumptions from any specific ecosystem.
- It conveys the idea of a declaration file without implying format,
  location, or language semantics.
- It works as a noun in commands, APIs, and documentation without
  requiring qualification ("Manifest file," "Manifest document" —
  neither is needed).

---

## Mapping summary

| Ecosystem | Manifest | Capsule |
|-----------|----------|---------|
| Go | `go.mod` (`module` line) | Go module |
| npm/TypeScript | `package.json` (`name` field) | npm package |
| Rust | `Cargo.toml` (`[package]` → `name`) | Cargo crate/package |
| Dart | `pubspec.yaml` (`name:` line) | Dart package |
| Gradle/Kotlin | `build.gradle.kts` (source path common prefix) | Gradle module |
