# BAFT for IntelliJ

Fast, multilingual architecture enforcement from Mermaid diagrams, surfaced directly in IntelliJ.

This plugin does not implement architecture rules itself. It runs the [BAFT](https://github.com/dariushalipour/baft) CLI, reads its diagnostics, and turns violations into editor annotations.

## What You Get

- Automatic diagnostics for projects that have a `BAFT.md`
- Live updates while you type, including unsaved files
- No plugin-specific architecture logic or duplicate rule system
- The CLI stays the single source of truth

## Requirements

- IntelliJ IDEA 2024.1+ or another IntelliJ-based IDE
- `baft` installed and available in `PATH`

## Install BAFT

```bash
go install github.com/dariushalipour/baft@latest
```

That usually installs `baft` into `$HOME/go/bin`.

If needed, add it to `PATH`.

For `zsh` or `bash`:

```bash
export PATH="$HOME/go/bin:$PATH"
```

For `fish`:

```fish
fish_add_path $HOME/go/bin
```

Then verify:

```bash
baft --version
```

## How It Works

The plugin runs:

```bash
baft check --reporter=intellij .
```

When a file is unsaved, the plugin adds `--overlay-stdin` and streams the current in-memory document contents to the CLI. That keeps diagnostics aligned with what is on screen, not just what is on disk.

## Usage

Open a project that contains a supported module and a `BAFT.md`. Violations appear automatically as annotations in the editor and as entries in the Problems tool window.

## Troubleshooting

**"BAFT: binary not found in PATH"**

The plugin cannot find `baft`. Install it with `go install` and make sure the directory containing the binary is in `PATH`.
