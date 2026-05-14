<!-- BAFT architecture contract: edit nodes and edges to change allowed imports. -->
<!-- If BAFT is new to you, run `baft manual`. -->
<!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
<!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->

```mermaid
flowchart TD
  api["src/main/kotlin/com/example/app/api/&ast;&ast;"]
  application["src/main/kotlin/com/example/app/application/&ast;&ast;"]
  domain["src/main/kotlin/com/example/app/domain/&ast;&ast;"]
  shared["src/main/kotlin/com/example/app/shared/&ast;&ast;"]

  api --> application
  api --> shared
  application --> domain
  application --> shared
  shared --> domain

  style api stroke:#0f4cde,stroke-width:2px
  style application stroke:#c43d18,stroke-width:2px
  style domain stroke:#007e5f,stroke-width:2px
  style shared stroke:#8f2bd1,stroke-width:2px
  linkStyle 0 stroke:#0f4cde,stroke-width:2px
  linkStyle 1 stroke:#0f4cde,stroke-width:2px
  linkStyle 2 stroke:#c43d18,stroke-width:2px
  linkStyle 3 stroke:#c43d18,stroke-width:2px
  linkStyle 4 stroke:#8f2bd1,stroke-width:2px
```
