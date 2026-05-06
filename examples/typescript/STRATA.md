<!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
<!-- Layers & allowed imports are both documentation and enforcement. -->
<!-- Run `strata` for the complete specification. -->
<!-- Install: `go install github.com/dariushalipour/strata@latest` -->
<!-- (ensure ~/go/bin is in your PATH) -->

```mermaid
flowchart TD
  api["src/api/**"]
  auth["src/auth/**"]
  core["src/core/**"]
  domain["src/domain/**"]
  shared["src/shared/**"]
  usecase["src/usecase/**"]
  api --> usecase
  api --> auth
  api --> core
  api --> shared
  auth --> core
  auth --> domain
  usecase --> domain
  usecase --> core
  core --> domain
  shared --> domain
```
