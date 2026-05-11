<!-- BAFT — Architecture Contract: billing bounded context -->
<!-- billing/** is governed by this file. -->
<!-- Cross-context edges from api or notifications are governed by lib/BAFT.md -->

```mermaid
flowchart TD
  api["api/**"]
  domain["domain/**"]
  usecase["usecase/**"]
  api --> usecase
  usecase --> domain
```
