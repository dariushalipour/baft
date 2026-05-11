# STRATA for IntelliJ

Architecture violation diagnostics powered by the [STRATA](https://github.com/dariushalipour/strata) CLI.

Violations appear as red squiggles in the editor and as entries in the Problems tool window — no configuration required beyond having `strata` installed.

---

## Requirements

- **IntelliJ IDEA** 2024.1+ (or any IntelliJ-based IDE)
- **`strata` CLI** installed and available in your `PATH`

---

## Install the CLI

```bash
go install github.com/dariushalipour/strata@latest
```

This places the `strata` binary in your Go bin directory (usually `$HOME/go/bin`).

### Make sure `strata` is in your PATH

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
strata --version
```

---

## How it works

The plugin uses IntelliJ's `ExternalAnnotator` pipeline to run `strata check --reporter=intellij .` from the project root. When there are unsaved files in the project, it adds `--overlay-stdin` and streams the current in-memory document contents to the CLI so diagnostics stay live while you type.

The plugin parses the JSON output and publishes annotations. No architecture logic lives in the plugin — the CLI is the single source of truth.

---

## Usage

Open any project that has a `STRATA.md` manifest. Violations appear automatically as red squiggles and refresh as you edit. Click any squiggle to see the rule name and message; the Problems tool window lists all violations across the project.

---

## Troubleshooting

**"STRATA: binary not found in PATH"**

The plugin cannot find `strata`. Install it with `go install` (see above) and ensure `$HOME/go/bin` is in your `PATH`. If you installed it elsewhere, add that directory to `PATH` instead.
