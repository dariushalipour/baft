Feature: Go language adapter integration with architecture check

  Scenario: *_test.go files are excluded from architecture checks
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ go.mod
      ├─ BAFT.md
      └─ internal/
         ├─ application/
         │  ├─ order.go
         │  └─ order_test.go
         └─ domain/
            └─ model.go
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
    Given file "internal/application/order.go" has content "package application"
    Given file "internal/application/order_test.go" has content:
      """go
      package application_test

      import "example.com/billing/internal/domain"
      """
    Given file "internal/domain/model.go" has content "package domain"
    Given the check uses the "go" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported
