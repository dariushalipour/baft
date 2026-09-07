# Validation

`check` exists to answer one question — does the code comply with its contracts? — and produces two kinds of output on the way:

- **Relation violations** — the source code imports something the contract does not allow.
- **Contract diagnostics** — the contract file itself is unreadable, invalid, or unsupported for the active language.

Every rule id for both is tabulated in [the manual](../manual.md#handling-violations); this page covers only why contract diagnostics come in three categories, and what that changes.

---

## The three categories

| Category                   | Question it answers                                  | Examples                                                                                          | Effect on the run                                    |
| -------------------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| **Parser-local, fatal**    | Did Baft read the Mermaid text at all?               | missing or duplicated mermaid block, malformed node or edge line, empty graph                     | No usable graph — relation checking stops for that scope |
| **Contract validation**    | Given readable text, is it a valid contract?         | cycles, undefined edge nodes, duplicate or overlapping globs, `..` in a glob, empty globs         | Graph stays usable — relation checking continues      |
| **Language and business**  | Is this contract valid for the active language?      | a file-shaped node where `SupportsFileGlobs()` is false, `namespaceMode` with no declared namespaces | Graph stays usable — relation checking continues      |

---

## Why the split matters

Only the parser-local category may suppress relation checking. Fold the other two into it and a contract nit hides every real source violation behind it; fold it into them and Baft judges code against a graph it never built.

Because the last two preserve the graph, one run can report a contract problem such as a cycle *and* a source violation such as `app` importing `api` without an edge. That is the difference between "Baft could not understand the contract" and "Baft understood the contract and found several problems."
