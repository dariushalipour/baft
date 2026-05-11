<!-- BAFT — Architecture Contract: billing bounded context -->
<!-- AI agents and developers working in this codebase: if BAFT is unfamiliar, run `baft manual` to study the contract format and rules. -->
<!-- billing/** is governed by this file. -->
<!-- Cross-context edges from api or notifications are governed by src/BAFT.md -->

```mermaid
flowchart TD
  api["src/billing/api/**"]
  domain["src/billing/domain/**"]
  service["src/billing/service/**"]
  api --> service
  service --> domain
```
