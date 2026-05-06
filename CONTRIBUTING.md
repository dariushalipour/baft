# Contributing

## Quick start

```
go build -o strata .
./strata check /path/to/repo
./strata draft /path/to/repo
go test ./...
```

## Architecture

```
strata/
├── main.go                          # CLI: subcommands, version, reporters
├── go.mod                           # Zero external dependencies
└── internal/
    ├── ui/ui.go                     # Terminal output (✓, ✗, ℹ)
    └── strata/
        ├── language.go              # Language interface + Capsule struct
        ├── graph.go                 # Mermaid parser, glob matching, Graph type
        ├── check.go                 # File walk + rule application
        ├── strata.go                # Run() — orchestrates discovery + checks
        ├── golang/golang.go         # Go adapter
        ├── dart/dart.go             # Dart adapter
        ├── kotlin/kotlin.go         # Kotlin adapter
        ├── typescript/typescript.go # TS adapter
        └── rust/rust.go             # Rust adapter
```

The core (`graph.go`, `check.go`, `strata.go`, `language.go`) knows nothing about Go, Dart, or Kotlin. Language-specific logic lives behind the `Language` interface.

## Adding a language adapter

Create a new capsule under `internal/strata/<lang>/` and implement the `Language` interface:

```go
type Language interface {
    Name() string
    Discover(rootDir string) ([]Capsule, error)
    IsGovernedFile(rel string) bool
    ParseImports(absPath string) ([]string, error)
    ResolveInternalTarget(spec string, c Capsule, fileRel string) (targetDir string, internal bool)
    SupportsFileGlobs() bool
}
```

Each method:

| Method | Purpose |
|---|---|
| `Name()` | Short identifier for diagnostics (e.g. `"go"`, `"dart"`) |
| `Discover()` | Walk the repo tree, return every directory that has both a module manifest and a `STRATA.md` |
| `IsGovernedFile()` | Return `true` for source files that should be checked (exclude tests, generated files, etc.) |
| `ParseImports()` | Extract raw import specifiers from a file. Language-specific format. |
| `ResolveInternalTarget()` | Map a raw import specifier to a capsule-relative path. Return `internal=false` for external/stdlib imports. |
| `SupportsFileGlobs()` | Return `true` if individual files can be claimed by nodes (e.g. `lib/src/providers.dart`). Return `false` for directory-only languages (Go). |

Then register it in `main.go`:

```go
result := strata.Run(root, []strata.Language{
    golang.Language{},
    dart.Language{},
    mylang.Language{},
}, strata.OptJSON(jsonOut))
```

That's it. No other changes needed.

## STRATA.md format

Each governed capsule has a `STRATA.md` at its root. The first ```mermaid block is parsed:

````markdown
```mermaid
flowchart TD
  main["."]
  httpapi["internal/adapter/http/**"]
  usecase["internal/usecase/**"]:::endophobic
  domain["internal/domain/**"]

  main --> httpapi --> usecase --> domain
```
````

- **Nodes**: `[ID]["<glob>"]` — the glob claims directories or files inside the capsule
- **Edges**: `A --> B` — node A may import node B
- `:::endophobic` — forbids all same-node imports — files in an endophobic node cannot import any other file in the same node

### Glob syntax

| Glob | Matches |
|---|---|
| `.` | Only the capsule root |
| `internal/domain/**` | `internal/domain` and any subdirectory |
| `internal/infra/*` | Exactly `internal/infra/<one-segment>` |
| `internal/infra/*/**` | `internal/infra/<x>/<y>` and deeper (not the port dir itself) |
| `lib/src/providers.dart` | A single file (only for languages that support file globs) |

Most specific match wins. File-shaped globs beat directory-shaped globs.

## Testing

Run all tests:

```
go test ./...
```

Tests are unit-only — no mocks, no fakes, no integration. The graph parser and glob matcher have thorough coverage in `graph_test.go`. Each adapter has its own test file.

When adding an adapter, test at minimum:
- `IsGovernedFile` with representative paths
- `ParseImports` with a synthetic file
- `ResolveInternalTarget` with internal, external, and edge-case imports

## Common pitfalls

### `append` mutates slices in-place

```go
// WRONG — candidate and common share the same backing array
candidate := append(common, p)
// ... use candidate ...
common = append(common, p)  // common now has p twice

// RIGHT — copy first, then append
candidate := append([]string(nil), common...)
candidate = append(candidate, p)
// ... use candidate ...
common = append(common, p)
```

When you `append` to a slice that has spare capacity, Go reuses the backing array. A "candidate" slice built from `common` will silently mutate `common` if you later append to `common`. Always copy with `append([]string(nil), src...)` before building a temporary view.

### Regex capture groups eat optional suffixes

```go
// WRONG — captures the wildcard: "com.example.utils.*"
re := regexp.MustCompile(`import\s+([A-Za-z_.\*]+)`)

// RIGHT — wildcard outside the capture group
re := regexp.MustCompile(`import\s+([A-Za-z_][A-Za-z0-9_.]*)\.\*?`)
```

If an optional part of your pattern (like `.*` wildcards) sits inside a capture group, it becomes part of the captured string. Keep optional suffixes outside the group, or strip them in code after capture.

### Capsule prefix matching requires word boundaries

```go
// WRONG — "com.example2" matches "com.example"
strings.HasPrefix(spec, basePkg)

// RIGHT — check the next character is a dot
strings.HasPrefix(spec, basePkg) && spec[len(basePkg)] == '.'
```

`strings.HasPrefix("com.example2", "com.example")` is `true`. Always verify the character after the prefix is `.` (or end-of-string for exact matches).

### Cumulative prefix algorithms need running state

Finding a common capsule prefix across multiple paths requires building up the candidate cumulatively:

```go
// WRONG — checks each part in isolation
for _, p := range parts {
    for _, path := range allPaths {
        if !strings.HasPrefix(path, p+"/") { /* fail */ }
    }
}

// RIGHT — builds cumulative prefix
candidate := []string{}
for _, p := range parts {
    candidate = append(candidate, p)
    prefix := strings.Join(candidate, "/") + "/"
    for _, path := range allPaths {
        if !strings.HasPrefix(path, prefix) && path != strings.Join(candidate, "/") {
            goto done
        }
    }
done:
}
```

Checking each path segment individually doesn't work — you need to verify the full accumulated prefix against every path.

### Kotlin multi-platform has many source sets

Kotlin isn't just `src/main/kotlin`. Multi-platform projects use `commonMain`, `jvmMain`, `androidMain`, `iosMain`, `darwinMain`, `jsMain`, `nativeMain`, and their `*Test` counterparts. Your `IsGovernedFile` and `findBaseCapsule` must recognize all of them.

### Generated files need explicit exclusion

Kotlin code generators produce files in predictable paths. Exclude them in `IsGovernedFile`:

```
/generated/
/kapt/
/ksp/
/buildSrc/
```

These directories can appear inside source trees and will produce false positives if not filtered.

### Go version compatibility with composite literals

`Language{}.Name()` may fail to compile on Go 1.21 depending on the receiver type. Use `(Language{}).Name()` with explicit parentheses to disambiguate.

### `filepath.Join` doesn't accept spread slices

```go
// WRONG — compile error: cannot use parts ([]string) as type string
filepath.Join("src", parts...)

// RIGHT — use append to build the argument list
filepath.Join(append([]string{"src"}, parts...)...)
```

`filepath.Join` takes variadic `string` args, not a `[]string`. The `append([]string{"base"}, slice...)` pattern is the idiomatic workaround.

## Rules

- Default to **no comments**. If the reason for code is non-obvious, rename or refactor instead.
- One short line max when a comment is warranted. Explain **why**, never **what**.
- Fix every broken test you encounter.
- Prefer clarity over cleverness. A future maintainer will read this code.

## Releasing

Strata uses semantic versioning with `v`-prefixed Git tags (e.g. `v0.1.0`).

### Steps

1. Ensure tests pass:
   ```
   go test ./...
   ```

2. Tag the release:
   ```
   git tag -s v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```

3. Create a GitHub release at `https://github.com/dariushalipour/strata/releases/new`:
   - Target the new tag
   - Auto-generate release notes or summarize changes
   - Publish

4. Verify `go install` works:
   ```
   go install github.com/dariushalipour/strata@v0.1.0
   strata --version
   ```

### Version scheme

- `v0.x.y` — pre-1.0, breaking changes may occur between minor versions
- `v1.x.y` — stable API, semver-compliant

### Building with version info

For local builds that report a proper version string:

```
go build -ldflags "-X main.version=v0.1.0" -o strata .
```

The `main.version` variable is set to `"dev"` by default when not provided via `-ldflags`.
