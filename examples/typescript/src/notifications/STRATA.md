<!-- STRATA — Architecture Contract: notifications bounded context -->
<!-- notifications/** is governed by this file. -->
<!-- Cross-context edges from billing or api are governed by src/STRATA.md -->

```mermaid
flowchart TD
  domain["domain/**"]
  usecase["usecase/**"]
  usecase --> domain
```
