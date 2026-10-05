package presets

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestLegacyMergeClaims_DevinConfigCoversUserScopeHooks(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		UserScope: true,
		Hooks:     []config.HookGroup{{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "echo guard"}}}},
	}

	// Act
	claims := LegacyMergeClaims(MergedDocDevinConfig, cfg)

	// Assert
	var paths [][]string
	for _, claim := range claims {
		paths = append(paths, claim.Path)
	}
	assert.Contains(t, paths, []string{"hooks", "PreToolUse"})
}
