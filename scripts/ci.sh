#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VSCODE_DIR="$REPO_ROOT/vscode-extension"
JETBRAINS_DIR="$REPO_ROOT/intellij-plugin"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓${NC} $1"; }
fail() { echo -e "${RED}✗${NC} $1"; exit 1; }
header() { echo -e "${YELLOW}→ $1${NC}"; }

errors=0

# ── Go ────────────────────────────────────────────────────────────────
cd "$REPO_ROOT"

header "go fmt"
if unformatted=$(go fmt ./...); then
    if [ -n "$unformatted" ]; then
        fail "go fmt found unformatted files:\n$unformatted"
    fi
    pass "go fmt"
else
    fail "go fmt"
fi

header "staticcheck"
if staticcheck ./... 2>&1; then
    pass "staticcheck"
else
    fail "staticcheck"
fi

header "go test"
if go test ./... 2>&1; then
    pass "go test"
else
    fail "go test"
fi

# ── VSCode Extension ──────────────────────────────────────────────────
if [ -d "$VSCODE_DIR" ]; then
    cd "$VSCODE_DIR"

    header "vscode tsc"
    if npm run compile 2>&1; then
        pass "vscode tsc"
    else
        fail "vscode tsc"
    fi

    header "vscode vitest"
    if npm test 2>&1; then
        pass "vscode vitest"
    else
        fail "vscode vitest"
    fi
else
    header "vscode extension (skipped — directory not found)"
fi

# ── JetBrains Plugin ──────────────────────────────────────────────────
if [ -d "$JETBRAINS_DIR" ]; then
    cd "$JETBRAINS_DIR"

    header "intellij compile"
    if ./gradlew compileKotlin 2>&1; then
        pass "intellij compile"
    else
        fail "intellij compile"
    fi

    header "intellij test"
    if ./gradlew test 2>&1; then
        pass "intellij test"
    else
        fail "intellij test"
    fi
else
    header "intellij plugin (skipped — directory not found)"
fi

echo ""
pass "All checks passed"
