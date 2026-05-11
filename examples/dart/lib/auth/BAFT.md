<!-- BAFT — Architecture Contract: auth bounded context -->
<!-- auth/** is governed by this file. -->
<!-- Cross-context edges from billing or api are governed by lib/BAFT.md -->

```mermaid
flowchart TD
  domain["domain/**"]
  usecase["usecase/**"]
  usecase --> domain
```
