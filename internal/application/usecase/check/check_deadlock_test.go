package check

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	golangLang "github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

// TestWalk_NoDeadlockWhenFilesExceedChannelBuffer guards against a deadlock
// where workChan is filled synchronously before any workers are started.
// The buffer is numWorkers*2; if len(filesToCheck) > buffer, the send blocks
// with no consumers running.
func TestWalk_NoDeadlockWhenFilesExceedChannelBuffer(t *testing.T) {
	numFiles := runtime.NumCPU()*2 + 1
	capsuleDir := "/capsule"

	fsys := memfs.New()
	for i := 0; i < numFiles; i++ {
		path := fmt.Sprintf("%s/file%d.go", capsuleDir, i)
		if err := fsys.WriteFile(path, []byte("package capsule\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	ch := &capsuleChecker{
		res:            &capsuleResult{},
		fsys:           fsys,
		capsule:        port.Capsule{Dir: capsuleDir},
		lang:           golangLang.Language{},
		contractDirAbs: capsuleDir,
		scopeCache:     newScopeCache(fsys, &mermaid.MermaidRepository{}),
		parseCache:     newParseCache(),
		contractContext: contractContext{
			hasRootContract: false,
		},
	}

	done := make(chan error, 1)
	go func() { done <- ch.walk(context.Background(), fsys, capsuleDir) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("walk returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("walk deadlocked: workChan is filled before workers are started")
	}
}

// TestRunWithContext_NoDeadlockWhenCapsulesExceedChannelBuffer guards the same
// class of bug in RunWithContext's outer worker pool.
func TestRunWithContext_NoDeadlockWhenCapsulesExceedChannelBuffer(t *testing.T) {
	numCapsules := runtime.NumCPU()*2 + 1
	rootDir := "/root"

	fsys := memfs.New()
	for i := 0; i < numCapsules; i++ {
		capsuleDir := fmt.Sprintf("%s/pkg%d", rootDir, i)
		goMod := fmt.Sprintf("module example.com/pkg%d\n\ngo 1.21\n", i)
		if err := fsys.WriteFile(capsuleDir+"/go.mod", []byte(goMod), 0o644); err != nil {
			t.Fatalf("WriteFile go.mod: %v", err)
		}
		if err := fsys.WriteFile(fmt.Sprintf("%s/main.go", capsuleDir), []byte("package main\n"), 0o644); err != nil {
			t.Fatalf("WriteFile main.go: %v", err)
		}
		baft := "# Nodes\n\n## main\nmain/**\n"
		if err := fsys.WriteFile(capsuleDir+"/BAFT.md", []byte(baft), 0o644); err != nil {
			t.Fatalf("WriteFile BAFT.md: %v", err)
		}
	}

	lang := golangLang.Language{}
	repo := &mermaid.MermaidRepository{}
	discovery := service.NewCapsuleDiscovery()
	lang.Register(discovery)

	done := make(chan *port.CheckResult, 1)
	go func() {
		done <- RunWithContext(context.Background(), fsys, rootDir, []port.Language{lang}, repo, discovery)
	}()

	select {
	case result := <-done:
		if result == nil {
			t.Fatal("RunWithContext returned nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunWithContext deadlocked: workChan is filled before workers are started")
	}
}
