<!-- BAFT — Architecture Contract: auth bounded context -->
<!-- AI agents and developers working in this codebase: if BAFT is unfamiliar, run `baft manual` to study the contract format and rules. -->
<!-- auth/** is governed by this file. -->
<!-- Cross-context edges from billing or api are governed by internal/BAFT.md -->

```mermaid
flowchart TD
  domain["internal/auth/domain/**"]
  usecase["internal/auth/usecase/**"]
  usecase --> domain
```
