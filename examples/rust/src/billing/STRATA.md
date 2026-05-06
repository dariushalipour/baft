<!-- STRATA — Architecture Contract: billing bounded context -->
<!-- billing/** is governed by this file. -->
<!-- Cross-context edges from api or notifications are governed by src/STRATA.md -->

```mermaid
flowchart TD
  api["src/billing/api/**"]
  domain["src/billing/domain/**"]
  service["src/billing/service/**"]
  api --> service
  service --> domain
```
