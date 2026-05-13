Feature: TypeScript language adapter integration with architecture check

  Scenario: .test.ts and .spec.ts files are excluded from architecture checks
    Given a fresh workspace at "/Users/jane/baft" with this layout:
      """tree
      ├─ package.json
      ├─ BAFT.md
      └─ src/
         ├─ application/
         │  ├─ order.ts
         │  ├─ order.test.ts
         │  └─ order.spec.ts
         └─ domain/
            └─ model.ts
      """
    Given file "package.json" has content '{"name": "billing"}'
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        app["src/application/&ast;&ast;"]
        domain["src/domain/&ast;&ast;"]
      ```
      """
    Given file "src/application/order.ts" has content "export const order = {};"
    Given file "src/application/order.test.ts" has content:
      """typescript
      import { model } from '../domain/model';
      """
    Given file "src/application/order.spec.ts" has content:
      """typescript
      import { model } from '../domain/model';
      """
    Given file "src/domain/model.ts" has content "export const model = {};"
    Given the check uses the "typescript" language adapter
    When the check runs from "/Users/jane/baft"
    Then 1 capsule is discovered
    And 0 relations are examined
    And 2 files are encountered and 2 files are scanned
    And 0 errors and 0 violations are reported
