<!-- BAFT — Architecture Contract: Human-maintained. Edit this file to change rules. -->
<!-- Layers & allowed imports are both documentation and enforcement. -->
<!-- Run `baft` for the complete specification. -->
<!-- Install: `go install github.com/dariushalipour/baft@latest` -->
<!-- (ensure ~/go/bin is in your PATH) -->

```mermaid
flowchart TD
  api["internal/api/**"]
  auth["internal/auth/**"]
  billing["internal/billing/**"]
  cmd["internal/cmd/**"]
  domain["internal/domain/**"]
  cmd --> api
  cmd --> domain
  api --> billing
  api --> auth
  api --> domain
  billing --> auth
  billing --> domain
```
