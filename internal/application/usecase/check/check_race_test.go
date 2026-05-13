package check

import (
	"sync"
	"testing"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

func TestCapsuleChecker_ConcurrentMerge(t *testing.T) {
	ch := &capsuleChecker{
		res: &capsuleResult{graph: &graph.Graph{}},
	}

	numWorkers := 10
	results := make(chan fileCheckResult, numWorkers*2)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := fileCheckResult{
				filesEncountered: 1,
				filesScanned:     1,
				relations:        2,
				violations:       nil,
			}
			results <- res
		}()
	}
	wg.Wait()
	close(results)

	// Single consumer merges results (mirrors production checkCapsule pattern).
	for res := range results {
		ch.mergeFileResult(res)
	}

	if ch.res.filesEncountered != 10 {
		t.Errorf("expected 10 files encountered, got %d", ch.res.filesEncountered)
	}
	if ch.res.filesScanned != 10 {
		t.Errorf("expected 10 files scanned, got %d", ch.res.filesScanned)
	}
	if ch.res.relations != 20 {
		t.Errorf("expected 20 relations, got %d", ch.res.relations)
	}
}
