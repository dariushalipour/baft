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
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Discovery error is surfaced as a result
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    Given the filesystem always returns a walk error
    When the check runs from "/Users/jane/baft"
    Then 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 1 error is reported

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
        app["application/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
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
    And 2 files are encountered
    And 2 files are scanned
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft/auth: read /Users/jane/baft/auth/BAFT.md: permission denied
      """
    And 0 violations are reported

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
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 3 errors are reported
    And the errors are:
      """errors
      /Users/jane/baft/auth: node "api" uses raw "*" in glob "api/**"; write &ast; instead (/Users/jane/baft/auth/BAFT.md:3)
      /Users/jane/baft/billing: handler (/Users/jane/baft/billing/BAFT.md:3) references api/handler.go — file-shaped nodes require a language that supports file globs
      /Users/jane/baft/billing: model (/Users/jane/baft/billing/BAFT.md:4) references domain/model.go — file-shaped nodes require a language that supports file globs
      """

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
        app["application/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
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
    And 4 files are encountered
    And 4 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Single capsule with multiple relation violations and two config errors
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
        api["internal/api/&ast;&ast;"]
        handler["internal/api/handler.go"]
        model["internal/model/repo.go"]
        app --> domain
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
    And 4 files are encountered
    And 4 files are scanned
    And 2 violations are reported
    And 2 errors are reported
    And the violations are:
      """violations
      /Users/jane/baft: internal/application/order.go:3:8 (app) → internal/api (api) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      /Users/jane/baft: internal/application/other.go:3:8 (app) → internal/api (api) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      """
    And the errors are:
      """errors
      /Users/jane/baft: handler (/Users/jane/baft/BAFT.md:6) references internal/api/handler.go — file-shaped nodes require a language that supports file globs
      /Users/jane/baft: model (/Users/jane/baft/BAFT.md:7) references internal/model/repo.go — file-shaped nodes require a language that supports file globs
      """

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
        app["application/&ast;&ast;"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 1 file is encountered
    And 1 file is scanned
    And 0 errors are reported
    And 0 violations are reported

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
        app["internal/application/&ast;&ast;"] --> domain["internal/domain/&ast;&ast;"]
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
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
      ```
      """
    Given file "BAFT.md" has unsaved content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/&ast;&ast;"] --> domain["internal/domain/&ast;&ast;"]
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
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
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
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
        app["internal/application/&ast;&ast;"] --> domain["internal/domain/&ast;&ast;"]
        api["internal/api/&ast;&ast;"]
      ```
      """
    Given file "BAFT.md" has unsaved content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/&ast;&ast;"] --> api["internal/api/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
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
    And 3 files are encountered
    And 3 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
    And 6 files are encountered
    And 6 files are scanned
    And 4 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft/handlers: ui/button.ts:1:23 (handlers) → ui/form.ts (handlers) — handlers is endophobic (/Users/jane/baft/handlers/BAFT.md)
      /Users/jane/baft/handlers: utils/helpers.ts:1:25 (handlers) → ui/button.ts (handlers) — handlers is endophobic (/Users/jane/baft/handlers/BAFT.md)
      /Users/jane/baft/internal: usecase/create/create_status.go:3:8 (usecase) → usecase/get (usecase) — usecase is endophobic (/Users/jane/baft/internal/BAFT.md)
      /Users/jane/baft/internal: usecase/create_order.go:3:8 (usecase) → usecase (usecase) — usecase is endophobic (/Users/jane/baft/internal/BAFT.md)
      """

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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
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
    And 2 files are encountered
    And 2 files are scanned
    And 1 violation is reported
    And the violation is:
      """violations
      /Users/jane/baft: internal/application/order.go:3:8 (app) → internal/domain (domain) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
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
        app["application/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
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
    And 4 files are encountered
    And 4 files are scanned
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft/auth: api/handler.go:3:8 (handler) → domain (domain) — relation not allowed (add edge in /Users/jane/baft/auth/BAFT.md or move the file)
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
        handler["internal/api/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
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
    And 4 files are encountered
    And 3 files are scanned
    And 0 errors are reported
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft: internal/api/other.go:3:8: import "example.com/billing/internal/valueobject" matches no node in /Users/jane/baft/BAFT.md
      /Users/jane/baft: internal/valueobject/vo.go is governed but matches no node in /Users/jane/baft/BAFT.md
      """

  Scenario: Language doesn't support file globs but BAFT.md has file-shaped glob — violation
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["internal/api/handler.go"]
        domain["internal/domain/&ast;&ast;"]
      ```
      """
    Given file "internal/api/handler.go" has content "package api"
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: handler (/Users/jane/baft/BAFT.md:3) references internal/api/handler.go — file-shaped nodes require a language that supports file globs
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
      
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
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 1 errors are reported
    And the error is:
      """errors
      /Users/jane/baft: domain (/Users/jane/baft/internal/application/BAFT.md:3) references ../domain/** — ".." not allowed in node globs
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
        domain["domain/&ast;&ast;"]
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
        domain["domain/&ast;&ast;"]
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
    And 4 files are encountered
    And 4 files are scanned
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/alice/dev/auth: api/login.go:3:8 (api) → domain (domain) — relation not allowed (add edge in /Users/alice/dev/auth/BAFT.md or move the file)
      /Users/alice/dev/billing: api/handler.go:3:8 (api) → domain (domain) — relation not allowed (add edge in /Users/alice/dev/billing/BAFT.md or move the file)
      """
    And 2 errors are reported
    And the errors are:
      """errors
      /Users/alice/dev/auth: api (/Users/alice/dev/auth/BAFT.md:4) references api/login.go — file-shaped nodes require a language that supports file globs
      /Users/alice/dev/billing: api (/Users/alice/dev/billing/BAFT.md:4) references api/handler.go — file-shaped nodes require a language that supports file globs
      """

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
        usecase["usecase/&ast;&ast;"] --> auth["../auth/usecase/&ast;&ast;"]
        usecase --> domain["domain/&ast;&ast;"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
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
    And 4 files are encountered
    And 4 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: auth (/Users/jane/baft/billing/BAFT.md:3) references ../auth/usecase/** — ".." not allowed in node globs
      """

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
        api["api/&ast;&ast;"]
        kk["..domain/&ast;&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 error is reported
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
        api["api/&ast;&ast;"]
        kk["nested/..domain/&ast;&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 error is reported
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
        domain["domain/&ast;&ast;"]
        dowhatever["do&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 error is reported
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
        domain["domain/&ast;&ast;"]
        samething["domain/&ast;&ast;"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: glob "domain/**" claimed by multiple nodes: domain, samething (/Users/jane/baft/billing/BAFT.md:4)
      """

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
        usecase["usecase/&ast;&ast;"] --> domain["domain/&ast;&ast;"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
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
    And 4 files are encountered
    And 4 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
        usecase["usecase/&ast;&ast;"] --> domain["domain/&ast;&ast;"]
      ```
      """
    Given file "platform/shared/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        logging["logging/&ast;&ast;"]
        database["database/&ast;&ast;"]
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
    And 5 files are encountered
    And 5 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
        usecase["usecase/&ast;&ast;"] --> domain["domain/&ast;&ast;"]
      ```
      """
    Given file "platform/shared/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        logging["logging/&ast;&ast;"]
        database["database/&ast;&ast;"]
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
    And 5 files are encountered
    And 5 files are scanned
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft: platform/billing/usecase/create_order.go:4:8 (billing) → platform/shared/logging (shared) — relation not allowed (add edge in /Users/jane/baft/platform/BAFT.md or move the file)
      /Users/jane/baft: platform/billing/usecase/create_order.go:5:8 (platform) → utils (utils) — relation not allowed (add edge in /Users/jane/baft/BAFT.md or move the file)
      """
    And 0 errors are reported

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
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft/auth: glob "billing/**" claimed by multiple nodes: billing, shared_again (/Users/jane/baft/BAFT.md:4)
      """

  Scenario: Malformed mermaid diagram in BAFT.md is reported as a parse error
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
        app["internal/application/&ast;&ast;"
        domain["internal/domain/&ast;&ast;"]
        app --> domain
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
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: unrecognized mermaid line: app["internal/application/&ast;&ast;" (/Users/jane/baft/BAFT.md:3)
      """

  Scenario: Raw asterisks in mermaid node labels are reported as a config error
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
        app["internal/application/**"]
        domain["internal/domain/&ast;&ast;"]
        app --> domain
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
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: node "app" uses raw "*" in glob "internal/application/**"; write &ast; instead (/Users/jane/baft/BAFT.md:3)
      """

  Scenario: Overlapping node globs are reported as an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ handlers/
         │  └─ create.go
         └─ services/
            └─ create.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handlers["internal/handlers/&ast;&ast;"]
        services["internal/&ast;&ast;"]
      ```
      """
    Given file "internal/handlers/create.go" has content "package handlers"
    Given file "internal/services/create.go" has content "package services"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: node "handlers" (/Users/jane/baft/BAFT.md:3) and node "services" (/Users/jane/baft/BAFT.md:4) overlap — file internal/handlers/create.go matches both globs
      """

  Scenario: Unclosed mermaid block is reported as a parse error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/&ast;&ast;"]
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: unclosed ```mermaid block (/Users/jane/baft/BAFT.md)
      """

  Scenario: Missing mermaid block is reported as a parse error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      Some markdown without a mermaid block.
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: no ```mermaid block found (/Users/jane/baft/BAFT.md)
      """

  Scenario: Mermaid block with no nodes is reported as an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: mermaid block declared no nodes (/Users/jane/baft/BAFT.md)
      """

  Scenario: Check passes when capsule root has governed sub dirs and parent declares cross-context edge
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
    And 3 files are encountered
    And 3 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Invalid edge token is reported as a parse error
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
        app --> bad-name!
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
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: invalid edge token "bad-name!" in line "app --> bad-name!" (/Users/jane/baft/BAFT.md:5)
      """

  Scenario: Node redefined with different glob is reported as an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/&ast;&ast;"]
        app["internal/domain/&ast;&ast;"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: node "app" redefined with a different glob ("internal/application/**" vs "internal/domain/**") (/Users/jane/baft/BAFT.md:4)
      """

  Scenario: Duplicate node globs are reported as an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/&ast;&ast;"]
        svc["internal/application/&ast;&ast;"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: glob "internal/application/**" claimed by multiple nodes: app, svc (/Users/jane/baft/BAFT.md:4)
      """

  Scenario: Duplicate node globs do not suppress invalid node glob errors
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
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/&ast;&ast;"]
        svc["internal/application/&ast;&ast;"]
        bad["../internal/domain/&ast;&ast;"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 2 errors are reported
    And the errors are:
      """errors
       /Users/jane/baft: bad (/Users/jane/baft/BAFT.md:5) references ../internal/domain/** — ".." not allowed in node globs
       /Users/jane/baft: glob "internal/application/**" claimed by multiple nodes: app, svc (/Users/jane/baft/BAFT.md:4)
      """

  Scenario: Empty node glob is reported as an error
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app[""]
        domain["internal/application/&ast;&ast;"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: node "app" has empty glob (/Users/jane/baft/BAFT.md:3)
      """

  Scenario: Edge line with fewer than two tokens is reported as a parse error
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
        app -->
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
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: edge has fewer than two nodes: "app -->" (/Users/jane/baft/BAFT.md:5)
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
        app["application/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
      ```
      """
    Given file "auth/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/&ast;&ast;"]
        domain["domain/&ast;&ast;"]
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
    And 2 files are encountered
    And 2 files are scanned
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft/auth: read /Users/jane/baft/auth/BAFT.md: permission denied
      """
    And 1 violation is reported
    And the violation is:
      """violations
      /Users/jane/baft/billing: application/order.go:3:8 (app) → domain (domain) — relation not allowed (add edge in /Users/jane/baft/billing/BAFT.md or move the file)
      """

  Scenario: Cyclic dependency between nodes is reported as an error
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
        app --> domain
        domain --> app
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content:
      """go
      package domain
      
      import "example.com/billing/internal/application"
      """
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: cycle detected: app → domain → app (/Users/jane/baft/BAFT.md:6)
      """

  Scenario: Multiple mermaid validation errors are all reported without short-circuiting
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
        app[""]
        domain[""]
        infra["internal/infrastructure/&ast;&ast;"]
        util["internal/util/&ast;&ast;"]
        app --> domain
        domain --> app
        app --> infra
        infra --> util
        util --> infra
        util --> nonexistent
        util --> missing
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content:
      """go
      package domain
      """
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 6 errors are reported
    And the errors are:
      """errors
      /Users/jane/baft: node "app" has empty glob (/Users/jane/baft/BAFT.md:3)
      /Users/jane/baft: node "domain" has empty glob (/Users/jane/baft/BAFT.md:4)
      /Users/jane/baft: cycle detected: app → domain → app (/Users/jane/baft/BAFT.md:8)
      /Users/jane/baft: cycle detected: infra → util → infra (/Users/jane/baft/BAFT.md:11)
      /Users/jane/baft: edge references undefined node "nonexistent" (/Users/jane/baft/BAFT.md:12)
      /Users/jane/baft: edge references undefined node "missing" (/Users/jane/baft/BAFT.md:13)
      """

  Scenario: Edge references undefined node is reported as an error
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
        app --> infrastructure
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
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: edge references undefined node "infrastructure" (/Users/jane/baft/BAFT.md:5)
      """

  Scenario: Multiple mermaid blocks in BAFT.md is reported as an error
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
        app["internal/application/&ast;&ast;"] --> domain["internal/domain/&ast;&ast;"]
      ```
      Some explanatory text between blocks.
      ```mermaid
      flowchart TD
        domain["internal/domain/&ast;&ast;"]
        infra["internal/infrastructure/&ast;&ast;"]
        infra --> domain
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
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: multiple ```mermaid blocks found (/Users/jane/baft/BAFT.md:6)
      """

  Scenario: Self-referencing edge is reported as an error
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
        app["internal/application/&ast;&ast;"]
        domain["internal/domain/&ast;&ast;"]
        app --> domain
        domain --> domain
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content:
      """go
      package domain
      
      import "example.com/billing/internal/domain"
      """
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/baft: edge references same node on both sides: domain → domain (/Users/jane/baft/BAFT.md:6)
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
        core["core/&ast;&ast;"]
      ```
      """
    Given file "core/config.go" has content "package core"
    Given file "sub/go.mod" has content "module example.com/sub"
    Given file "sub/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        svc["service/&ast;&ast;"]
      ```
      """
    Given file "sub/domain/model.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 2 capsules are discovered
    And 1 violation is reported
    And the violation is:
      """violations
      /Users/jane/baft/sub: domain/model.go is governed but matches no node in /Users/jane/baft/BAFT.md
      """

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
    And 1 files are encountered
    And 1 files are scanned
    And 0 violations are reported
    And 0 errors are reported

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
        usecase["usecase/&ast;&ast;"] --> core["core/&ast;&ast;"]
        external["external/&ast;&ast;"]
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
    And 3 files are encountered
    And 3 files are scanned
    And 1 violations are reported
    And the violations are:
      """violations
      /Users/jane/baft/billing: api/usecase/create_order.go:4:8 (usecase) → api/external (external) — relation not allowed (add edge in /Users/jane/baft/billing/api/BAFT.md or move the file)
      """
    And 0 errors are reported
