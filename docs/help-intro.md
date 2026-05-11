Baft checks architecture contracts declared in `BAFT.md`.

A `BAFT.md` file contains one Mermaid flowchart that:
- defines nodes with globs
- defines allowed imports with arrows
- can mark nodes `:::endophobic` to forbid same-node imports

Typical workflow:
1. Write `BAFT.md` by hand, or generate a draft with `baft draft .`
2. Run `baft check .`
3. Fix violations by adding edges or moving code

Supported languages: Go, TypeScript, Dart, Kotlin, Rust.

Full local manual: `baft manual`