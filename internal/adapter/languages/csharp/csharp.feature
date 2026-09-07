Feature: C# language adapter with namespace mode

  Scenario: Namespace mode skips file-glob check for dotted namespace patterns
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  └─ Controller.cs
      └─ Domain/
         └─ Entity.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        domain["MyApp.Domain"]
        api --> domain
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using System;

      namespace MyApp.Api
      {
          public class Controller { }
      }
      """
    Given file "Domain/Entity.cs" has content:
      """csharp
      using System;
      using MyApp.Api;

      namespace MyApp.Domain
      {
          public class Entity { }
      }
       """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 0 errors and 1 violations are reported

  # Regression: file glob validation must fire in namespace mode.
  # Previously, validateLanguageGraph skipped file-glob errors when
  # NamespaceMode was true, allowing invalid patterns to pass silently.
  Scenario: File glob patterns in namespace mode still produce validation error
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  └─ Controller.cs
      └─ Domain/
         └─ Entity.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["Api/Controller.cs"]
        domain["MyApp.Domain"]
        api --> domain
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using System;
      using MyApp.Domain;

      namespace MyApp.Api
      {
          public class Controller { }
      }
      """
    Given file "Domain/Entity.cs" has content:
      """csharp
      using System;

      namespace MyApp.Domain
      {
          public class Entity { }
      }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 1 errors and 0 violations are reported
    And the errors are:
      """errors
      /Users/jane/myapp: api (/Users/jane/myapp/BAFT.md:4) references Api/Controller.cs — file-shaped nodes require a language that supports file globs
      """

 # Regression: ancestor contracts without namespace mode must not silently
  # suppress cross-scope violation detection. Previously, NodeForNamespace
  # was called on non-namespace-mode ancestor graphs, which always returned
  # "" (no match), causing violations to be missed. The fix skips ancestors
  # that don't have namespaceMode enabled.
  Scenario: Non-namespace-mode ancestor does not suppress root violation detection
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  ├─ BAFT.md
      │  └─ Controller.cs
      └─ Infra/
         └─ Database.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        domain["MyApp.Domain"]
        infra["MyApp.Infra"]
        api --> domain
      ```
      """
    Given file "Api/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        api["Api/&ast;&ast;"]
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using MyApp.Infra;

      namespace MyApp.Api
      {
          public class Controller { }
      }
      """
    Given file "Infra/Database.cs" has content:
      """csharp
      namespace MyApp.Infra
      {
          public class Database { }
      }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 0 errors and 2 violations are reported

  Scenario: Cross-scope namespace check allows permitted relation
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  ├─ BAFT.md
      │  └─ Controller.cs
      └─ Domain/
         └─ Entity.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        domain["MyApp.Domain"]
        api --> domain
      ```
      """
    Given file "Api/BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        domain["MyApp.Domain"]
        api --> domain
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using System;
      using MyApp.Domain;

      namespace MyApp.Api
      {
          public class Controller { }
      }
      """
    Given file "Domain/Entity.cs" has content:
      """csharp
      using System;

      namespace MyApp.Domain
      {
          public class Entity { }
      }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 0 errors and 0 violations are reported

  # Regression: multiple files in the same target namespace must produce only
  # one violation per disallowed import, not one per file.
  Scenario: Multiple files in target namespace produce single violation
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  └─ Controller.cs
      └─ Infra/
         ├─ Database.cs
         └─ Connection.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        infra["MyApp.Infra"]
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using System;
      using MyApp.Infra;

      namespace MyApp.Api
      {
          public class Controller { }
      }
      """
    Given file "Infra/Database.cs" has content:
      """csharp
      using System;

      namespace MyApp.Infra
      {
          public class Database { }
      }
      """
    Given file "Infra/Connection.cs" has content:
      """csharp
      using System;

      namespace MyApp.Infra
      {
          public class Connection { }
      }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 0 errors and 1 violations are reported

  Scenario: Namespace mode accepts wildcard namespace nodes
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  └─ Controller.cs
      └─ Domain/
         └─ Entity.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api.&ast;"]
        domain["MyApp.Domain.&ast;"]
        api --> domain
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using MyApp.Domain.Entities;

      namespace MyApp.Api.Controllers
      {
          public class Controller { }
      }
      """
    Given file "Domain/Entity.cs" has content:
      """csharp
      using MyApp.Api.Controllers;

      namespace MyApp.Domain.Entities
      {
          public class Entity { }
      }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 2 relations are examined
    And 0 errors and 1 violations are reported

  Scenario: Namespace mode without any declared namespace reports a diagnostic
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      └─ Api/
         └─ Controller.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        domain["MyApp.Domain"]
        api --> domain
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      public class Controller { }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 1 error and 0 violations are reported
    And the error is:
      """errors
      /Users/jane/myapp: namespaceMode is enabled but no scanned file declares a namespace (/Users/jane/myapp/BAFT.md)
      """

  Scenario: One using counts as one relation regardless of files in the namespace
    Given a fresh workspace at "/Users/jane/myapp" with this layout:
      """tree
      ├─ MyApp.csproj
      ├─ BAFT.md
      ├─ Api/
      │  └─ Controller.cs
      └─ Domain/
         ├─ Entity.cs
         └─ Value.cs
      """
    Given file "MyApp.csproj" has content "<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>"
    Given file "BAFT.md" has content:
      """config
      ```mermaid
      flowchart TD
        %% config namespaceMode "true"
        api["MyApp.Api"]
        domain["MyApp.Domain"]
        api --> domain
      ```
      """
    Given file "Api/Controller.cs" has content:
      """csharp
      using MyApp.Domain;

      namespace MyApp.Api
      {
          public class Controller { }
      }
      """
    Given file "Domain/Entity.cs" has content:
      """csharp
      namespace MyApp.Domain
      {
          public class Entity { }
      }
      """
    Given file "Domain/Value.cs" has content:
      """csharp
      namespace MyApp.Domain
      {
          public class Value { }
      }
      """
    Given the check uses the "csharp" language adapter
    When the check runs from "/Users/jane/myapp"
    Then 1 capsule is discovered
    And 1 relation is examined
    And 3 files are encountered and 3 files are scanned
    And 0 errors and 0 violations are reported
