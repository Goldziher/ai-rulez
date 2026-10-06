package generator

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A Generator owns the rules-folder set of its config from the start, so a
// shallow copy of the config (the generator makes several) shares it instead of
// lazily growing a private one, and parallel adds do not race on its creation.
func TestNewGeneratorInitializesTheRulesDirSet(t *testing.T) {
	// Arrange
	cfg := &config.Config{}

	// Act
	g := NewGenerator(cfg)
	cp := *g.config
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { cp.AddRulesDir("custom/rules") })
	}
	wg.Wait()

	// Assert
	require.NotNil(t, cfg.RulesDirs)
	assert.True(t, cfg.InRulesDir("custom/rules/x.md"), "a folder added through a copy is seen by the original")
}
