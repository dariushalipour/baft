package check

import (
	"context"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	golangLang "github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

// TestRunWithContext_ReturnsContextCanceledError verifies that when the context is cancelled,
// the RunWithContext function returns a CheckResult containing the cancellation error,
// rather than a successful result.
func TestRunWithContext_ReturnsContextCanceledError(t *testing.T) {
	rootDir := "/root"
	fsys := memfs.New()

	// Setup a minimal capsule
	_ = fsys.WriteFile("/pkg1/go.mod", []byte("module example\n\ngo 1.21\n"), 0o644)
	_ = fsys.WriteFile("/pkg1/main.go", []byte("package main\n"), 0o644)
	_ = fsys.WriteFile("/pkg1/BAFT.md", []byte("# Nodes\n\n## main\nmain/**\n"), 0o644)

	repo := &mermaid.MermaidRepository{}
	discovery := service.NewCapsuleDiscovery()
	golangLang.Language{}.Register(discovery)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := RunWithContext(ctx, fsys, rootDir, []port.Language{golangLang.Language{}}, repo, discovery)

	// Verify that the error is actually reported
	foundCancellationError := false
	for _, errStr := range result.Errors {
		if strings.Contains(errStr, context.Canceled.Error()) {
			foundCancellationError = true
			break
		}
	}

	if !foundCancellationError {
		t.Errorf("expected error containing %q, but got %v", context.Canceled.Error(), result.Errors)
	}
}

// TestWalk_ReturnsContextCanceledError verifies that the internal walk function
// also returns the cancellation error correctly.
func TestWalk_ReturnsContextCanceledError(t *testing.T) {
	capsuleDir := "/capsule"
	fsys := memfs.New()
	_ = fsys.WriteFile(capsuleDir+"/go.mod", []byte("module example\n\ngo 1.21\n"), 0o644)
	_ = fsys.WriteFile(capsuleDir+"/main.go", []byte("package main\n"), 0o644)
	_ = fsys.WriteFile(capsuleDir+"/BAFT.md", []byte("# Nodes\n\n## main\nmain/**\n"), 0o644)

	repo := &mermaid.MermaidRepository{}
	lang := golangLang.Language{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := (&capsuleChecker{
		res:            &capsuleResult{},
		fsys:           fsys,
		capsule:        port.Capsule{Dir: capsuleDir},
		lang:           lang,
		contractDirAbs: capsuleDir,
		scopeCache:     newScopeCache(fsys, repo),
		parseCache:     newParseCache(),
	}).walk(ctx, capsuleDir)

	if err == nil {
		t.Fatal("expected error from walk due to context cancellation, got nil")
	}

	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("expected error %q, got %q", context.Canceled.Error(), err.Error())
	}
}
