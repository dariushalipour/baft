<!-- BAFT — Architecture Contract: auth bounded context -->
<!-- auth/** is governed by this file. -->
<!-- Cross-context edges from billing or api are governed by src/BAFT.md -->

```mermaid
flowchart TD
  domain["src/auth/domain/**"]
  service["src/auth/service/**"]
  service --> domain
```
