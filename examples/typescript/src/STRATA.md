<!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
<!-- Layers & allowed imports are both documentation and enforcement. -->
<!-- Run `strata` for the complete specification. -->
<!-- Install: `go install github.com/dariushalipour/strata@latest` -->
<!-- (ensure ~/go/bin is in your PATH) -->

```mermaid
flowchart TD
  api["api/**"]
  auth["auth/**"]
  billing["billing/**"]
  core["core/**"]
  notifications["notifications/**"]
  api --> billing
  api --> auth
  api --> core
  billing --> auth
  billing --> notifications
  billing --> core
  notifications --> auth
  notifications --> core
```
