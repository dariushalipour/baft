<!-- BAFT — Architecture Contract: billing bounded context -->
<!-- billing/** is governed by this file. -->
<!-- Cross-context edges from api or notifications are governed by app/BAFT.md -->

```mermaid
flowchart TD
  api["api/**"]
  domain["domain/**"]
  application["application/**"]
  api --> application
  application --> domain
```
