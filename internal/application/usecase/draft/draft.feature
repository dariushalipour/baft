Feature: Draft STRATA.md from actual imports
  As a developer
  I want strata to generate a STRATA.md that reflects my real import graph
  So that I have an accurate starting point for my architecture rules

  Scenario: No capsules discovered yields an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    When the draft runs from "/Users/jane/strata"
    Then the draft errors

  Scenario: Empty capsule is skipped and other capsules are drafted
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ empty/
      │  ├─ go.mod
      │  └─ README.md
      ├─ services/
      │  ├─ go.mod
      │  └─ cmd/
      │     └─ main.go
      └─ libs/
         ├─ go.mod
         └─ domain/
            └─ model.go
      """
    Given file "empty/go.mod" has content "module example.com/empty"
    Given file "empty/README.md" has content "docs only"
    Given file "services/go.mod" has content "module example.com/services"
    Given file "services/cmd/main.go" has content:
      """go
      package main
      
      import "example.com/libs/domain"
      
      func main() { _ = domain.User{} }
      """
    Given file "libs/go.mod" has content "module example.com/libs"
    Given file "libs/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given the draft uses the "go" language adapter
    When the draft runs from "/Users/jane/strata"
    Then the draft succeeds
    And 2 capsules are drafted
    And 1 error is reported
    And the error is:
      """
        /Users/jane/strata/empty: capsule at /Users/jane/strata/empty has no governed files to draft
      """
    And "services/STRATA.md" is expected to have content:
      """config
      <!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
      <!-- Layers & allowed imports are both documentation and enforcement. -->
      <!-- Run `strata` for the complete specification. -->
      <!-- Install: `go install github.com/dariushalipour/strata@latest` -->
      <!-- (ensure ~/go/bin is in your PATH) -->
      
      ```mermaid
      flowchart TD
        cmd["cmd/&ast;&ast;"]
      
      ```
      """
    And "libs/STRATA.md" is expected to have content:
      """config
      <!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
      <!-- Layers & allowed imports are both documentation and enforcement. -->
      <!-- Run `strata` for the complete specification. -->
      <!-- Install: `go install github.com/dariushalipour/strata@latest` -->
      <!-- (ensure ~/go/bin is in your PATH) -->
      
      ```mermaid
      flowchart TD
        domain["domain/&ast;&ast;"]
      
      ```
      """
    And "empty/STRATA.md" should not exist
    And "STRATA.md" should not exist

  Scenario: Draft analyzes Go project imports and writes STRATA.md
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ internal/
      │  ├─ domain/
      │  │  └─ model.go
      │  └─ usecase/
      │     └─ create.go
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/test"
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      
      import "fmt"
      
      type User struct {
        Name string
      }
      
      func init() { _ = fmt.Println("domain") }
      """
    Given file "internal/usecase/create.go" has content:
      """go
      package usecase
      
      import (
        "example.com/test/internal/domain"
      )
      
      func Create() domain.User {
        return domain.User{}
      }
      """
    Given file "main.go" has content:
      """go
      package main
      
      import (
        "example.com/test/internal/usecase"
      )
      
      func main() {
        usecase.Create()
      }
      """
    Given the draft uses the "go" language adapter
    When the draft runs from "/Users/jane/strata"
    Then the draft succeeds
    And 1 capsule is drafted
    And capsule 1 has 3 files scanned
    And capsule 1 has 3 nodes
    And capsule 1 has 2 edges
    And "STRATA.md" is expected to have content:
      """config
      <!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
      <!-- Layers & allowed imports are both documentation and enforcement. -->
      <!-- Run `strata` for the complete specification. -->
      <!-- Install: `go install github.com/dariushalipour/strata@latest` -->
      <!-- (ensure ~/go/bin is in your PATH) -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain/&ast;&ast;"]
        internal_slash_usecase["internal/usecase/&ast;&ast;"]
      
        root --> internal_slash_usecase
        internal_slash_usecase --> internal_slash_domain
      ```
      """

  Scenario: Draft skips capsule with existing STRATA.md
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/skip"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        old["."]
      ```
      """
    Given file "main.go" has content:
      """go
      package main
      func main() {}
      """
    Given the draft uses the "go" language adapter
    When the draft runs from "/Users/jane/strata"
    Then the draft succeeds
    And 0 capsules are drafted
    And STRATA.md is unchanged

  Scenario: Draft partially skips nested package with existing STRATA.md
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ internal/
      │  ├─ domain/
      │  │  └─ model.go
      │  └─ nested/
      │     ├─ STRATA.md
      │     └─ api/
      │        └─ handler.go
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/nested"
    Given file "internal/nested/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["internal/nested/api/**"]
      ```
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/nested/api/handler.go" has content:
      """go
      package api
      """
    Given file "main.go" has content:
      """go
      package main
      
      import "example.com/nested/internal/domain"
      
      func main() { _ = domain.User{} }
      """
    Given the draft uses the "go" language adapter
    When the draft runs from "/Users/jane/strata"
    Then the draft succeeds
    And 1 capsule is drafted
    And capsule 1 has 2 files scanned
    And "STRATA.md" is expected to have content:
      """config
      <!-- STRATA — Architecture Contract: Human-maintained. Edit this file to change rules. -->
      <!-- Layers & allowed imports are both documentation and enforcement. -->
      <!-- Run `strata` for the complete specification. -->
      <!-- Install: `go install github.com/dariushalipour/strata@latest` -->
      <!-- (ensure ~/go/bin is in your PATH) -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain/&ast;&ast;"]
      
        root --> internal_slash_domain
      ```
      """

  Scenario: Draft does not double-scan files in nested skipped package
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      └─ internal/
         └─ a/
            ├─ top.go
            └─ b/
               ├─ STRATA.md
               └─ deep.go
      """
    Given file "go.mod" has content "module example.com/double"
    Given file "internal/a/b/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        x["."]
      ```
      """
    Given file "internal/a/top.go" has content:
      """go
      package a
      """
    Given file "internal/a/b/deep.go" has content:
      """go
      package b
      """
    Given the draft uses the "go" language adapter
    When the draft runs from "/Users/jane/strata"
    Then the draft succeeds
    And 1 capsule is drafted
    And capsule 1 has 1 file scanned

  Scenario: Draft ignores missing files gracefully
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ internal/
      │  ├─ domain/
      │  │  └─ model.go
      │  └─ usecase/
      │     └─ generated.go
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/test"
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/usecase/generated.go" has content:
      """go
      package usecase
      """
    Given file "main.go" has content:
      """go
      package main
      """
    Given the "go" language adapter cannot read "internal/usecase/generated.go"
    Given the draft uses the "go" language adapter
    When the draft runs from "/Users/jane/strata"
    Then the draft succeeds
    And 1 capsule is drafted
    And capsule 1 has 2 files scanned
