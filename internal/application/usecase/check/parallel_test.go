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
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

// TestRunParallel_CancelDrainsPool verifies the pool unwinds when the context
// is cancelled while workers keep consuming: the feeder stops feeding, closes
// the work channel anyway, and the results channel closes.
func TestRunParallel_CancelDrainsPool(t *testing.T) {
	items := make([]int, 10_000)
	ctx, cancel := context.WithCancel(context.Background())

	results := runParallel(ctx, items, func(in <-chan int, emit func(int) bool) {
		for item := range in {
			emit(item)
		}
	})

	<-results
	cancel()

	done := make(chan struct{})
	go func() {
		for range results {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("results channel never closed after cancellation")
	}
}

// slowLanguage blocks in ParseImports until unblock is closed and then
// succeeds, so every worker stays alive after cancellation instead of
// returning early with an error.
type slowLanguage struct {
	golangLang.Language
	unblock chan struct{}
}

func (l *slowLanguage) ParseImports(port.FileSystem, string) ([]port.ImportSpec, error) {
	<-l.unblock
	return nil, nil
}

// TestWalk_CancelMidWalkWithSucceedingWorkers reproduces the deadlock the
// shared pool fixes: with workers that keep succeeding, a feeder that returned
// on ctx.Done() without closing the work channel left every worker ranging
// over a channel that never closed, so walk never returned.
func TestWalk_CancelMidWalkWithSucceedingWorkers(t *testing.T) {
	capsuleDir := "/capsule"

	fsys := memfs.New()
	for i := 0; i < runtime.NumCPU()*4+100; i++ {
		if err := fsys.WriteFile(fmt.Sprintf("%s/file%d.go", capsuleDir, i), []byte("package capsule\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	repo := &mermaid.MermaidRepository{}
	g, err := repo.Load("```mermaid\nflowchart TD\n  capsule[\"&ast;&ast;\"]\n```\n")
	if err != nil {
		t.Fatalf("Load graph: %v", err)
	}

	unblock := make(chan struct{})
	ch := &capsuleChecker{
		res:            &capsuleResult{},
		fsys:           fsys,
		capsule:        port.Capsule{Dir: capsuleDir},
		lang:           &slowLanguage{unblock: unblock},
		contractDirAbs: capsuleDir,
		scopeCache:     newScopeCache(fsys, repo),
		parseCache:     newParseCache(),
		contractContext: contractContext{
			hasRootContract: true,
			rootGraphIndex:  graph.NewGraphIndex(g),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ch.walk(ctx, fsys, capsuleDir) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	close(unblock)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("walk returned unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("walk deadlocked: cancelled feeder left workers ranging over an unclosed channel")
	}
}
