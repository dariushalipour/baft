# Contributing

## Quick start

```bash
go build -o baft .
./baft check .
./baft dump .
go test ./...
```

## Quality gate

There is no hosted CI. `scripts/ci.sh` is the gate, and it is run by hand before every push:

```bash
./scripts/ci.sh
```

It stops at the first failure and covers, in order: `go fmt ./...` (which rewrites files, and fails the run if anything needed rewriting), `staticcheck ./...`, `go test ./...`, `baft check .` against Baft's own contract, `npm run compile` and `npm test` in `vscode-extension/`, then `./gradlew compileKotlin` and `./gradlew test` in `intellij-plugin/`. A plugin section is skipped only when its directory is absent.

Prerequisites:

- Go. `go.mod` declares `go 1.21`, and that floor is not tested anywhere — do not use language or stdlib features newer than 1.21.
- `staticcheck`: `go install honnef.co/go/tools/cmd/staticcheck@latest`
- Node and npm for the VS Code extension; a JDK for the Gradle wrapper.

Two suites `ci.sh` does not run. Run them by hand when you touch the code they exercise:

```bash
go build -o baft . && BAFT_BINARY="$PWD/baft" go test ./internal/integrations/...  # CLI contract tests, skipped without the env var
go test -race ./...
```

## Architecture

Hexagonal, one direction: `main.go` → `internal/cli/` → `application/usecase/*` → `internal/port/` → `domain/graph`. Everything that touches the outside world (filesystems, Mermaid, languages, reporters) is an adapter under `internal/adapter/`. The domain knows nothing about any language. The root `BAFT.md` is the authority on where a new package goes — `internal/integrations/` (embedded editor plugins) and `internal/treeview/` (`dump` output rendering) are declared there as their own nodes, not as adapters.

The concepts each layer implements are documented in [docs/concepts/](docs/concepts/): [capsule](docs/concepts/capsule.md), [manifest](docs/concepts/manifest.md), [contract](docs/concepts/contract.md), [language](docs/concepts/language.md), [validation](docs/concepts/validation.md), [.baftignore](docs/concepts/baftignore.md).

This layering is not a convention — it is enforced by Baft itself. The root `BAFT.md` is Baft's own contract, and both `go test ./...` and `scripts/ci.sh` run `baft check .` against it. Adapters are wired only in `internal/cli/`, the composition root; the application layer talks to them through `port/`.

## Adding a language adapter

`internal/port/language.go` is the source of truth for the method set — read the interface there rather than a copy in prose. Then:

1. Create `internal/adapter/languages/<lang>/` implementing `port.Language`.
2. In `Register`, declare the manifest `Names`, a `ParseFunc` that extracts the capsule identifier, and `BaseIgnoreEntries` for test and generated files.
3. Register it in `internal/cli/registry.go`: add the name to the `languageNames` default slice and a case to the `newLanguage` switch. Those two together are the registry; `resolveLangs` only dedupes what they return.
4. Add the name to the language lists in `docs/cli-assets/check-usage.txt`, `docs/cli-assets/dump-usage.txt` and `docs/cli-assets/help-intro.txt`. `TestCLIAssetsListEveryLanguage` fails until you do.
5. Add `<lang>_test.go` covering `IsScannableFile`, `ParseImports`, `ResolveInternalTarget`, and `GetFileNamespace`, plus a `<lang>.feature` beside it for the end-to-end path. Go, TypeScript and C# have one; Dart, JVM, Python and Rust predate the rule and are still owed theirs.
6. Update the tables in `README.md` and [docs/concepts/language.md](docs/concepts/language.md).

`SupportsFileGlobs` returns `false` unless nodes may name individual files (only TypeScript and Dart do today).

## Contract format

See [docs/concepts/contract.md](docs/concepts/contract.md) for the format and [docs/manual.md](docs/manual.md) (`baft manual`) for the working model. Do not restate either here.

## Testing

`go test ./...` runs Go unit tests plus the godog suites: `internal/features/features_test.go` and the `*.feature` files sitting next to the code they cover. Filesystem-dependent tests mostly run against `internal/adapter/fs/memfs` and need no fixtures on disk; the ones that must exercise the real filesystem — `internal/cli`, `realfs`, `ignorefs`, `walk`, `python`, `integrations` — build their trees in `t.TempDir()`.

`internal/cli/docs_test.go` guards the hand-written docs against the code: every language in `languageNames` must appear in all three CLI assets, and every rule id in `internal/application/usecase/check` must appear in `docs/manual.md`.

## Pitfalls worth knowing

**Kotlin multiplatform has many source sets.** Not just `src/main/kotlin`: `commonMain`, `jvmMain`, `androidMain`, `iosMain`, `darwinMain`, `jsMain`, `nativeMain`, and their `*Test` counterparts. `IsScannableFile` and `findBaseCapsule` must recognize all of them.

**Generated files need explicit exclusion.** `/generated/`, `/kapt/`, `/ksp/`, `/buildSrc/` appear inside source trees and produce false positives if `IsScannableFile` does not filter them.

**Dot-separated namespaces have shared helpers.** `internal/adapter/languages/internal/namespaces` owns the two traps every dot-namespaced adapter (C#, JVM, Python) used to get wrong: `IsInternal` (`strings.HasPrefix("com.example2", "com.example")` is true, so the next character must be `.`) and `CommonPrefix` (the capsule ID is the longest shared *segment* prefix, or `app` swallows `application`). Call them; do not re-implement them.

**Optional suffixes must sit outside the capture group.** `import\s+([A-Za-z_.\*]+)` captures the `.*` wildcard into the package name; keep it outside the group or strip it after capture.

## Rules

- Every letter in this repository is a liability. Prefer deleting code to adding it.
- Default to **no comments**. If the reason for code is non-obvious, rename or refactor instead. One short line max when a comment is warranted, and it explains **why**.
- Fix every broken test you encounter.

## Releasing

Semantic versioning with `v`-prefixed Git tags.

```bash
./scripts/ci.sh
git tag -s v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

Then create the GitHub release against the new tag and verify:

```bash
go install github.com/dariushalipour/baft@v0.1.0
baft --version
```

`main.version` is set by `-ldflags` at build time and falls back to the module build info, then to `dev`. For a local build that reports a real version:

```bash
go build -ldflags "-X main.version=v0.1.0" -o baft .
```

### Plugin artifacts

`baft integrate` installs the plugin artifacts embedded under `internal/integrations/embedded/`, and the compatibility check compares versions exactly. Nothing verifies that the embedded blobs match the plugin sources, so the order matters:

1. Bump the version in its source of truth — `vscode-extension/package.json` or `intellij-plugin/gradle.properties`. Never hand-edit version strings in the other JetBrains files.
2. `./scripts/build-plugins.sh` to rebuild and re-embed the artifacts.
3. `go build -o baft .` so the new artifacts are compiled in.
4. `baft integrate` to verify.

Skip step 2 and the CLI keeps shipping the previously embedded version, which users see as a mismatch prompt.
