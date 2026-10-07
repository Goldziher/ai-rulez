package lint

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRegistryBuildsConcurrently asks for the registry from many goroutines at
// once (the first lint of a long-running host): ruleTables builds it once and
// hands every caller the same one. buildRuleSet itself is not for direct
// concurrent use, since the families fill the analyzer table. Run it with -race.
func TestRegistryBuildsConcurrently(t *testing.T) {
	t.Parallel()
	const workers = 8
	tables := make([]*ruleSet, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tables[i] = ruleTables()
			_ = AnalyzerFor("AR005")
		}()
	}
	wg.Wait()
	for i := 1; i < workers; i++ {
		assert.Same(t, tables[0], tables[i], "caller %d", i)
	}
}
