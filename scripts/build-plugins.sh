#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VSCODE_DIR="$REPO_ROOT/vscode-extension"
JETBRAINS_DIR="$REPO_ROOT/intellij-plugin"
EMBEDDED_VSCODE_DIR="$REPO_ROOT/internal/integrations/embedded/vscode"
EMBEDDED_JETBRAINS_DIR="$REPO_ROOT/internal/integrations/embedded/jetbrains"
EMBEDDED_VSCODE_PACKAGE="$EMBEDDED_VSCODE_DIR/baft-vscode.vsix"
EMBEDDED_JETBRAINS_PACKAGE="$EMBEDDED_JETBRAINS_DIR/baft-intellij.zip"

mkdir -p "$EMBEDDED_VSCODE_DIR" "$EMBEDDED_JETBRAINS_DIR"

cd "$VSCODE_DIR"
npm install
npm run compile
vsce package
VSCODE_PACKAGE="$(ls -t baft-*.vsix | head -n 1)"
mv "$VSCODE_PACKAGE" "$EMBEDDED_VSCODE_PACKAGE"

cd "$JETBRAINS_DIR"
./gradlew buildPlugin
JETBRAINS_PACKAGE="$(ls -t build/distributions/baft-intellij-*.zip | head -n 1)"
mv "$JETBRAINS_PACKAGE" "$EMBEDDED_JETBRAINS_PACKAGE"

echo "Embedded plugin assets updated:"
echo "- $EMBEDDED_VSCODE_PACKAGE"
echo "- $EMBEDDED_JETBRAINS_PACKAGE"
