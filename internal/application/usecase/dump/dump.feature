Feature: Dump BAFT.md from actual imports
  As a developer
  I want baft to generate a BAFT.md that reflects my real import graph
  So that I have an accurate starting point for my architecture rules

  Scenario: No capsules discovered yields an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    When the dump runs from "/Users/jane/baft"
    Then the dump errors

  Scenario: Empty contract candidate is reported and other contracts are dumped
    Given a fresh workspace at "/Users/jane/baft" with this layout:
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
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    Then 1 errors and 0 violations are reported
    And the error is:
      """
        /Users/jane/baft/empty: capsule at /Users/jane/baft/empty has no scannable files to dump
      """
    And Contract at "services/BAFT.md" has 1 node and 0 edges
    And Contract at "services/BAFT.md" is new
    And Contract at "libs/BAFT.md" has 1 node and 0 edges
    And Contract at "libs/BAFT.md" is new
    And file "services/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        cmd["cmd"]
      
      ```
      """
    And file "libs/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        domain["domain"]
      
      ```
      """
    And file "empty/BAFT.md" should not exist
    And file "BAFT.md" should not exist

  Scenario: Dump analyzes Go project imports and writes BAFT.md
    Given a fresh workspace at "/Users/jane/baft" with this layout:
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
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 3 nodes and 2 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain"]
        internal_slash_usecase["internal/usecase"]
      
        root --> internal_slash_usecase
        internal_slash_usecase --> internal_slash_domain
      ```
      """

  Scenario: Dump writes BAFT.md even when the actual imports form a cycle
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ internal/
         ├─ application/
         │  └─ service.go
         └─ domain/
            └─ model.go
      """
    Given file "go.mod" has content "module example.com/cycle"
    Given file "internal/application/service.go" has content:
      """go
      package application
      
      import "example.com/cycle/internal/domain"
      
      func Service() domain.Model { return domain.Model{} }
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      
      import "example.com/cycle/internal/application"
      
      type Model struct{}
      
      func New() application.Service { return application.Service{} }
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 2 nodes and 2 edges
    And Contract at "BAFT.md" is new
    Then 1 capsule is dumped
    And file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        internal_slash_application["internal/application"]
        internal_slash_domain["internal/domain"]
      
        internal_slash_application --> internal_slash_domain
        internal_slash_domain --> internal_slash_application
      ```
      """

  Scenario: Dump reports no untracked contracts when all already have BAFT.md
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/complete"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        root["."]
      ```
      """
    Given file "main.go" has content:
      """go
      package main
      func main() {}
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    Then file "BAFT.md" should stay the same

  Scenario: Dump respects existing inner contract before dumping outer contract
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ internal/
      │  ├─ domain/
      │  │  └─ model.go
      │  └─ nested/
      │     ├─ BAFT.md
      │     └─ api/
      │        └─ handler.go
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/nested"
    Given file "internal/nested/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["internal/nested/api"]
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
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain"]
      
        root --> internal_slash_domain
      ```
      """
    And file "internal/nested/BAFT.md" should stay the same

  Scenario: Dump does not scan files owned by an existing inner contract
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ internal/
         └─ a/
            ├─ top.go
            └─ b/
               ├─ BAFT.md
               └─ deep.go
      """
    Given file "go.mod" has content "module example.com/double"
    Given file "internal/a/b/BAFT.md" has content:
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
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 1 nodes and 0 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        internal_slash_a["internal/a"]
      
      ```
      """
    And file "internal/a/b/BAFT.md" should stay the same

  Scenario: Dump fills gaps - adds missing nodes to existing BAFT.md
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ domain/
         │  └─ model.go
         └─ usecase/
            └─ create.go
      """
    Given file "go.mod" has content "module example.com/gaps"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        internal_slash_domain["internal/domain"]
      ```
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/usecase/create.go" has content:
      """go
      package usecase
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" is an amendment
    And Contract at "BAFT.md" added 1 nodes and 0 edges
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        internal_slash_domain["internal/domain"]
        internal_slash_usecase["internal/usecase"]
      
      ```
      """

  Scenario: Dump fills gaps - reuses custom node IDs when adding missing edges
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ domain/
         │  └─ model.go
         └─ usecase/
            └─ create.go
      """
    Given file "go.mod" has content "module example.com/gaps"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["internal/domain"]
        usecases["internal/usecase"]
      ```
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/usecase/create.go" has content:
      """go
      package usecase
      
      import "example.com/gaps/internal/domain"
      
      func Create() domain.User { return domain.User{} }
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" is an amendment
    And Contract at "BAFT.md" added 0 nodes and 1 edges
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        domain["internal/domain"]
        usecases["internal/usecase"]
      
        usecases --> domain
      ```
      """

  Scenario: Dump fills gaps - preserves bare directory globs on existing nodes
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ dirx/
      │  └─ x.go
      └─ diry/
         └─ y.go
      """
    Given file "go.mod" has content "module example.com/gaps"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        dirx["dirx"]
      ```
      """
    Given file "dirx/x.go" has content:
      """go
      package dirx
      
      type X struct{}
      """
    Given file "diry/y.go" has content:
      """go
      package diry
      
      type Y struct{}
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" is an amendment
    And Contract at "BAFT.md" added 1 nodes and 0 edges
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        dirx["dirx"]
        diry["diry"]
      
      ```
      """

  Scenario: Dump fills gaps - preserves endophobic modifiers on existing nodes
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ domain/
         │  └─ model.go
         └─ usecase/
            └─ create.go
      """
    Given file "go.mod" has content "module example.com/gaps"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecases["internal/usecase"]:::endophobic
      ```
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/usecase/create.go" has content:
      """go
      package usecase
      
      import "example.com/gaps/internal/domain"
      
      func Create() domain.User { return domain.User{} }
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" is an amendment
    And Contract at "BAFT.md" added 1 nodes and 1 edges
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        internal_slash_domain["internal/domain"]
        usecases["internal/usecase"]:::endophobic
      
        usecases --> internal_slash_domain
      
        %% ------------------------------------------------------------------------------------------
        %% AUTO-GENERATED STYLING: Do not edit manually.
        %% If you add, delete, or reorder nodes, you MUST run 'baft restyle' or format via your IDE.
        %% Outdated references will either break the render entirely or silently mess up the styling.
        %% ------------------------------------------------------------------------------------------
        style usecases stroke-width:2px,stroke-dasharray:5 5
      ```
      """

  Scenario: Dump mixed - some contracts get new BAFT.md, existing ones get gap-filled
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ services/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ cmd/
      │  │  └─ main.go
      │  └─ internal/
      │     └─ app/
      │        └─ app.go
      └─ libs/
         ├─ go.mod
         └─ domain/
            └─ model.go
      """
    Given file "services/go.mod" has content "module example.com/services"
    Given file "services/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        cmd["cmd"]
      ```
      """
    Given file "services/cmd/main.go" has content:
      """go
      package main
      func main() {}
      """
    Given file "services/internal/app/app.go" has content:
      """go
      package app
      """
    Given file "libs/go.mod" has content "module example.com/libs"
    Given file "libs/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "services/BAFT.md" is an amendment
    And Contract at "services/BAFT.md" added 1 nodes and 0 edges
    And Contract at "libs/BAFT.md" is new
    And Contract at "libs/BAFT.md" has 1 nodes and 0 edges
    Then file "services/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        cmd["cmd"]
        internal_slash_app["internal/app"]
      
      ```
      """
    And file "libs/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        domain["domain"]
      
      ```
      """

  Scenario: Dump inner capsules first - deepest contracts dumped before outer
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ main.go
      └─ internal/
         ├─ domain/
         │  └─ model.go
         └─ tool/
            ├─ go.mod
            └─ runner.go
      """
    Given file "go.mod" has content "module example.com/root"
    Given file "main.go" has content:
      """go
      package main
      
      import "example.com/root/internal/domain"
      
      func main() { _ = domain.User{} }
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/tool/go.mod" has content "module example.com/tool"
    Given file "internal/tool/runner.go" has content:
      """go
      package tool
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "internal/tool/BAFT.md" has 1 nodes and 0 edges
    And Contract at "internal/tool/BAFT.md" is new
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "internal/tool/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        root["."]
      
      ```
      """
    And file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain"]
      
        root --> internal_slash_domain
      ```
      """

  Scenario: Dump inner capsule first - inner gets gap-filled, outer gets new BAFT.md
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ main.go
      └─ internal/
         ├─ domain/
         │  └─ model.go
         └─ tool/
            ├─ go.mod
            ├─ BAFT.md
            ├─ api/
            │  └─ handler.go
            └─ domain/
               └─ model.go
      """
    Given file "go.mod" has content "module example.com/root"
    Given file "main.go" has content:
      """go
      package main
      
      import "example.com/root/internal/domain"
      
      func main() { _ = domain.User{} }
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given file "internal/tool/go.mod" has content "module example.com/tool"
    Given file "internal/tool/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api"]
      ```
      """
    Given file "internal/tool/api/handler.go" has content:
      """go
      package api
      
      import "example.com/tool/domain"
      
      func Handle() domain.Widget { return domain.Widget{} }
      """
    Given file "internal/tool/domain/model.go" has content:
      """go
      package domain
      type Widget struct{}
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "internal/tool/BAFT.md" is an amendment
    And Contract at "internal/tool/BAFT.md" added 1 nodes and 1 edges
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "internal/tool/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        api["api"]
        domain["domain"]
      
        api --> domain
      ```
      """
    And file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain"]
      
        root --> internal_slash_domain
      ```
      """

  Scenario: Dump skips and reports invalid existing BAFT.md without overwriting it
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ main.go
      """
    Given file "go.mod" has content "module example.com/invalid"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        root["."]
      """
    Given file "main.go" has content:
      """go
      package main
      func main() {}
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    Then the error is:
      """
      /Users/jane/baft/BAFT.md: contract-load-error: unclosed ```mermaid block
      """
    And file "BAFT.md" should stay the same

  Scenario: Dump skips and reports existing BAFT.md with circular dependency
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  └─ service.go
         └─ domain/
            └─ model.go
      """
    Given file "go.mod" has content "module example.com/cycle"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
      
        app --> domain
        domain --> app
      ```
      """
    Given file "internal/application/service.go" has content:
      """go
      package application
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    Then the error is:
      """
      /Users/jane/baft/BAFT.md: circular-dependency: circular dependency
      """
    And file "BAFT.md" should stay the same

  Scenario: Dump ignores missing files gracefully
    Given a fresh workspace at "/Users/jane/baft" with this layout:
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
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 2 nodes and 0 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        root["."]
        internal_slash_domain["internal/domain"]
      
      ```
      """

  Scenario: Dump at TypeScript package root defaults to merged dir-level globs when there is no cycle
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ calculator.ts
      │  └─ invoice.ts
      ├─ shared/
      │  └─ utils.ts
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":".","paths":{"@myorg/shared/*":["shared/*"],"@myorg/billing/*":["billing/*"]}}}
      """
    Given file "shared/utils.ts" has content:
      """typescript
      export function formatDate(d: Date) { return d.toISOString() }
      """
    Given file "billing/calculator.ts" has content:
      """typescript
      export function calculateTax(amount: number) { return amount * 0.1 }
      """
    Given file "billing/invoice.ts" has content:
      """typescript
      import { formatDate } from "@myorg/shared/utils"
      import { calculateTax } from "./calculator"
      
      export function generateInvoice() {
        const tax = calculateTax(100)
        return formatDate(new Date()) + " tax:" + tax
      }
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        billing["billing/&ast;.&ast;"]
        shared_slash_utils_dot_ts["shared/utils.ts"]
      
        billing --> shared_slash_utils_dot_ts
      ```
      """

  Scenario: Dump at TypeScript package root reuses an existing file node when filling a missing edge from a merged dir node
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ api/
      │  ├─ entry.ts
      │  └─ helper.ts
      ├─ usecase/
      │  ├─ consumer.ts
      │  └─ helper.ts
      ├─ BAFT.md
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":"."}}
      """
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api/&ast;.&ast;"]
        usecase_slash_consumer_dot_ts["usecase/consumer.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
      ```
      """
    Given file "api/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given file "api/entry.ts" has content:
      """typescript
      import { consume } from "../usecase/consumer"
      
      export function run() {
        return consume()
      }
      """
    Given file "usecase/consumer.ts" has content:
      """typescript
      export function consume() {
        return "ok"
      }
      """
    Given file "usecase/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" is an amendment
    And Contract at "BAFT.md" added 0 nodes and 1 edges
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        api["api/&ast;.&ast;"]
        usecase_slash_consumer_dot_ts["usecase/consumer.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
      
        api --> usecase_slash_consumer_dot_ts
      ```
      """

  Scenario: Dump at TypeScript package root amends an existing contract by recreating a missing exact file node instead of widening it to a merged dir node
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ usecase/
      │  ├─ consumer.ts
      │  └─ helper.ts
      ├─ alpha.ts
      ├─ beta.ts
      ├─ gamma.ts
      ├─ delta.ts
      ├─ epsilon.ts
      ├─ zeta.ts
      ├─ eta.ts
      ├─ theta.ts
      ├─ BAFT.md
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":"."}}
      """
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        alpha_dot_ts["alpha.ts"]
        beta_dot_ts["beta.ts"]
        delta_dot_ts["delta.ts"]
        epsilon_dot_ts["epsilon.ts"]
        eta_dot_ts["eta.ts"]
        gamma_dot_ts["gamma.ts"]
        theta_dot_ts["theta.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
        zeta_dot_ts["zeta.ts"]
      ```
      """
    Given file "alpha.ts" has content:
      """typescript
      export const alpha = "alpha"
      """
    Given file "beta.ts" has content:
      """typescript
      export const beta = "beta"
      """
    Given file "gamma.ts" has content:
      """typescript
      export const gamma = "gamma"
      """
    Given file "delta.ts" has content:
      """typescript
      export const delta = "delta"
      """
    Given file "epsilon.ts" has content:
      """typescript
      export const epsilon = "epsilon"
      """
    Given file "zeta.ts" has content:
      """typescript
      export const zeta = "zeta"
      """
    Given file "eta.ts" has content:
      """typescript
      export const eta = "eta"
      """
    Given file "theta.ts" has content:
      """typescript
      export const theta = "theta"
      """
    Given file "usecase/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given file "usecase/consumer.ts" has content:
      """typescript
      import { alpha } from "../alpha"
      import { beta } from "../beta"
      import { gamma } from "../gamma"
      import { delta } from "../delta"
      import { epsilon } from "../epsilon"
      import { zeta } from "../zeta"
      import { eta } from "../eta"
      import { theta } from "../theta"
      
      export function consume() {
        return [alpha, beta, gamma, delta, epsilon, zeta, eta, theta].join(":")
      }
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" is an amendment
    And Contract at "BAFT.md" added 1 nodes and 8 edges
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        alpha_dot_ts["alpha.ts"]
        beta_dot_ts["beta.ts"]
        delta_dot_ts["delta.ts"]
        epsilon_dot_ts["epsilon.ts"]
        eta_dot_ts["eta.ts"]
        gamma_dot_ts["gamma.ts"]
        theta_dot_ts["theta.ts"]
        usecase_slash_consumer_dot_ts["usecase/consumer.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
        zeta_dot_ts["zeta.ts"]
      
        usecase_slash_consumer_dot_ts --> alpha_dot_ts
        usecase_slash_consumer_dot_ts --> beta_dot_ts
        usecase_slash_consumer_dot_ts --> delta_dot_ts
        usecase_slash_consumer_dot_ts --> epsilon_dot_ts
        usecase_slash_consumer_dot_ts --> eta_dot_ts
        usecase_slash_consumer_dot_ts --> gamma_dot_ts
        usecase_slash_consumer_dot_ts --> theta_dot_ts
        usecase_slash_consumer_dot_ts --> zeta_dot_ts
      ```
      """

  Scenario: Dump at TypeScript package root expands the smaller merged node first when that breaks the cycle
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ api/
      │  ├─ entry.ts
      │  └─ helper.ts
      ├─ usecase/
      │  ├─ consumer.ts
      │  ├─ producer.ts
      │  └─ helper.ts
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":"."}}
      """
    Given file "api/helper.ts" has content:
      """typescript
      export function helper() { return "ok" }
      """
    Given file "api/entry.ts" has content:
      """typescript
      import { consume } from "../usecase/consumer"
      
      export function run() {
        return consume()
      }
      """
    Given file "usecase/consumer.ts" has content:
      """typescript
      import { helper } from "../api/helper"
      
      export function consume() {
        return helper()
      }
      """
    Given file "usecase/producer.ts" has content:
      """typescript
      export const producerMarker = 1
      """
    Given file "usecase/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 3 nodes and 2 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        api_slash_entry_dot_ts["api/entry.ts"]
        api_slash_helper_dot_ts["api/helper.ts"]
        usecase["usecase/&ast;.&ast;"]
      
        api_slash_entry_dot_ts --> usecase
        usecase --> api_slash_helper_dot_ts
      ```
      """

  Scenario: Dump at TypeScript package root expands the larger merged node when the smaller one still cycles
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ api/
      │  ├─ entry.ts
      │  └─ helper.ts
      ├─ usecase/
      │  ├─ consumer.ts
      │  ├─ producer.ts
      │  └─ helper.ts
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":"."}}
      """
    Given file "api/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given file "api/entry.ts" has content:
      """typescript
      import { consume } from "../usecase/consumer"
      
      export function run() {
        return consume()
      }
      """
    Given file "usecase/consumer.ts" has content:
      """typescript
      export function consume() {
        return "ok"
      }
      """
    Given file "usecase/producer.ts" has content:
      """typescript
      import { run } from "../api/entry"
      
      export function produce() {
        return run()
      }
      """
    Given file "usecase/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 4 nodes and 2 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        api["api/&ast;.&ast;"]
        usecase_slash_consumer_dot_ts["usecase/consumer.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
        usecase_slash_producer_dot_ts["usecase/producer.ts"]
      
        api --> usecase_slash_consumer_dot_ts
        usecase_slash_producer_dot_ts --> api
      ```
      """

  Scenario: Dump at TypeScript package root expands both merged nodes when neither alone breaks the cycle
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ api/
      │  ├─ entry.ts
      │  └─ helper.ts
      ├─ usecase/
      │  ├─ consumer.ts
      │  ├─ producer.ts
      │  └─ helper.ts
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":"."}}
      """
    Given file "api/helper.ts" has content:
      """typescript
      export function helper() { return "ok" }
      """
    Given file "api/entry.ts" has content:
      """typescript
      import { consume } from "../usecase/consumer"
      
      export function run() {
        return consume()
      }
      """
    Given file "usecase/consumer.ts" has content:
      """typescript
      import { helper } from "../api/helper"
      
      export function consume() {
        return helper()
      }
      """
    Given file "usecase/producer.ts" has content:
      """typescript
      import { run } from "../api/entry"
      
      export function produce() {
        return run()
      }
      """
    Given file "usecase/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 5 nodes and 3 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        api_slash_entry_dot_ts["api/entry.ts"]
        api_slash_helper_dot_ts["api/helper.ts"]
        usecase_slash_consumer_dot_ts["usecase/consumer.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
        usecase_slash_producer_dot_ts["usecase/producer.ts"]
      
        api_slash_entry_dot_ts --> usecase_slash_consumer_dot_ts
        usecase_slash_consumer_dot_ts --> api_slash_helper_dot_ts
        usecase_slash_producer_dot_ts --> api_slash_entry_dot_ts
      ```
      """

  Scenario: Dump at TypeScript package root keeps the real cycle when expanding both merged nodes still cycles
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ api/
      │  ├─ entry.ts
      │  └─ helper.ts
      ├─ usecase/
      │  ├─ consumer.ts
      │  ├─ producer.ts
      │  └─ helper.ts
      ├─ package.json
      └─ tsconfig.json
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":"."}}
      """
    Given file "api/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given file "api/entry.ts" has content:
      """typescript
      import { consume } from "../usecase/consumer"
      
      export function run() {
        return consume()
      }
      """
    Given file "usecase/consumer.ts" has content:
      """typescript
      import { run } from "../api/entry"
      
      export function consume() {
        return run()
      }
      """
    Given file "usecase/producer.ts" has content:
      """typescript
      export const producerMarker = 1
      """
    Given file "usecase/helper.ts" has content:
      """typescript
      export const helperMarker = 1
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 5 nodes and 2 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        api_slash_entry_dot_ts["api/entry.ts"]
        api_slash_helper_dot_ts["api/helper.ts"]
        usecase_slash_consumer_dot_ts["usecase/consumer.ts"]
        usecase_slash_helper_dot_ts["usecase/helper.ts"]
        usecase_slash_producer_dot_ts["usecase/producer.ts"]
      
        api_slash_entry_dot_ts --> usecase_slash_consumer_dot_ts
        usecase_slash_consumer_dot_ts --> api_slash_entry_dot_ts
      ```
      """

  Scenario: Dump runs from a TypeScript subdirectory bounded context
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ calculator.ts
      │  └─ invoice.ts
      ├─ shared/
      │  └─ utils.ts
      ├─ package.json
      ├─ tsconfig.json
      └─ README.md
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "README.md" has content "# Monorepo"
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":".","paths":{"@myorg/shared/*":["shared/*"],"@myorg/billing/*":["billing/*"]}}}
      """
    Given file "shared/utils.ts" has content:
      """typescript
      export function formatDate(d: Date) { return d.toISOString() }
      """
    Given file "billing/calculator.ts" has content:
      """typescript
      export function calculateTax(amount: number) { return amount * 0.1 }
      """
    Given file "billing/invoice.ts" has content:
      """typescript
      import { formatDate } from "@myorg/shared/utils"
      import { calculateTax } from "./calculator"
      
      export function generateInvoice() {
        const tax = calculateTax(100)
        return formatDate(new Date()) + " tax:" + tax
      }
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft/billing"
    And Contract at "billing/BAFT.md" has 2 nodes and 1 edges
    And Contract at "billing/BAFT.md" is new
    Then file "billing/BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        calculator_dot_ts["calculator.ts"]
        invoice_dot_ts["invoice.ts"]
      
        invoice_dot_ts --> calculator_dot_ts
      ```
      """
    And file "shared/BAFT.md" should not exist
    And file "BAFT.md" should not exist

  Scenario: Dump runs from a relative path inside a TypeScript subdirectory bounded context
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ calculator.ts
      │  └─ invoice.ts
      ├─ shared/
      │  └─ utils.ts
      ├─ package.json
      ├─ tsconfig.json
      └─ README.md
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "README.md" has content "# Monorepo"
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":".","paths":{"@myorg/shared/*":["shared/*"],"@myorg/billing/*":["billing/*"]}}}
      """
    Given file "shared/utils.ts" has content:
      """typescript
      export function formatDate(d: Date) { return d.toISOString() }
      """
    Given file "billing/calculator.ts" has content:
      """typescript
      export function calculateTax(amount: number) { return amount * 0.1 }
      """
    Given file "billing/invoice.ts" has content:
      """typescript
      import { formatDate } from "@myorg/shared/utils"
      import { calculateTax } from "./calculator"
      
      export function generateInvoice() {
        const tax = calculateTax(100)
        return formatDate(new Date()) + " tax:" + tax
      }
      """
    Given the dump uses the "typescript" language adapter
    And the working directory is "/Users/jane/baft/billing"
    When the dump runs from "."
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        calculator_dot_ts["calculator.ts"]
        invoice_dot_ts["invoice.ts"]
      
        invoice_dot_ts --> calculator_dot_ts
      ```
      """
    And file "../shared/BAFT.md" should not exist
    And file "../BAFT.md" should not exist

  Scenario: Dump from workspace root shows cross-context imports
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ BAFT.md
      │  ├─ calculator.ts
      │  └─ invoice.ts
      ├─ shared/
      │  └─ utils.ts
      ├─ package.json
      ├─ tsconfig.json
      └─ README.md
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "README.md" has content "# Monorepo"
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":".","paths":{"@myorg/shared/*":["shared/*"],"@myorg/billing/*":["billing/*"]}}}
      """
    Given file "shared/utils.ts" has content:
      """typescript
      export function formatDate(d: Date) { return d.toISOString() }
      """
    And file "billing/BAFT.md" has content:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        calculator_dot_ts["calculator.ts"]
        invoice_dot_ts["invoice.ts"]
      
        invoice_dot_ts --> calculator_dot_ts
      ```
      """
    Given file "billing/calculator.ts" has content:
      """typescript
      export function calculateTax(amount: number) { return amount * 0.1 }
      """
    Given file "billing/invoice.ts" has content:
      """typescript
      import { formatDate } from "@myorg/shared/utils"
      import { calculateTax } from "./calculator"
      
      export function generateInvoice() {
        const tax = calculateTax(100)
        return formatDate(new Date()) + " tax:" + tax
      }
      """
    Given the dump uses the "typescript" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        billing["billing/&ast;&ast;"]
        shared_slash_utils_dot_ts["shared/utils.ts"]
      
        billing --> shared_slash_utils_dot_ts
      ```
      """
    And file "billing/BAFT.md" should stay the same

  Scenario: Dump ignores files matching .gitignore and .baftignore
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ .gitignore
      ├─ .baftignore
      └─ internal/
         ├─ application/
         │  └─ order.go
         ├─ generated/
         │  └─ generated.go
         ├─ vendor/
         │  └─ vendor.go
         └─ domain/
            └─ model.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file ".gitignore" has content:
      """ignore
      internal/generated/generated.go
      """
    Given file ".baftignore" has content:
      """ignore
      internal/vendor/vendor.go
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/generated/generated.go" has content:
      """go
      package generated
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/vendor/vendor.go" has content:
      """go
      package vendor
      
      import "example.com/billing/internal/generated"
      """
    Given file "internal/domain/model.go" has content:
      """go
      package domain
      type User struct{}
      """
    Given the dump uses the "go" language adapter
    When the dump runs from "/Users/jane/baft"
    And Contract at "BAFT.md" has 2 nodes and 1 edges
    And Contract at "BAFT.md" is new
    Then file "BAFT.md" should be:
      """config
      <!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->
      <!-- If Baft is new to you, run `baft manual`. -->
      <!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->
      <!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->
      
      ```mermaid
      flowchart TD
        internal_slash_application["internal/application"]
        internal_slash_domain["internal/domain"]
      
        internal_slash_application --> internal_slash_domain
      ```
      """
