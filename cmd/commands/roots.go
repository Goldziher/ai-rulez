package commands

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

// rootRun is what one root's work produced and how long it took.
type rootRun[T any] struct {
	value   T
	elapsed time.Duration
}

// forEachRoot runs work for every path on a pool of at most GOMAXPROCS
// goroutines and calls emit, on the calling goroutine and in input order, with
// each result as soon as every earlier root has been emitted. A slow first root
// therefore delays printing but never reorders it, and emit needs no lock to
// aggregate: it is never called concurrently. work must not write to the
// terminal itself (it runs concurrently); what it wants said goes in its result.
func forEachRoot[T any](paths []string, work func(path string) T, emit func(index int, path string, value T, elapsed time.Duration)) {
	if len(paths) == 0 {
		return
	}
	workers := min(runtime.GOMAXPROCS(0), len(paths))
	done := make([]chan rootRun[T], len(paths))
	for i := range done {
		done[i] = make(chan rootRun[T], 1)
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				start := time.Now()
				v := work(paths[i])
				done[i] <- rootRun[T]{value: v, elapsed: time.Since(start)}
			}
		}()
	}
	go func() {
		for i := range paths {
			next <- i
		}
		close(next)
	}()
	for i, path := range paths {
		r := <-done[i]
		emit(i, path, r.value, r.elapsed)
	}
	wg.Wait()
}

// reportRootProgress writes the "[3/12] path (1.2s)" line a recursive run prints
// as each root finishes. It goes to stderr and is silent under --quiet and a
// structured --format, the same condition as the other progress notices.
func reportRootProgress(index, total int, path string, elapsed time.Duration) {
	if progress.IsQuiet() {
		return
	}
	fmt.Fprintf(os.Stderr, "[%d/%d] %s (%s)\n", index+1, total, path, elapsed.Round(time.Millisecond))
}
