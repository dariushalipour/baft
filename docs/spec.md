STRATA - Architecture Contract Spec
==================================================

STRATA enforces architecture rules per-capsule. Each capsule declares nodes + allowed imports in a `STRATA.md` (mermaid flowchart). The tool verifies all internal imports respect the declared node graph. STRATA.md is both docs and enforcement.

STRATA.md FORMAT
----------------
Single fenced mermaid flowchart block. Everything outside is ignored.

    # STRATA - Architecture Contract
    ```mermaid
    flowchart TD
      domain["domain/**"]
      usecase["usecase/**"]:::endophobic
      infra["infra/**"]

      usecase --> domain
      infra --> usecase
      infra --> domain
    ```

Rules: (1) exactly one ```mermaid block; (2) %% comments inside block are ignored; (3) NO subgraph syntax; (4) write `&ast;` for `*` in labels (parser converts back).

NODES
-----
Syntax: `nodeId["path/**"]` (directory glob) or `nodeId["path/file.go"]` (single file). Node IDs are arbitrary unique identifiers.

- Directory glob claims every governed source file under that dir.
- File glob claims one file.
- Every governed file must match at least one node — unmatched files are violations.

Go, Kotlin, Rust: directory-level nodes only. Same-dir files needing different rules must be moved.
TypeScript, Dart: file-level nodes allowed for finer-grained rules.

EDGES
-----
Syntax: `nodeA --> nodeB` — files in A may import files in B.
- Implicit self-edge: a file may always import within its own node.
- No edge A --> B means cross-node imports are violations.
- Chained edges (A --> B --> C) do NOT imply A may import C directly.

WORKFLOW
--------
1. `strata draft /repo` — scans source files, maps imports, writes STRATA.md per capsule (skips existing). For nested capsules, parent draft includes child directories as nodes with observed cross-directory edges. **WARNING: the auto-generated STRATA.md is never ready for use.** It produces a flat, unabstracted graph of every actual file-level (if Dart/Typescript, otherwise dir-level) relationship. It is only the first unacceptable draft — you must manually abstract nodes and prune edges to declare your intended architecture.
2. `strata check /repo` — reads STRATA.md files, verifies all imports respect edges. Each STRATA.md is evaluated in its own scope (see NESTED CAPSULES).
3. Edit STRATA.md manually, then `strata check` to verify.
4. To re-bootstrap: delete STRATA.md, run `strata draft` again.

VIOLATIONS
----------
- `<file> is governed but matches no node` — add a node.
- `<file> imports <target> - target matches no node` — target undeclared.
- `<file> imports <target> - A -> B not allowed` — add edge or move file.
- `<file> imports <target> - cross-directory edge not declared in parent` — add edge in parent STRATA.md.

Exit: 0 = clean, 1 = violations/error.

NESTED CAPSULES
---------------
A child dir with its own STRATA.md is an independent bounded context. The parent and child each govern different scopes:

**Child scope:**
- Child STRATA.md only evaluates imports where both source AND target are within the child directory.
- Cross-directory imports from a child file are treated as third-party — the child STRATA.md ignores them entirely.
- Sibling bounded contexts cannot be referenced by child STRATA.md (no `../sibling/**` globs).

**Parent scope:**
- Parent STRATA.md can reference child directories as nodes (e.g., `auth["auth/**"]`).
- Parent governs cross-context edges between children (e.g., `billing --> auth`).
- Parent does not scan files inside child directories for "unmatched file" violations — those are the child's responsibility.
- If a child file imports a sibling-context file and the parent has no edge between them, the parent STRATA.md reports the violation.

**Example:**
```
services/
├── STRATA.md              ← billing --> auth, billing --> shared
├── auth/
│   └── STRATA.md          ← app --> domain (auth internals only)
├── billing/
│   └── STRATA.md          ← app --> domain (billing internals only)
```

When `billing/app/x.go` imports `auth/domain/y.go`:
- Parent checks: `billing → auth` allowed? If not → parent violation.
- Child ignores: target is outside child scope.

JSON OUTPUT
-----------
`strata check --reporter=json /repo` — per-capsule results, violation counts, file lists.

SUPPORTED LANGUAGES
-------------------
Go, TypeScript, Kotlin, Rust, Dart. Auto-discovered via module markers (go.mod, package.json, build.gradle.kts, Cargo.toml, pubspec.yaml). Governed extensions are language-specific.

LLM TIPS
--------
- Check STRATA.md edges before generating cross-node imports.
- New files must fall under a declared node glob.
- Run `strata check` after changes.
- No subgraph syntax. Use `&ast;` for `*` in globs.
