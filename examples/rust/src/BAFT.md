<!-- BAFT — Architecture Contract: Human-maintained. Edit this file to change rules. -->
<!-- Layers & allowed imports are both documentation and enforcement. -->
<!-- Run `baft` for the complete specification. -->
<!-- Install: `go install github.com/dariushalipour/baft@latest` -->
<!-- (ensure ~/go/bin is in your PATH) -->

```mermaid
flowchart TD
  auth["src/auth/**"]
  billing["src/billing/**"]
  shared["src/shared/**"]
  billing --> auth
  billing --> shared
```
