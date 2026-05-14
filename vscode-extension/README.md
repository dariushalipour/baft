# 🧶 Baft for VS Code

Fast, multilingual architecture enforcement from Mermaid diagrams, surfaced directly in VS Code.

This extension does not implement architecture rules itself. It runs the [BAFT](https://github.com/dariushalipour/baft) CLI, reads its diagnostics, and turns violations into red squiggles and Problems entries.

## What You Get

- Automatic diagnostics for projects that have a `BAFT.md`
- Live updates while you type, including unsaved changes
- `Format Document` support for `BAFT.md`
- Configurable formatting palette with `baft.format.colorPalette`
- The CLI stays the single source of truth

## Requirements

- VS Code 1.85+
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

On save, and shortly after edits stop, the extension runs:

```bash
baft check --reporter=vsce .
```

When a file is dirty, the extension adds `--overlay-stdin` and streams the current unsaved buffer contents to the CLI. That keeps diagnostics accurate without requiring you to save first.

When you format a `BAFT.md`, the extension runs:

```bash
baft restyle --stdin --path /absolute/path/to/BAFT.md --color-palette <name>
```

The formatter only targets `BAFT.md` files and restyles the active document instead of walking the whole workspace.

## Usage

Open a workspace that contains a supported project and a `BAFT.md`. Violations appear automatically in the editor and in the Problems panel.

To restyle a contract, run `Format Document` on a `BAFT.md` or enable format on save for your BAFT workflow. The palette defaults to `vibrant` and can be changed with `baft.format.colorPalette`.

## Troubleshooting

**"BAFT: binary not found in PATH"**

VS Code cannot find `baft`. Install it with `go install` and make sure the directory containing the binary is in `PATH`.
