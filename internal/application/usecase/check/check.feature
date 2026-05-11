Feature: Architecture rule checking
  As a developer
  I want strata to verify that my code respects the architecture I declared
  So that my design does not silently degrade

  Scenario: No capsules discovered yields an empty result
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    When the check runs from "/Users/jane/strata"
    Then 0 capsules are discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Discovery error is surfaced as a result
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      └─ src/
         └─ app.ts
      """
    Given the filesystem always returns a walk error
    When the check runs from "/Users/jane/strata"
    Then 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 1 error is reported

  Scenario: Check continues after a capsule error and reports remaining capsules
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      ├─ auth/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application/**"]
        domain["domain/**"]
      ```
      """
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/**"]
        domain["domain/**"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/api/handler.go" has content "package api"
    Given file "auth/domain/auth.go" has content "package domain"
    Given the filesystem is not permitted to read "auth/STRATA.md"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 2 files are encountered
    And 2 files are scanned
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata/auth: read /Users/jane/strata/auth/STRATA.md: permission denied
      """
    And 0 violations are reported

  Scenario: Check runs from a parent directory above capsules
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ go.mod
         ├─ STRATA.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application/**"]
        domain["domain/**"]
      ```
      """
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/**"]
        domain["domain/**"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/api/handler.go" has content "package api"
    Given file "auth/domain/auth.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 4 files are encountered
    And 4 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Single capsule with multiple relation violations and two config errors
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
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
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
        api["internal/api/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 4 files are encountered
    And 4 files are scanned
    And 2 violations are reported
    And 2 errors are reported
    And the violations are:
      """violations
      /Users/jane/strata: internal/application/order.go:3:8 (app) → internal/api (api) — relation not allowed (add edge in /Users/jane/strata/STRATA.md or move the file)
      /Users/jane/strata: internal/application/other.go:3:8 (app) → internal/api (api) — relation not allowed (add edge in /Users/jane/strata/STRATA.md or move the file)
      """
    And the errors are:
      """errors
      /Users/jane/strata: handler (/Users/jane/strata/STRATA.md:6) references internal/api/handler.go — file-shaped nodes require a language that supports file globs
      /Users/jane/strata: model (/Users/jane/strata/STRATA.md:7) references internal/model/repo.go — file-shaped nodes require a language that supports file globs
      """

  Scenario: Capsules discovered but missing STRATA.md is skipped
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ billing/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  └─ application/
      │     └─ order.go
      └─ orphan/
         └─ go.mod
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "orphan/go.mod" has content "module example.com/orphan"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application/**"]
      ```
      """
    Given file "billing/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 1 file is encountered
    And 1 file is scanned
    And 0 errors are reported
    And 0 violations are reported

  Scenario: Architecture is respected when imports follow declared rules
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"] --> domain["internal/domain/**"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Unsaved STRATA.md content is checked without writing to disk
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
      ```
      """
    Given file "STRATA.md" has unsaved content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"] --> domain["internal/domain/**"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application

      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Unsaved source content is checked without writing to disk
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Multiple unsaved files are checked together
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ api/
         │  └─ handler.go
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"] --> domain["internal/domain/**"]
        api["internal/api/**"]
      ```
      """
    Given file "STRATA.md" has unsaved content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"] --> api["internal/api/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 3 files are encountered
    And 3 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Endophobic node forbids same-node imports
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ internal/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  └─ usecase/
      │     ├─ create_order.go
      │     ├─ get_order.go
      │     └─ create/
      │        └─ create_status.go
      └─ handlers/
         ├─ package.json
         ├─ STRATA.md
         ├─ ui/
         │  ├─ button.ts
         │  └─ form.ts
         └─ utils/
            └─ helpers.ts
      """
    Given file "internal/go.mod" has content "module example.com/billing"
    Given file "internal/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"]:::endophobic
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
      {"name":"@strata/billing"}
      """
    Given file "handlers/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handlers["*"]:::endophobic
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
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 4 relations are examined
    And 6 files are encountered
    And 6 files are scanned
    And 4 violations are reported
    And the violations are:
      """violations
      /Users/jane/strata/handlers: ui/button.ts:1:23 (handlers) → ui/form.ts (handlers) — handlers is endophobic (/Users/jane/strata/handlers/STRATA.md)
      /Users/jane/strata/handlers: utils/helpers.ts:1:25 (handlers) → ui/button.ts (handlers) — handlers is endophobic (/Users/jane/strata/handlers/STRATA.md)
      /Users/jane/strata/internal: usecase/create/create_status.go:3:8 (usecase) → usecase/get (usecase) — usecase is endophobic (/Users/jane/strata/internal/STRATA.md)
      /Users/jane/strata/internal: usecase/create_order.go:3:8 (usecase) → usecase (usecase) — usecase is endophobic (/Users/jane/strata/internal/STRATA.md)
      """

  Scenario: Architecture violation is detected when imports break declared rules
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
      ```
      """
    Given file "internal/application/order.go" has content:
      """go
      package application
      
      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered
    And 2 files are scanned
    And 1 violation is reported
    And the violation is:
      """violations
      /Users/jane/strata: internal/application/order.go:3:8 (app) → internal/domain (domain) — relation not allowed (add edge in /Users/jane/strata/STRATA.md or move the file)
      """

  Scenario: Multiple capsules each with architecture violations, all violations reported
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ billing/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      ├─ auth/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application/**"]
        domain["domain/**"]
      ```
      """
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/**"]
        domain["domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 2 relations are examined
    And 4 files are encountered
    And 4 files are scanned
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/strata/auth: api/handler.go:3:8 (handler) → domain (domain) — relation not allowed (add edge in /Users/jane/strata/auth/STRATA.md or move the file)
      /Users/jane/strata/billing: application/order.go:3:8 (app) → domain (domain) — relation not allowed (add edge in /Users/jane/strata/billing/STRATA.md or move the file)
      """

  Scenario: File with imports that matches no node in STRATA.md is a violation
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
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
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["internal/api/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 4 files are encountered
    And 3 files are scanned
    And 0 errors are reported
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/strata: internal/api/other.go:3:8: import "example.com/billing/internal/valueobject" matches no node in /Users/jane/strata/STRATA.md
      /Users/jane/strata: internal/valueobject/vo.go is governed but matches no node in /Users/jane/strata/STRATA.md
      """

  Scenario: Language doesn't support file globs but STRATA.md has file-shaped glob — violation
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["internal/api/handler.go"]
        domain["internal/domain/**"]
      ```
      """
    Given file "internal/api/handler.go" has content "package api"
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: handler (/Users/jane/strata/STRATA.md:3) references internal/api/handler.go — file-shaped nodes require a language that supports file globs
      """

  Scenario: Import from a path outside any capsule is ignored
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
      
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Capsule with scoped config only — allowed and disallowed relations
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      └─ internal/
         ├─ application/
         │  ├─ STRATA.md
         │  ├─ create_order.go
         │  └─ get_order.go
         └─ domain/
            ├─ order.go
            └─ repo.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "internal/application/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["**"] --> domain["../domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 2 files are encountered
    And 2 files are scanned
    And 0 violations are reported
    And 1 errors are reported
    And the error is:
      """errors
      /Users/jane/strata: domain (/Users/jane/strata/internal/application/STRATA.md:3) references ../domain/** — ".." not allowed in node globs
      """

  Scenario: Capsule label uses absolute path for root capsule
    Given a fresh workspace at "/Users/alice/dev" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ go.mod
         ├─ STRATA.md
         ├─ api/
         │  └─ login.go
         └─ domain/
            └─ user.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain/**"]
        api["api/handler.go"]
      ```
      """
    Given file "billing/api/handler.go" has content:
      """go
      package api
      
      import "example.com/billing/domain"
      """
    Given file "billing/domain/order.go" has content "package domain"
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain/**"]
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
      /Users/alice/dev/auth: api/login.go:3:8 (api) → domain (domain) — relation not allowed (add edge in /Users/alice/dev/auth/STRATA.md or move the file)
      /Users/alice/dev/billing: api/handler.go:3:8 (api) → domain (domain) — relation not allowed (add edge in /Users/alice/dev/billing/STRATA.md or move the file)
      """
    And 2 errors are reported
    And the errors are:
      """errors
      /Users/alice/dev/auth: api (/Users/alice/dev/auth/STRATA.md:4) references api/login.go — file-shaped nodes require a language that supports file globs
      /Users/alice/dev/billing: api (/Users/alice/dev/billing/STRATA.md:4) references api/handler.go — file-shaped nodes require a language that supports file globs
      """

  Scenario: Capsule uses .. to reference sibling bounded context
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ billing/
      │  ├─ STRATA.md
      │  ├─ usecase/
      │  │  └─ create_order.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ STRATA.md
         ├─ usecase/
         │  └─ register_user.go
         └─ domain/
            └─ user.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"] --> auth["../auth/usecase/**"]
        usecase --> domain["domain/**"]
      ```
      """
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"]
        domain["domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 4 files are encountered
    And 4 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: auth (/Users/jane/strata/billing/STRATA.md:3) references ../auth/usecase/** — ".." not allowed in node globs
      """

  Scenario: Scoped config has a node whose glob starts with .. (prefix, not standalone segment)
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ STRATA.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api/**"]
        kk["..domain/**"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: kk (/Users/jane/strata/billing/STRATA.md:4) references ..domain/** — ".." not allowed in node globs
      """

  Scenario: Scoped config has a node whose later segment starts with .. (prefix, not standalone segment)
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ STRATA.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["api/**"]
        kk["nested/..domain/**"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: kk (/Users/jane/strata/billing/STRATA.md:4) references nested/..domain/** — ".." not allowed in node globs
      """

  Scenario: Scoped config inside a capsule has overlapping node globs
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ STRATA.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain/**"]
        dowhatever["do*"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: node "domain" (/Users/jane/strata/billing/STRATA.md:3) and node "dowhatever" (/Users/jane/strata/billing/STRATA.md:4) overlap — file domain/order.go matches both globs
      """

  Scenario: Scoped config inside a capsule with duplicate node globs is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      └─ billing/
         ├─ STRATA.md
         ├─ api/
         │  └─ handler.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        domain["domain/**"]
        samething["domain/**"]
      ```
      """
    Given file "billing/api/handler.go" has content "package api"
    Given file "billing/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: glob "domain/**" claimed by multiple nodes: domain, samething (/Users/jane/strata/billing/STRATA.md:4)
      """

  Scenario: Parent STRATA.md declares cross-context edge between sibling capsules
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      ├─ billing/
      │  ├─ STRATA.md
      │  ├─ usecase/
      │  │  └─ create_order.go
      │  └─ domain/
      │     └─ order.go
      └─ auth/
         ├─ STRATA.md
         ├─ usecase/
         │  └─ register_user.go
         └─ domain/
            └─ user.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/**"] --> auth["auth/**"]
      ```
      """
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"] --> domain["domain/**"]
      ```
      """
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"]
        domain["domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 4 files are encountered
    And 4 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Parent capsule root allows import from nested capsule to sibling scope as single node
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      ├─ platform/
      │  ├─ STRATA.md
      │  ├─ billing/
      │  │  ├─ STRATA.md
      │  │  ├─ usecase/
      │  │  │  └─ create_order.go
      │  │  └─ domain/
      │  │     └─ order.go
      │  └─ shared/
      │     ├─ STRATA.md
      │     ├─ logging/
      │     │  └─ logger.go
      │     └─ database/
      │        └─ connection.go
      └─ shared/
         └─ utils.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        platform["platform/**"] --> shared_lib["shared/**"]
      ```
      """
    Given file "platform/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/**"] --> shared["shared/**"]
      ```
      """
    Given file "platform/billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"] --> domain["domain/**"]
      ```
      """
    Given file "platform/shared/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        logging["logging/**"]
        database["database/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 3 relations are examined
    And 5 files are encountered
    And 5 files are scanned
    And 0 violations are reported
    And 0 errors are reported

  Scenario: Parent capsule root denies import that intermediate scopes would allow
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      ├─ platform/
      │  ├─ STRATA.md
      │  ├─ billing/
      │  │  ├─ STRATA.md
      │  │  ├─ usecase/
      │  │  │  └─ create_order.go
      │  │  └─ domain/
      │  │     └─ order.go
      │  └─ shared/
      │     ├─ STRATA.md
      │     ├─ logging/
      │     │  └─ logger.go
      │     └─ database/
      │        └─ connection.go
      └─ utils/
         └─ utils.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        platform["platform/**"]
        utils["utils/**"]
      ```
      """
    Given file "platform/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/**"]
        shared["shared/**"]
      ```
      """
    Given file "platform/billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        usecase["usecase/**"] --> domain["domain/**"]
      ```
      """
    Given file "platform/shared/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        logging["logging/**"]
        database["database/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 3 relations are examined
    And 5 files are encountered
    And 5 files are scanned
    And 2 violations are reported
    And the violations are:
      """violations
      /Users/jane/strata: platform/billing/usecase/create_order.go:4:8 (billing) → platform/shared/logging (shared) — relation not allowed (add edge in /Users/jane/strata/platform/STRATA.md or move the file)
      /Users/jane/strata: platform/billing/usecase/create_order.go:5:8 (platform) → utils (utils) — relation not allowed (add edge in /Users/jane/strata/STRATA.md or move the file)
      """
    And 0 errors are reported

  Scenario: Shared root config load error is reported once across sibling capsules
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ STRATA.md
      ├─ auth/
      │  └─ go.mod
      └─ billing/
         └─ go.mod
      """
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        billing["billing/**"]
        shared_again["billing/**"]
      ```
      """
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata/auth: glob "billing/**" claimed by multiple nodes: billing, shared_again (/Users/jane/strata/STRATA.md:4)
      """

  Scenario: Malformed mermaid diagram in STRATA.md is reported as a parse error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: unrecognized mermaid line: app["internal/application/**" (/Users/jane/strata/STRATA.md:3)
      """

  Scenario: Overlapping node globs are reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ handlers/
         │  └─ create.go
         └─ services/
            └─ create.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handlers["internal/handlers/**"]
        services["internal/**"]
      ```
      """
    Given file "internal/handlers/create.go" has content "package handlers"
    Given file "internal/services/create.go" has content "package services"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: node "handlers" (/Users/jane/strata/STRATA.md:3) and node "services" (/Users/jane/strata/STRATA.md:4) overlap — file internal/handlers/create.go matches both globs
      """

  Scenario: Unclosed mermaid block is reported as a parse error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: unclosed ```mermaid block (/Users/jane/strata/STRATA.md)
      """

  Scenario: Missing mermaid block is reported as a parse error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      Some markdown without a mermaid block.
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: no ```mermaid block found (/Users/jane/strata/STRATA.md)
      """

  Scenario: Mermaid block with no nodes is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: mermaid block declared no nodes (/Users/jane/strata/STRATA.md)
      """

  Scenario: Invalid edge token is reported as a parse error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: invalid edge token "bad-name!" in line "app --> bad-name!" (/Users/jane/strata/STRATA.md:5)
      """

  Scenario: Node redefined with different glob is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        app["internal/domain/**"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: node "app" redefined with a different glob ("internal/application/**" vs "internal/domain/**") (/Users/jane/strata/STRATA.md:4)
      """

  Scenario: Duplicate node globs are reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        svc["internal/application/**"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: glob "internal/application/**" claimed by multiple nodes: app, svc (/Users/jane/strata/STRATA.md:4)
      """

  Scenario: Duplicate node globs do not suppress invalid node glob errors
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        svc["internal/application/**"]
        bad["../internal/domain/**"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given file "internal/domain/order.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 2 errors are reported
    And the errors are:
      """errors
       /Users/jane/strata: bad (/Users/jane/strata/STRATA.md:5) references ../internal/domain/** — ".." not allowed in node globs
       /Users/jane/strata: glob "internal/application/**" claimed by multiple nodes: app, svc (/Users/jane/strata/STRATA.md:4)
      """

  Scenario: Empty node glob is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         └─ application/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/app"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app[""]
        domain["internal/application/**"]
      ```
      """
    Given file "internal/application/order.go" has content "package application"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: node "app" has empty glob (/Users/jane/strata/STRATA.md:3)
      """

  Scenario: Edge line with fewer than two tokens is reported as a parse error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: edge has fewer than two nodes: "app -->" (/Users/jane/strata/STRATA.md:5)
      """

  Scenario: One capsule errors and another produces violations, both reported
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ billing/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ application/
      │  │  └─ order.go
      │  └─ domain/
      │     └─ order.go
      ├─ auth/
      │  ├─ go.mod
      │  ├─ STRATA.md
      │  ├─ api/
      │  │  └─ handler.go
      │  └─ domain/
      │     └─ auth.go
      """
    Given file "billing/go.mod" has content "module example.com/billing"
    Given file "auth/go.mod" has content "module example.com/auth"
    Given file "billing/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["application/**"]
        domain["domain/**"]
      ```
      """
    Given file "auth/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        handler["api/**"]
        domain["domain/**"]
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
    Given the filesystem is not permitted to read "auth/STRATA.md"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 1 relation is examined
    And 2 files are encountered
    And 2 files are scanned
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata/auth: read /Users/jane/strata/auth/STRATA.md: permission denied
      """
    And 1 violation is reported
    And the violation is:
      """violations
      /Users/jane/strata/billing: application/order.go:3:8 (app) → domain (domain) — relation not allowed (add edge in /Users/jane/strata/billing/STRATA.md or move the file)
      """

  Scenario: Cyclic dependency between nodes is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: cycle detected: app → domain → app (/Users/jane/strata/STRATA.md:6)
      """

  Scenario: Edge references undefined node is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: edge references undefined node "infrastructure" (/Users/jane/strata/STRATA.md:5)
      """

  Scenario: Multiple mermaid blocks in STRATA.md is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"] --> domain["internal/domain/**"]
      ```
      Some explanatory text between blocks.
      ```mermaid
      flowchart TD
        domain["internal/domain/**"]
        infra["internal/infrastructure/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: multiple ```mermaid blocks found (/Users/jane/strata/STRATA.md:6)
      """

  Scenario: Self-referencing edge is reported as an error
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      └─ internal/
         ├─ application/
         │  └─ order.go
         └─ domain/
            └─ order.go
      """
    Given file "go.mod" has content "module example.com/billing"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["internal/application/**"]
        domain["internal/domain/**"]
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
    When the check runs from "/Users/jane/strata"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 0 files are encountered
    And 0 files are scanned
    And 0 violations are reported
    And 1 error is reported
    And the error is:
      """errors
      /Users/jane/strata: edge references same node on both sides: domain → domain (/Users/jane/strata/STRATA.md:6)
      """

  Scenario: Files in a nested capsule are not double-reported by the parent capsule
    Given a fresh workspace at "/Users/jane/strata" with this layout:
      """tree
      ├─ go.mod
      ├─ STRATA.md
      ├─ core/
      │  └─ config.go
      └─ sub/
         ├─ go.mod
         ├─ STRATA.md
         └─ domain/
            └─ model.go
      """
    Given file "go.mod" has content "module example.com/root"
    Given file "STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        core["core/**"]
      ```
      """
    Given file "core/config.go" has content "package core"
    Given file "sub/go.mod" has content "module example.com/sub"
    Given file "sub/STRATA.md" has content:
      """config
      ```mermaid
      flowchart TD
        svc["service/**"]
      ```
      """
    Given file "sub/domain/model.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/strata"
    Then 2 capsules are discovered
    And 1 violation is reported
