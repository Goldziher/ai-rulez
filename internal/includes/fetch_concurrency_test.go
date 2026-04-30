package includes

import (
	"sync"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestLockForFetch_SameKeyReturnsSameMutex(t *testing.T) {
	a := lockForFetch("/some/cache/dir")
	b := lockForFetch("/some/cache/dir")
	if a != b {
		t.Fatalf("lockForFetch returned distinct mutexes for the same key")
	}
}

func TestLockForFetch_DifferentKeysReturnDistinctMutexes(t *testing.T) {
	a := lockForFetch("/cache/one")
	b := lockForFetch("/cache/two")
	if a == b {
		t.Fatalf("lockForFetch returned same mutex for different keys")
	}
}

func TestLockForFetch_ConcurrentAccessIsSafe(t *testing.T) {
	// Hammering lockForFetch from many goroutines must not race.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := lockForFetch("/parallel/key")
			m.Lock()
			//nolint:staticcheck // intentional bare lock/unlock to exercise the registry
			m.Unlock()
		}()
	}
	wg.Wait()
}

func TestScannedTreeCache_StoreAndRetrieve(t *testing.T) {
	dir := t.TempDir()

	if got := cachedScan(dir); got != nil {
		t.Fatalf("expected empty cache, got %#v", got)
	}

	tree := &config.ContentTree{
		Domains: map[string]*config.Domain{
			"d1": {Name: "d1"},
		},
	}
	storeScan(dir, tree)

	got := cachedScan(dir)
	if got == nil {
		t.Fatalf("expected cached tree after storeScan, got nil")
	}
	if got != tree {
		t.Fatalf("cachedScan returned a different pointer than was stored")
	}

	invalidateScan(dir)
	if got := cachedScan(dir); got != nil {
		t.Fatalf("expected nil after invalidate, got %#v", got)
	}
}

func TestScannedTreeCache_KeyedByPath(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	treeA := &config.ContentTree{}
	treeB := &config.ContentTree{}
	storeScan(a, treeA)
	storeScan(b, treeB)

	if cachedScan(a) != treeA || cachedScan(b) != treeB {
		t.Fatal("scanned-tree cache mixed up entries between keys")
	}

	invalidateScan(a)
	if cachedScan(a) != nil {
		t.Fatal("invalidate(a) did not clear a")
	}
	if cachedScan(b) != treeB {
		t.Fatal("invalidate(a) incorrectly cleared b")
	}
	invalidateScan(b)
}

func TestScannedTreeCache_ConcurrentStoreAndRead(t *testing.T) {
	// Race-detector-friendly stress: many readers and writers on distinct keys.
	const goroutines = 50
	keys := make([]string, goroutines)
	for i := range keys {
		keys[i] = t.TempDir()
	}

	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(2)
		go func() {
			defer wg.Done()
			storeScan(k, &config.ContentTree{})
		}()
		go func() {
			defer wg.Done()
			_ = cachedScan(k)
		}()
	}
	wg.Wait()

	for _, k := range keys {
		invalidateScan(k)
	}
}
