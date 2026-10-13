package lint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func TestLoaderConcurrentSameRootLoadsOnce(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o600))
	loader := &Loader{}
	const workers = 32
	trees := make([]*Tree, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})

	// Act
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			trees[i], errs[i] = loader.Load(dir)
		}()
	}
	close(start)
	wg.Wait()

	// Assert
	for i := range workers {
		require.NoError(t, errs[i])
		assert.Same(t, trees[0], trees[i], "every caller shares the one loaded tree")
	}
}

func TestLoaderLoadsEachKeyOnce(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	var loads atomic.Int32
	loader := &Loader{load: func(ctx context.Context, base, root string) (*Tree, error) {
		loads.Add(1)
		time.Sleep(10 * time.Millisecond)
		return LoadTreeContext(ctx, gitutil.Git{}, base, root)
	}}

	// Act
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := loader.Load(dir)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// Assert
	assert.Equal(t, int32(1), loads.Load())
}

// Different repositories load in parallel: a slow load of one must not hold
// the other behind it.
func TestLoaderLoadsDifferentKeysConcurrently(t *testing.T) {
	// Arrange: each load waits for the other to have started, so both can only
	// finish when the loads overlap.
	a, b := t.TempDir(), t.TempDir()
	started := make(chan string, 2)
	release := make(chan struct{})
	loader := &Loader{load: func(ctx context.Context, base, root string) (*Tree, error) {
		started <- base
		select {
		case <-release:
		case <-time.After(10 * time.Second):
			return nil, errors.New("the other load never started: loads are serialized")
		}
		return LoadTreeContext(ctx, gitutil.Git{}, base, root)
	}}

	// Act
	errs := make(chan error, 2)
	for _, dir := range []string{a, b} {
		go func() {
			_, err := loader.Load(dir)
			errs <- err
		}()
	}
	<-started
	<-started
	close(release)

	// Assert
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
}

func TestLoaderRetriesAfterAFailedLoad(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	var calls atomic.Int32
	loader := &Loader{load: func(ctx context.Context, base, root string) (*Tree, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("boom")
		}
		return LoadTreeContext(ctx, gitutil.Git{}, base, root)
	}}

	// Act
	_, first := loader.Load(dir)
	tree, second := loader.Load(dir)

	// Assert
	require.Error(t, first)
	require.NoError(t, second)
	assert.NotNil(t, tree)
}
