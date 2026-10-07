package presets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestPassedThroughHooksAndMCPKeysAreInTheExecutingRegistry(t *testing.T) {
	for _, field := range copilotAgentFields {
		norm := config.NormalizeFrontmatterKey(field)
		if strings.Contains(norm, "mcp") || strings.Contains(norm, "hook") {
			assert.True(t, config.IsExecutingFrontmatterKey(field), "copilot passes %q through", field)
		}
	}
}
