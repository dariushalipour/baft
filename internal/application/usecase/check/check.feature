Feature: Architecture rule checking
  As a developer
  I want baft to verify that my code respects the architecture I declared
  So that my design does not silently degrade

  Scenario: No capsules discovered yields an empty result
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    When the check runs from "/Users/jane/baft"
    Then 0 capsules are discovered
    And 0 relations are examined
    And 0 files are encountered and 0 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Discovery error is surfaced as a result
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    Given the filesystem always returns a walk error
    When the check runs from "/Users/jane/baft"
    Then 0 relations are examined
    And 0 files are encountered and 0 files are scanned
    And 1 errors and 0 violations are reported

  Scenario: Capsules discovered but missing BAFT.md is skipped
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ billing/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  └─ application/
      │     └─ order.go
      └─ orphan/
         └─ go.mod
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "orphan/go.mod" has content "module example.com/orphan"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
          app["application"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 1 files are encountered and 1 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Check continues after a capsule error and reports remaining capsules
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      ├─ auth/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application"]
        domain["domain"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api"]
        domain["domain"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/api/handler.go" has content "package api"
    Given file "auth/domain/auth.go" has content "package domain"
    Given the filesystem is not permitted to read "auth/BAFT.md"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 2 files are encountered and 2 files are scanned
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft/auth: read /Users/jane/baft/auth/BAFT.md: permission denied
      """

  Scenario: Multiple capsule errors all appear, including raw asterisk parse errors
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  └─ BAFT.md
      └─ auth/
         ├─ go.mod
         └─ BAFT.md
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/handler.go"]
        model["domain/model.go"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api/**"]
        domain["domain/&ast;&ast;"]
      ```
      """
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 0 files are encountered and 0 files are scanned
    And 3 errors and 0 violations are reported
    And the errors are:
      """errors
      /Users/jane/baft/auth: node "api" uses raw "*" in glob "api/**"; write &ast; instead (/Users/jane/baft/auth/BAFT.md:3)
      /Users/jane/baft/billing: handler (/Users/jane/baft/billing/BAFT.md:3) references api/handler.go — file-shaped nodes require a language that supports file globs
      /Users/jane/baft/billing: model (/Users/jane/baft/billing/BAFT.md:4) references domain/model.go — file-shaped nodes require a language that supports file globs
      """

  Scenario: Shared root config load error is reported once across sibling capsules
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ BAFT.md
      ├─ auth/
      │  └─ go.mod
      └─ billing/
         └─ go.mod
      """
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/&ast;&ast;"]
        shared_again["billing/&ast;&ast;"]
      ```
      """
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 0 files are encountered and 0 files are scanned
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft/auth: glob "billing/**" claimed by multiple nodes: billing, shared_again (/Users/jane/baft/BAFT.md:4)
      """

  Scenario: Architecture is respected when imports follow declared rules
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"] --> domain["internal/domain"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Architecture violation is detected when imports break declared rules
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 1 violations are reported
    And the violation is:
      """violations
      /Users/jane/baft: internal/application/order.go:3:8 (app) → internal/domain (domain) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      """

  Scenario: Single capsule with multiple relation violations and three config errors
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  ├─ order.go
         │  └─ other.go
         ├─ domain/
         │  └─ order.go
         └─ api/
            └─ handler.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
        api["internal/api"]
        handler["internal/api/handler.go"]
        model["internal/model/repo.go"]
        app --> domain
        domain --> app
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/app/internal/api"
      """
    Given file "internal/application/other.go" has content:
      """go
      package application
      
      import "example.com/app/internal/api"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given file "internal/api/handler.go" has content "package api"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 4 files are encountered and 4 files are scanned
    And 3 errors and 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft: internal/application/order.go:3:8 (app) → internal/api (api) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      /Users/jane/baft: internal/application/other.go:3:8 (app) → internal/api (api) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      """
    And the errors are:
      """errors
      /Users/jane/baft: circular dependency: app → domain → app (/Users/jane/baft/BAFT.md:9)
      /Users/jane/baft: handler (/Users/jane/baft/BAFT.md:6) references internal/api/handler.go — file-shaped nodes require a language that supports file globs
      /Users/jane/baft: model (/Users/jane/baft/BAFT.md:7) references internal/model/repo.go — file-shaped nodes require a language that supports file globs
      """

  Scenario: Multiple capsules each with architecture violations, all violations reported
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ billing/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      ├─ auth/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application"]
        domain["domain"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api"]
        domain["domain"]
      ```
      """
    Given file "billing/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/domain"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/api/handler.go" has content:
      """go
      package api
      
      import "example.com/auth/domain"
      """
    Given file "auth/domain/auth.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 2 relations are examined
    And 4 files are encountered and 4 files are scanned
    And 0 errors and 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft/auth: api/handler.go:3:8 (handler) → domain (domain) — relation not allowed (add edge in /Users/jane/baft/auth/BAFT.md or move the file)
      /Users/jane/baft/billing: application/order.go:3:8 (app) → domain (domain) — relation not allowed (add edge in /Users/jane/baft/billing/BAFT.md or move the file)
      """

  Scenario: One capsule errors and another produces violations, both reported
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      ├─ auth/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application"]
        domain["domain"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api"]
        domain["domain"]
      ```
      """
    Given file "billing/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/domain"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/api/handler.go" has content:
      """go
      package api
      
      import "example.com/auth/domain"
      """
    Given file "auth/domain/auth.go" has content "package domain"
    Given the filesystem is not permitted to read "auth/BAFT.md"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 1 relation is examined
    And 2 files are encountered and 2 files are scanned
    And 1 errors and 1 violations are reported
    And the error is:
      """errors
      /Users/jane/baft/auth: read /Users/jane/baft/auth/BAFT.md: permission denied
      """
    And the violation is:
      """violations
      /Users/jane/baft/billing: application/order.go:3:8 (app) → domain (domain) — relation not allowed (add edge in /Users/jane/baft/billing/BAFT.md or move the file)
      """

  Scenario: File with imports that matches no node in BAFT.md is a violation
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ api/
         │  ├─ handler.go
         │  └─ other.go
         ├─ domain/
         │  └─ order.go
         └─ valueobject/
            └─ vo.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["internal/api"]
        domain["internal/domain"]
      ```
      """
    Given file "internal/api/handler.go" has content "package api"
    Given file "internal/api/other.go" has content:
      """go
      package api
      
      import "example.com/billing/internal/valueobject"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given file "internal/valueobject/vo.go" has content "package valueobject"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 4 files are encountered and 3 files are scanned
    And 0 errors and 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft: internal/api/other.go:3:8: import "example.com/billing/internal/valueobject" matches no node in /Users/jane/baft/BAFT.md
      /Users/jane/baft: internal/valueobject/vo.go is tracked by /Users/jane/baft/BAFT.md but matches no node
      """

  Scenario: Import from a path outside any capsule is ignored
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
      
        app --> domain
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      import "example.com/shared/pkg/utils"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Endophobic node forbids same-node imports
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ internal/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  └─ usecase/
      │     ├─ create_order.go
      │     ├─ get_order.go
      │     └─ create/
      │        └─ create_status.go
      └─ handlers/
         ├─ package.json
         ├─ BAFT.md
         ├─ ui/
         │  ├─ button.ts
         │  └─ form.ts
         └─ utils/
            └─ helpers.ts
      """
    Given file "internal/go.mod" has content "module example.com/billing"
    Given file "internal/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/&ast;&ast;"]:::endophobic
      ```
      """
    Given file "internal/usecase/create_order.go" has content:
      """go
      package usecase
      
      import "example.com/billing/usecase"
      """
    Given file "internal/usecase/get_order.go" has content "package get"
    Given file "internal/usecase/create/create_status.go" has content:
      """go
      package create
      
      import "example.com/billing/usecase/get"
      """
    Given file "handlers/package.json" has content:
      """json
      {"name":"@baft/billing"}
      """
    Given file "handlers/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handlers["&ast;"]:::endophobic
      ```
      """
    Given file "handlers/ui/button.ts" has content:
      """typescript
      import { form } from "./form"
      """
    Given file "handlers/ui/form.ts" has content "export const form = () => {}"
    Given file "handlers/utils/helpers.ts" has content:
      """typescript
      import { button } from "../ui/button"
      """
    Given the check uses the "go" language adapter
    Given the check uses the "typescript" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 4 relations are examined
    And 6 files are encountered and 6 files are scanned
    And 0 errors and 4 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft/handlers: ui/button.ts:1:23 (handlers) → ui/form.ts (handlers) — handlers is endophobic (/Users/jane/baft/handlers/BAFT.md)
      /Users/jane/baft/handlers: utils/helpers.ts:1:25 (handlers) → ui/button.ts (handlers) — handlers is endophobic (/Users/jane/baft/handlers/BAFT.md)
      /Users/jane/baft/internal: usecase/create/create_status.go:3:8 (usecase) → usecase/get (usecase) — usecase is endophobic (/Users/jane/baft/internal/BAFT.md)
      /Users/jane/baft/internal: usecase/create_order.go:3:8 (usecase) → usecase (usecase) — usecase is endophobic (/Users/jane/baft/internal/BAFT.md)
      """

  Scenario: Capsule with scoped config only — allowed and disallowed relations
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ internal/
         ├─ application/
         │  ├─ BAFT.md
         │  ├─ create_order.go
         │  └─ get_order.go
         └─ domain/
            ├─ order.go
            └─ repo.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "internal/application/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["&ast;&ast;"] --> domain["../domain/&ast;&ast;"]
      ```
      """
    Given file "internal/application/create_order.go" has content:
      """go
      package application
      
      import "example.com/app/internal/domain"
      """
    Given file "internal/application/get_order.go" has content:
      """go
      package application
      
      import "example.com/app/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given file "internal/domain/repo.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 2 files are encountered and 2 files are scanned
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft: domain (/Users/jane/baft/internal/application/BAFT.md:3) references ../domain/** — ".." not allowed in node globs
      """

  Scenario: Scoped config child imports from parent scope with one allowed and one violation
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      └─ billing/
         ├─ go.mod
         ├─ api/
         │  ├─ BAFT.md
         │  ├─ core/
         │  │  └─ logger.go
         │  ├─ external/
         │  │  └─ crypto.go
         │  └─ usecase/
         │     ├─ BAFT.md
         │     └─ create_order.go
      """
    Given file "billing/go.mod" has content "module example.com/app"
    Given file "billing/api/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/&ast;&ast;"] --> core["core"]
        external["external"]
      ```
      """
    Given file "billing/api/usecase/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["&ast;&ast;"]
      ```
      """
    Given file "billing/api/usecase/create_order.go" has content:
      """go
      package usecase
      
      import "example.com/app/api/core"
      import "example.com/app/api/external"
      """
    Given file "billing/api/core/logger.go" has content "package core"
    Given file "billing/api/external/crypto.go" has content "package external"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 3 files are encountered and 3 files are scanned
    And the violations are:
      """violations
      /Users/jane/baft/billing: api/usecase/create_order.go:4:8 (usecase) → api/external (external) — relation not allowed (add edge in /Users/jane/baft/billing/api/BAFT.md or move the file)
      """
    And 0 errors and 1 violations are reported

  Scenario: Scoped config without parent config treats out-of-scope imports as external
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ api/
         │  ├─ BAFT.md
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "billing/api/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["&ast;&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content:
      """go
      package api
      
      import "example.com/billing/domain"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 1 files are encountered and 1 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Check runs from a parent directory above capsules
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ go.mod
         ├─ BAFT.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application"]
        domain["domain"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api"]
        domain["domain"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/api/handler.go" has content "package api"
    Given file "auth/domain/auth.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 4 files are encountered and 4 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Parent BAFT.md declares cross-context edge between sibling capsules
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ billing/
      │  ├─ BAFT.md
      │  ├─ usecase/
      │  │  └─ create_order.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ BAFT.md
         ├─ usecase/
         │  └─ register_user.go
         └─ domain/
            └─ user.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/&ast;&ast;"] --> auth["auth/&ast;&ast;"]
      ```
      """
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase"] --> domain["domain"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase"]
        domain["domain"]
      ```
      """
    Given file "billing/usecase/create_order.go" has content:
      """go
      package usecase
      
      import "example.com/app/billing/domain"
      import "example.com/app/auth/usecase"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/usecase/register_user.go" has content "package usecase"
    Given file "auth/domain/user.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 4 files are encountered and 4 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Capsule uses .. to reference sibling bounded context
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ billing/
      │  ├─ BAFT.md
      │  ├─ usecase/
      │  │  └─ create_order.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ BAFT.md
         ├─ usecase/
         │  └─ register_user.go
         └─ domain/
            └─ user.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase"] --> auth["../auth/usecase/&ast;&ast;"]
        usecase --> domain["domain"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase"]
        domain["domain"]
      ```
      """
    Given file "billing/usecase/create_order.go" has content:
      """go
      package usecase
      
      import "example.com/billing/domain"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/usecase/register_user.go" has content "package usecase"
    Given file "auth/domain/user.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 4 files are encountered and 4 files are scanned
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft: auth (/Users/jane/baft/billing/BAFT.md:3) references ../auth/usecase/** — ".." not allowed in node globs
      """

  Scenario: Parent capsule root allows import from nested capsule to sibling scope as single node
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ platform/
      │  ├─ BAFT.md
      │  ├─ billing/
      │  │  ├─ BAFT.md
      │  │  ├─ usecase/
      │  │  │  └─ create_order.go
      │  │  └─ domain/
      │  │     └─ order.go
      │  └─ shared/
      │     ├─ BAFT.md
      │     ├─ logging/
      │     │  └─ logger.go
      │     └─ database/
      │        └─ connection.go
      └─ shared/
         └─ utils.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        platform["platform/&ast;&ast;"] --> shared_lib["shared/&ast;&ast;"]
      ```
      """
    Given file "platform/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/&ast;&ast;"] --> shared["shared/&ast;&ast;"]
      ```
      """
    Given file "platform/billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase"] --> domain["domain"]
      ```
      """
    Given file "platform/shared/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        logging["logging"]
        database["database"]
      ```
      """
    Given file "platform/billing/usecase/create_order.go" has content:
      """go
      package usecase
      
      import "example.com/app/platform/billing/domain"
      import "example.com/app/platform/shared/logging"
      import "example.com/app/shared/utils"
      """
    Given file "platform/billing/domain/order.go" has content "package domain"
    Given file "platform/shared/logging/logger.go" has content "package logging"
    Given file "platform/shared/database/connection.go" has content "package database"
    Given file "shared/utils.go" has content "package utils"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 3 relations are examined
    And 5 files are encountered and 5 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Parent capsule root denies import that intermediate scopes would allow
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ platform/
      │  ├─ BAFT.md
      │  ├─ billing/
      │  │  ├─ BAFT.md
      │  │  ├─ usecase/
      │  │  │  └─ create_order.go
      │  │  └─ domain/
      │  │     └─ order.go
      │  └─ shared/
      │     ├─ BAFT.md
      │     ├─ logging/
      │     │  └─ logger.go
      │     └─ database/
      │        └─ connection.go
      └─ utils/
         └─ utils.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        platform["platform/&ast;&ast;"]
        utils["utils/&ast;&ast;"]
      ```
      """
    Given file "platform/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/&ast;&ast;"]
        shared["shared/&ast;&ast;"]
      ```
      """
    Given file "platform/billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase"] --> domain["domain"]
      ```
      """
    Given file "platform/shared/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        logging["logging"]
        database["database"]
      ```
      """
    Given file "platform/billing/usecase/create_order.go" has content:
      """go
      package usecase
      
      import "example.com/app/platform/billing/domain"
      import "example.com/app/platform/shared/logging"
      import "example.com/app/utils"
      """
    Given file "platform/billing/domain/order.go" has content "package domain"
    Given file "platform/shared/logging/logger.go" has content "package logging"
    Given file "platform/shared/database/connection.go" has content "package database"
    Given file "utils/utils.go" has content "package utils"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 3 relations are examined
    And 5 files are encountered and 5 files are scanned
    And the violations are:
      """violations
      /Users/jane/baft: platform/billing/usecase/create_order.go:4:8 (billing) → platform/shared/logging (shared) — relation not allowed (add edge in /Users/jane/baft/platform/BAFT.md or move the file)
      /Users/jane/baft: platform/billing/usecase/create_order.go:5:8 (platform) → utils (utils) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      """
    And 0 errors and 2 violations are reported

  Scenario: Check passes when capsule root has tracked sub dirs and parent declares cross-context edge
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
      └─ BAFT.md
      """
    Given file "package.json" has content '{"name":"@myorg/app"}'
    Given file "tsconfig.json" has content:
      """json
      {"compilerOptions":{"baseUrl":".","paths":{"@myorg/shared/*":["shared/*"],"@myorg/billing/*":["billing/*"]}}}
      """
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/&ast;&ast;"] --> shared["shared/&ast;&ast;"]
      ```
      """
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        calculator_dot_ts["calculator.ts"]
        invoice_dot_ts["invoice.ts"]
      
        invoice_dot_ts --> calculator_dot_ts
      ```
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
    Given the check uses the "typescript" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 3 files are encountered and 3 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Unsaved BAFT.md content is checked without writing to disk
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
      ```
      """
    Given file "BAFT.md" has unsaved content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"] --> domain["internal/domain"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Unsaved source content is checked without writing to disk
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/order.go" has unsaved content:
      """go
      package application
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Multiple unsaved files are checked together
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ api/
         │  └─ handler.go
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"] --> domain["internal/domain"]
        api["internal/api"]
      ```
      """
    Given file "BAFT.md" has unsaved content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"] --> api["internal/api"]
        domain["internal/domain"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/order.go" has unsaved content:
      """go
      package application
      
      import "example.com/billing/internal/api"
      """
    Given file "internal/api/handler.go" has content "package api"
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 3 files are encountered and 3 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Import towards a baftignored file is treated as external
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ .baftignore
      └─ internal/
         ├─ application/
         │  └─ order.go
         ├─ orphan/
         │  └─ orphan.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
      
        app --> domain
      ```
      """
    Given file ".baftignore" has content:
      """ignore
      internal/orphan/orphan.go
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      import "example.com/billing/internal/orphan"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: .baftignore file ignored files are invisible to the check
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ .baftignore
      └─ internal/
         ├─ application/
         │  ├─ order.go
         │  └─ outlaw.go
         ├─ domain/
         │  └─ order.go
         ├─ immune/
         │  ├─ some.go
         │  └─ files.go
         └─ forbidden/
            └─ forbidden.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
        forbidden["internal/forbidden"]
      
        app --> domain
      ```
      """
    Given file ".baftignore" has content:
      """ignore
      internal/application/outlaw.go
      internal/immune/**
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/outlaw.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      import "example.com/billing/internal/forbidden"
      """
    Given file "internal/forbidden/forbidden.go" has content "package forbidden"
    Given file "internal/immune/some.go" has content "package immune"
    Given file "internal/immune/files.go" has content "package immune"
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 3 files are encountered and 3 files are scanned
    And 0 errors and 0 violations are reported

  Scenario: Negation pattern re-includes a previously ignored file
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ .baftignore
      └─ internal/
         ├─ application/
         │  ├─ order.go
         │  ├─ special_test.go
         │  └─ other_test.go
         ├─ domain/
         │  └─ order.go
         └─ forbidden/
            └─ secret.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
        forbidden["internal/forbidden"]
      
        app --> domain
      ```
      """
    Given file ".baftignore" has content:
      """ignore
      internal/application/*_test.go
      !internal/application/special_test.go
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/special_test.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/other_test.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      import "example.com/billing/internal/forbidden"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given file "internal/forbidden/secret.go" has content "package forbidden"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 4 files are encountered and 4 files are scanned
    And 2 relations are examined
    And 0 errors and 0 violations are reported

  Scenario: .baftignore and .gitignore are interchangeable, .baftignore takes precedence
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ .gitignore
      ├─ .baftignore
      └─ internal/
         ├─ application/
         │  ├─ order.go
         │  ├─ gitignored.go
         │  ├─ baftignored.go
         │  └─ bothignored.go
         ├─ domain/
         │  └─ order.go
         └─ forbidden/
            └─ secret.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
        forbidden["internal/forbidden"]
      
        app --> domain
      ```
      """
    Given file ".gitignore" has content:
      """ignore
      internal/application/gitignored.go
      internal/application/bothignored.go
      """
    Given file ".baftignore" has content:
      """ignore
      internal/application/baftignored.go
      !internal/application/bothignored.go
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/gitignored.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/application/baftignored.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      import "example.com/billing/internal/forbidden"
      """
    Given file "internal/application/bothignored.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given file "internal/forbidden/secret.go" has content "package forbidden"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 4 files are encountered and 4 files are scanned
    And 2 relations are examined
    And 0 errors and 0 violations are reported

  Scenario: Nested multiple .baftignore files act like .gitignore
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ .gitignore
      ├─ .baftignore
      └─ internal/
         ├─ application/
         │  ├─ BAFT.md
         │  ├─ order.go
         │  └─ .baftignore
         ├─ domain/
         │  └─ order.go
         ├─ forbidden/
         │  └─ secret.go
         └─ vendor/
            └─ lib.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application"]
        domain["internal/domain"]
        forbidden["internal/forbidden"]
      
        app --> domain
      ```
      """
    Given file "internal/application/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["&ast;&ast;"]
      ```
      """
    Given file ".gitignore" has content:
      """ignore
      internal/vendor/**
      """
    Given file ".baftignore" has content:
      """ignore
      internal/vendor/**
      """
    Given file "internal/application/.baftignore" has content:
      """ignore
      order.go
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      import "example.com/billing/internal/forbidden"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given file "internal/forbidden/secret.go" has content "package forbidden"
    Given file "internal/vendor/lib.go" has content "package vendor"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 2 files are encountered and 2 files are scanned
    And 0 relations are examined
    And 0 errors and 0 violations are reported

  Scenario: Scoped config has a node whose glob starts with .. (prefix, not standalone segment)
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ BAFT.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api"]
        kk["..domain/&ast;&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft: kk (/Users/jane/baft/billing/BAFT.md:4) references ..domain/** — ".." not allowed in node globs
      """

  Scenario: Scoped config has a node whose later segment starts with .. (prefix, not standalone segment)
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ BAFT.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api"]
        kk["nested/..domain/&ast;&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft: kk (/Users/jane/baft/billing/BAFT.md:4) references nested/..domain/** — ".." not allowed in node globs
      """

  Scenario: Scoped config inside a capsule has overlapping node globs
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ BAFT.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain"]
        dowhatever["do&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft: node "domain" (/Users/jane/baft/billing/BAFT.md:3) and node "dowhatever" (/Users/jane/baft/billing/BAFT.md:4) overlap — file domain/order.go matches both globs
      """

  Scenario: Scoped config inside a capsule with duplicate node globs is reported as an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ BAFT.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain"]
        samething["domain"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 errors and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/baft: glob "domain" claimed by multiple nodes: domain, samething (/Users/jane/baft/billing/BAFT.md:4)
      """

  Scenario: Files in a nested capsule are not double-reported by the parent capsule
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      ├─ core/
      │  └─ config.go
      └─ sub/
         ├─ go.mod
         ├─ BAFT.md
         └─ domain/
            └─ model.go
      """
    Given file "go.mod" has content "module example.com/root"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        core["core"]
      ```
      """
    Given file "core/config.go" has content "package core"
    Given file "sub/go.mod" has content "module example.com/sub"
    Given file "sub/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        svc["service"]
      ```
      """
    Given file "sub/domain/model.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 0 errors and 1 violations are reported
    And the violation is:
      """violations
      /Users/jane/baft/sub: domain/model.go is tracked by /Users/jane/baft/BAFT.md but matches no node
      """

  Scenario: Capsule label uses absolute path for root capsule
    Given a fresh workspace at "/Users/alice/dev" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ BAFT.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ go.mod
         ├─ BAFT.md
         ├─ api/
         │  └─ login.go
         └─ domain/
            └─ user.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain"]
        api["api/handler.go"]
      ```
      """
    Given file "billing/api/handler.go" has content:
      """go
      package api
      
      import "example.com/billing/domain"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain"]
        api["api/login.go"]
      ```
      """
    Given file "auth/api/login.go" has content:
      """go
      package api
      
      import "example.com/auth/domain"
      """
    Given file "auth/domain/user.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/alice/dev"
    Then 2 capsules are discovered
    And 2 relations are examined
    And 4 files are encountered and 4 files are scanned
    And the violations are:
      """violations
      /Users/alice/dev/auth: api/login.go:3:8 (api) → domain (domain) — relation not allowed (add edge in /Users/alice/dev/auth/BAFT.md or move the file)
      /Users/alice/dev/billing: api/handler.go:3:8 (api) → domain (domain) — relation not allowed (add edge in /Users/alice/dev/billing/BAFT.md or move the file)
      """
    And 2 errors and 2 violations are reported
    And the errors are:
      """errors
      /Users/alice/dev/auth: api (/Users/alice/dev/auth/BAFT.md:4) references api/login.go — file-shaped nodes require a language that supports file globs
      /Users/alice/dev/billing: api (/Users/alice/dev/billing/BAFT.md:4) references api/handler.go — file-shaped nodes require a language that supports file globs
      """
