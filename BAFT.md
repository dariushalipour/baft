<!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
<!-- If Baft is new to you, run `baft manual`. -->
<!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
<!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->

```mermaid
flowchart TD
  root["."]
  cli["internal/cli/&ast;&ast;"]
  application["internal/application/&ast;&ast;"]
  adapter["internal/adapter/&ast;&ast;"]
  integrations["internal/integrations/&ast;&ast;"]
  port["internal/port/&ast;&ast;"]
  domain["internal/domain/&ast;&ast;"]
  treeview["internal/treeview/&ast;&ast;"]
  vscode_extension["src"]
  intellij_plugin["src/main/kotlin/com/baft/intellij"]

  root --> cli
  cli --> adapter
  cli --> application
  cli --> integrations
  cli --> port
  application --> integrations
  application --> port
  application --> domain
  adapter --> port
  adapter --> domain
  port --> domain
```
