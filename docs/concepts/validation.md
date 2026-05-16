# Validation

Baft checks architecture in stages. That split matters because not every problem means the same thing.

Some problems mean Baft could not read the contract text at all. Some mean the contract text was readable, but the contract itself is invalid. Some mean the contract is structurally valid, but the selected language does not support one of its node shapes. Those are different failures, and they are reported differently.

This document describes how that validation works.

---

## The broader model

The `check` command exists primarily to answer whether the codebase complies with its contract files.

To answer that, Baft produces two different kinds of output during a check run:

- **Contract diagnostics** tell you that the BAFT.md itself is malformed, structurally invalid, or incompatible with the current language.
- **Relation violations** tell you that the source code imports something the contract does not allow.

The three categories in this document apply to **contract diagnostics**, not to relation violations.

The high-level flow is:

1. Read the BAFT.md contract text.
2. Validate the contract itself.
3. Validate language-specific constraints.
4. Compare real imports against the contract.

That flow gives Baft a simple rule about how contract diagnostics affect the main compliance check:

- If parsing fails, there is no usable graph.
- If contract validation fails, the graph may still be usable.
- If language validation fails, the graph may still be usable.

That is why some errors should stop relation checking and others should not.

---

## The three categories

### 1. Parser-local, fatal

This category answers one question: **did Baft successfully read the Mermaid contract text?**

These errors happen while Baft is reading the contract text. They are fatal because there is no trustworthy contract map to continue with.

Examples:

- malformed Mermaid lines
- invalid node syntax
- invalid edge syntax
- missing Mermaid block
- empty graph with no nodes

Properties of parser-local errors:

- They happen before contract validation.
- They prevent later validation stages from running on that contract.
- They should return no usable graph.

In short: **Baft could not read the contract into a usable structure.**

### 2. Contract validation, graph-based

This category answers a different question: **given a readable contract, is it a valid BAFT contract?**

These errors happen after the contract text has been read successfully. At this point Baft already has a usable map of the declared nodes and allowed directions. The contract may still be invalid, but the text itself was readable.

Examples:

- cycles
- undefined edge nodes
- duplicate globs
- overlapping node globs
- invalid node globs such as `..`
- empty globs

Properties of contract-validation errors:

- They run after the contract text has been read.
- They should preserve the usable contract map.
- They may coexist with relation violations in the same check result.

In short: **Baft understood the contract text, but the contract itself is invalid.**

### 3. Language and business validation

This category answers a third question: **is this contract valid for the selected language and checking context?**

These errors happen when the contract uses a shape that the selected language does not support, even though the contract itself is otherwise readable and structurally valid.

Examples:

- file-shaped nodes require a language that supports file globs

Properties of language and business validation errors:

- They are not parser errors.
- They are not contract-structure errors.
- They depend on the active language or the checking context.
- They may coexist with relation violations in the same result.

In short: **the contract may be structurally valid, but it is not valid for this language or validation context.**

---

## Why the split matters

Without this split, Baft tends to overload one bucket of errors and then make the wrong control-flow decision.

The practical consequences are:

- parser failures should stop relation checking
- contract-validation errors should usually not stop relation checking
- language-validation errors should usually not stop relation checking

That distinction is what allows a single run to report both of these at once:

- a contract problem such as a cycle
- a source-code violation such as `app` importing `api` without an allowed edge

This is the difference between "Baft could not understand the contract" and "Baft understood the contract and found multiple problems."

---

## What Baft Reports

Baft applies this split as follows:

- **Mermaid parsing** reports parser-local fatal errors and returns no usable graph when parsing fails.
- **Contract validation** reports empty globs, undefined edge nodes, cycles, duplicate globs, invalid node globs, and overlapping directory globs.
- **Language validation** reports rules such as unsupported file-shaped nodes in languages that do not allow them.

That means a `check` run can report both of these at once when a usable contract map exists:

- contract diagnostics such as a cycle or undefined edge node
- source-code violations such as an import that is not allowed by the contract

---

## Summary

The three categories are:

- **Parser-local, fatal**: the graph could not be built.
- **Contract validation, graph-based**: the graph exists, but the contract is invalid.
- **Language and business validation**: the contract is valid as a graph, but invalid for the active language or check context.

That separation keeps the meaning of each error clear and lets one run report both contract problems and source-level violations when the contract is still usable.