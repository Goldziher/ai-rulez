package lint

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
