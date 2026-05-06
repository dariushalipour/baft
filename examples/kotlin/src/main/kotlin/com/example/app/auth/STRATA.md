<!-- STRATA — Architecture Contract: auth bounded context -->
<!-- auth/** is governed by this file. -->
<!-- Cross-context edges from billing or api are governed by app/STRATA.md -->

```mermaid
flowchart TD
  domain["domain/**"]
  application["application/**"]
  application --> domain
```
