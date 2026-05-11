BAFT.md Manual for Working in BAFT-Governed Code
================================================

If you opened a `BAFT.md` file and need to know what it means or how to work with code governed by it, start here.

Local command: `baft manual`

`BAFT.md` is an executable architecture contract. It tells Baft which files belong to which node and which nodes may import which other nodes. The file is both documentation and enforcement.

AGENT FAST PATH
---------------
When working in a repository that contains `BAFT.md`:

1. Find the nearest `BAFT.md` that governs the file you are editing.
2. Map the source file to a node by matching its path against node globs.
3. Map each internal import target to a node.
4. The import is allowed only if:
   - source and target are in the same node and the source node is not `endophobic`; or
   - the contract has an explicit edge `source --> target`.
5. If the dependency is not allowed, do one of these instead:
   - move the file to a node that already has the required dependency
   - add the missing edge if that is the intended architecture
   - refactor to depend on an allowed intermediary
6. Run `baft check` after changes.

WHAT A BAFT.md FILE MEANS
-------------------------
- One `BAFT.md` governs one capsule: a supported project root plus its architecture contract.
- Nodes claim files with globs.
- Arrows declare allowed cross-node imports.
- Same-node imports are allowed by default.
- `:::endophobic` disables same-node imports for that node.
- Every governed file must match at least one node.

Everything outside the single fenced `mermaid` block is ignored by Baft. That means comments before the block are safe and may contain guidance for humans or AI agents.

MINIMAL EXAMPLE
---------------

    ```mermaid
    flowchart TD
      api["internal/api/**"]
      usecase["internal/usecase/**"]:::endophobic
      domain["internal/domain/**"]
      infra["internal/infra/**"]

      api --> usecase
      usecase --> domain
      usecase --> infra
      infra --> domain
    ```

This means:
- files in `api` may import `usecase`
- files in `usecase` may import `domain` and `infra`
- files in `infra` may import `domain`
- files in `usecase` may not import other files in `usecase`

FORMAT
------
- Exactly one fenced `mermaid` flowchart block is allowed.
- Everything outside that block is ignored.
- `%%` comments inside the Mermaid block are ignored.
- `subgraph` syntax is not supported.
- Write `&ast;` inside labels when you need a literal `*`; Baft decodes it back to `*`.

NODES
-----
Syntax: `nodeId["path/**"]` for a directory-shaped node or `nodeId["path/file.go"]` for a file-shaped node.

- Node IDs are arbitrary unique identifiers.
- Directory globs claim every governed source file under that directory.
- File-shaped globs claim exactly one file.
- Every governed file must match at least one node. Unmatched files are violations.
- Most specific match wins.
- File-shaped globs beat directory-shaped globs.

Language limits:
- Go, Kotlin, Rust: directory-shaped nodes only
- TypeScript, Dart: file-shaped nodes are supported

EDGES AND ENDOPHOBIC NODES
--------------------------
Syntax: `nodeA --> nodeB`.

- `A --> B` means files in `A` may import files in `B`.
- No edge means the cross-node import is forbidden.
- Chained edges are not transitive. `A --> B --> C` does not imply `A --> C`.
- Same-node imports are allowed by default.
- `:::endophobic` removes that implicit self-edge for the marked node.

WORKFLOW
--------
1. Write `BAFT.md` by hand, or bootstrap with `baft draft /repo`.
2. Treat generated drafts as raw material, not final architecture. They reflect current dependency reality, not the architecture you want.
3. Edit nodes and edges until the contract matches the intended design.
4. Run `baft check /repo`.
5. Fix violations by moving code, removing bad dependencies, or deliberately updating the contract.

NESTED CAPSULES
---------------
A child directory with its own `BAFT.md` is an independent bounded context. Parent and child contracts govern different scopes.

Child scope:
- A child `BAFT.md` only evaluates imports where both source and target are inside the child directory.
- Imports from a child file to a target outside the child are treated as external by the child contract.
- A child contract may not reference sibling directories with globs like `../sibling/**`.

Parent scope:
- The parent `BAFT.md` may reference child directories as nodes, for example `auth["auth/**"]`.
- The parent governs cross-context edges between children, for example `billing --> auth`.
- The parent does not scan files inside children for unmatched-file violations. That is the child's job.
- If a child file imports a sibling-context file and the parent has no edge for that relation, the parent reports the violation.

Example:

    services/
    |- BAFT.md              <- billing --> auth, billing --> shared
    |- auth/
    |  \- BAFT.md          <- app --> domain (auth internals only)
    \- billing/
       \- BAFT.md          <- app --> domain (billing internals only)

If `billing/app/x.go` imports `auth/domain/y.go`:
- the parent contract decides whether `billing --> auth` is allowed
- the child `billing/BAFT.md` ignores that import because the target is outside the child scope

VIOLATIONS
----------
Common failure modes:
- `<file> is governed but matches no node`
- `<file> imports <target> - target matches no node`
- `<file> imports <target> - A -> B not allowed`
- `<file> imports <target> - cross-directory edge not declared in parent`

Exit codes:
- `0` = clean
- `1` = violations or error

SUPPORTED LANGUAGES
-------------------
Baft currently supports Go, TypeScript, Kotlin, Rust, and Dart. Capsules are auto-discovered from standard manifests such as `go.mod`, `package.json`, `build.gradle.kts`, `Cargo.toml`, and `pubspec.yaml`.

AGENT GUARDRAILS
----------------
- Do not add a cross-node import just because it compiles. Check the contract first.
- Do not create a new file without making sure some node claims it.
- In nested capsules, do not try to authorize sibling imports from the child contract. That belongs in the parent contract.
- If you are changing the intended architecture, update `BAFT.md` in the same change as the code.
- Run `baft check` before finishing work.