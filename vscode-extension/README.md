# BAFT for VS Code

Architecture violation diagnostics powered by the [BAFT](https://github.com/dariushalipour/baft) CLI.

Violations appear as red squiggles in the editor and as entries in the Problems panel — no configuration required beyond having `baft` installed.

---

## Requirements

- **VS Code** 1.85+
- **`baft` CLI** installed and available in your `PATH`

---

## Install the CLI

```bash
go install github.com/dariushalipour/baft@latest
```

This places the `baft` binary in your Go bin directory (usually `$HOME/go/bin`).

### Make sure `baft` is in your PATH

Add the Go bin directory to your shell's `PATH` if you haven't already.

**zsh / bash** — add to `~/.zshrc` or `~/.bashrc`:

```bash
export PATH="$HOME/go/bin:$PATH"
```

**fish** — add to `~/.config/fish/config.fish`:

```fish
fish_add_path $HOME/go/bin
```

Then reload your shell or open a new terminal and verify:

```bash
baft --version
```

---

## How it works

On every save, and 750 ms after edits stop, the extension runs `baft check --reporter=vsce .` from the workspace root. When there are dirty files in that workspace folder, it adds `--overlay-stdin` and streams the current unsaved buffer contents to the CLI so diagnostics stay live while you type.

The extension parses the JSON output and publishes diagnostics. No architecture logic lives in the extension — the CLI is the single source of truth.

---

## Usage

Open any project that has a `BAFT.md` manifest. Violations appear automatically. No commands to run, no settings to configure.

---

## Troubleshooting

**"BAFT: binary not found in PATH"**

The extension cannot find `baft`. Install it with `go install` (see above) and ensure `$HOME/go/bin` is in your `PATH`. If you installed it elsewhere, add that directory to `PATH` instead.
