<!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
<!-- Layers & allowed imports are both documentation and enforcement. -->
<!-- Run `strata` for the complete specification. -->
<!-- Install: `go install github.com/dariushalipour/strata@latest` -->
<!-- (ensure ~/go/bin is in your PATH) -->

```mermaid
flowchart TD
  api["src/main/kotlin/com/example/app/api/**"]
  application["src/main/kotlin/com/example/app/application/**"]
  domain["src/main/kotlin/com/example/app/domain/**"]
  shared["src/main/kotlin/com/example/app/shared/**"]
  api --> application
  api --> shared
  application --> domain
  application --> shared
  shared --> domain
```
