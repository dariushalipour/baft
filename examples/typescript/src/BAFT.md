<!-- BAFT — Architecture Contract: Human-maintained. Edit this file to change rules. -->
<!-- AI agents and developers working in this codebase: if BAFT is unfamiliar, run `baft manual` to study the contract format and rules. -->
<!-- Layers & allowed imports are both documentation and enforcement. -->
<!-- Run `baft` for the complete specification. -->
<!-- Install: `go install github.com/dariushalipour/baft@latest` -->
<!-- (ensure ~/go/bin is in your PATH) -->

```mermaid
flowchart TD
  api["api/&ast;&ast;"]
  auth["auth/&ast;&ast;"]
  billing["billing/&ast;&ast;"]
  core["core/&ast;&ast;"]
  notifications["notifications/&ast;&ast;"]
  api --> billing
  api --> auth
  api --> core
  billing --> auth
  billing --> notifications
  billing --> core
  notifications --> auth
  notifications --> core
```
