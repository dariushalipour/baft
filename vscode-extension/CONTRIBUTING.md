# BAFT — VS Code Extension

Shows BAFT architecture violations as red squiggles and Problems panel entries by running the BAFT CLI and mapping its output to VS Code diagnostics.

## Prerequisites

- [Node.js](https://nodejs.org) (v18+)
- `baft` binary available in your `PATH`
- VS Code 1.85+

## How it works

On every save (immediately) and after edits stop (750 ms debounce), the extension runs `baft check --reporter=vsce .` from each workspace folder root. When there are dirty files in that folder, it adds `--overlay-stdin` and streams the unsaved buffer contents to the CLI so diagnostics reflect in-memory edits.

The extension parses the JSON array on stdout and publishes diagnostics. No architecture logic lives in the extension — the CLI is the single source of truth.

## Local development

```bash
npm install
```

Open the `vscode-extension/` folder in VS Code and press **F5** to launch an Extension Development Host. Violations in any open workspace folder will appear as red squiggles and in the Problems panel.

## Build a VSIX

Install the packaging tool:

```bash
npm install -g @vscode/vsce
```

Compile and package:

```bash
npm run compile
vsce package
```

This produces `baft-0.0.1.vsix` in the current directory.

## Install the VSIX manually

In VS Code:

1. Open the **Extensions** sidebar
2. Click the `...` menu (top-right of the sidebar)
3. Choose **Install from VSIX…**
4. Select the `.vsix` file

## Publish to the Marketplace

Follow the [VS Code publishing guide](https://code.visualstudio.com/api/working-with-extensions/publishing-extension):

```bash
vsce login dariushalipour
vsce publish
```
