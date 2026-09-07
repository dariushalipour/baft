package check

import (
	"context"
	"runtime"
	"sync"
)

// runParallel feeds items to a bounded pool of workers and streams whatever
// they emit on the returned channel, which closes once every worker is done.
// The feeder always closes the work channel, so a cancelled context unwinds
// the pool instead of stranding workers on an open channel; emit reports false
// once the context is done, telling a worker to stop early.
func runParallel[T, R any](ctx context.Context, items []T, worker func(in <-chan T, emit func(R) bool)) <-chan R {
	numWorkers := min(runtime.NumCPU(), len(items))
	work := make(chan T, numWorkers*2)
	results := make(chan R, numWorkers*2)

	go func() {
		defer close(work)
		for _, item := range items {
			select {
			case work <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(work, func(r R) bool {
				select {
				case results <- r:
					return true
				case <-ctx.Done():
					return false
				}
			})
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
