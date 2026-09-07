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
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

// blockingLanguage blocks workers in ParseImports until unblock is closed,
// then returns an error. This allows precise control over when workers exit,
// enabling tests to create the exact conditions for a feeder goroutine leak.
type blockingLanguage struct {
	golangLang.Language
	unblock chan struct{}
}

func (l *blockingLanguage) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	<-l.unblock
	return nil, fmt.Errorf("parse error")
}

// TestWalk_ContextCancelledFeederDoesntLeak verifies that when the context
// is cancelled while workers are blocked in ParseImports, the feeder
// goroutine exits via ctx.Done() instead of blocking forever on workChan.
//
// Without the fix, the feeder send (workChan <- fw) has no ctx.Done() case,
// so when workers exit after cancellation the feeder is left blocking
// indefinitely — a goroutine leak.
func TestWalk_ContextCancelledFeederDoesntLeak(t *testing.T) {
	capsuleDir := "/capsule"

	fsys := memfs.New()
	// Create many files — more than workChan buffer (numWorkers*2) plus
	// what workers can drain while blocked in ParseImports (numWorkers).
	// Feeder can send numWorkers*3 items before blocking.
	numCPU := runtime.NumCPU()
	numFiles := numCPU*4 + 100
	for i := 0; i < numFiles; i++ {
		path := fmt.Sprintf("%s/file%d.go", capsuleDir, i)
		if err := fsys.WriteFile(path, []byte("package capsule\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	repo := &mermaid.MermaidRepository{}
	g, err := repo.Load("```mermaid\nflowchart TD\n  capsule[\"&ast;&ast;\"]\n```\n")
	if err != nil {
		t.Fatalf("Load graph: %v", err)
	}

	unblock := make(chan struct{})
	blkLang := &blockingLanguage{unblock: unblock}

	ch := &capsuleChecker{
		res:            &capsuleResult{},
		fsys:           fsys,
		capsule:        port.Capsule{Dir: capsuleDir},
		lang:           blkLang,
		contractDirAbs: capsuleDir,
		scopeCache:     newScopeCache(fsys, repo),
		parseCache:     newParseCache(),
		contractContext: contractContext{
			hasRootContract: true,
			rootGraphIndex:  graph.NewGraphIndex(g),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	before := runtime.NumGoroutine()

	done := make(chan error, 1)
	go func() { done <- ch.walk(ctx, fsys, capsuleDir) }()

	// Let workers start and block in ParseImports. Feeder fills the
	// workChan buffer, then blocks because workers can't consume more.
	time.Sleep(100 * time.Millisecond)

	// Cancel context. The feeder should observe ctx.Done() and exit.
	// Without the fix, the feeder blocks forever on workChan <- fw.
	cancel()

	// Unblock workers so they can finish, get the error, and exit.
	close(unblock)

	// The walk's own return value is not pinned: after cancellation a worker's
	// result may be dropped rather than delivered, so only its exit matters.
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("walk deadlocked: feeder goroutine blocked on workChan without ctx.Done()")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()
	leaked := after - before
	if leaked > 0 {
		t.Fatalf("walk leaked %d goroutines (before: %d, after: %d): feeder blocked on workChan without ctx.Done()", leaked, before, after)
	}
}

// TestRunWithContext_ContextCancellationNoLeak verifies that cancelling
// the context during RunWithContext does not leak goroutines. The feeder
// and worker goroutines must check ctx.Done() on their channel sends,
// otherwise they block forever after cancellation.
func TestRunWithContext_ContextCancellationNoLeak(t *testing.T) {
	numCapsules := runtime.NumCPU()*4 + 100
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

	unblock := make(chan struct{})
	blkLang := &blockingLanguage{unblock: unblock}

	repo := &mermaid.MermaidRepository{}
	discovery := service.NewCapsuleDiscovery()
	golangLang.Language{}.Register(discovery)

	ctx, cancel := context.WithCancel(context.Background())

	before := runtime.NumGoroutine()

	done := make(chan *port.CheckResult, 1)
	go func() {
		done <- RunWithContext(ctx, fsys, rootDir, []port.Language{blkLang}, repo, discovery)
	}()

	// Let workers start and block in ParseImports (inside checkCapsule -> walk).
	// The RunWithContext feeder fills workChan buffer (numWorkers*2) then blocks.
	time.Sleep(100 * time.Millisecond)

	// Cancel context. Both the RunWithContext feeder and the inner walk feeders
	// should observe ctx.Done() and exit.
	cancel()

	// Unblock workers so they can finish and exit.
	close(unblock)

	select {
	case result := <-done:
		if result == nil {
			t.Fatal("RunWithContext returned nil")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("RunWithContext deadlocked: goroutines blocked on channel sends without ctx.Done()")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()
	leaked := after - before
	if leaked > 0 {
		t.Fatalf("RunWithContext leaked %d goroutines (before: %d, after: %d): goroutines blocked on channel sends without ctx.Done()", leaked, before, after)
	}
}

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

	repo := &mermaid.MermaidRepository{}
	g, err := repo.Load("```mermaid\nflowchart TD\n  capsule[\"&ast;&ast;\"]\n```\n")
	if err != nil {
		t.Fatalf("Load graph: %v", err)
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
			hasRootContract: true,
			rootGraphIndex:  graph.NewGraphIndex(g),
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
