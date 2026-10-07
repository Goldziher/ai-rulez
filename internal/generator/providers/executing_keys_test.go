package providers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The organization policy bounds the frontmatter keys in config's registry of
// executing keys. A provider spec that carries a hooks or MCP key through must
// name one the registry knows, or imported content could smuggle it past the
// policy.
func TestPassedThroughHooksAndMCPKeysAreInTheExecutingRegistry(t *testing.T) {
	names, err := BuiltinNames()
	require.NoError(t, err)
	for _, name := range names {
		gen, err := LoadBuiltin(name)
		require.NoError(t, err)
		for typ, out := range gen.Spec.Outputs {
			if out == nil || out.Frontmatter == nil {
				continue
			}
			for _, field := range out.Frontmatter.Fields {
				norm := config.NormalizeFrontmatterKey(field)
				if strings.Contains(norm, "mcp") || strings.Contains(norm, "hook") {
					assert.True(t, config.IsExecutingFrontmatterKey(field),
						"%s %s passes %q through but the executing-key registry does not know it", name, typ, field)
				}
			}
		}
	}
}

func TestExecutingFrontmatterKeysAreCaseAndSeparatorInsensitive(t *testing.T) {
	for _, key := range []string{"hooks", "Hooks", "mcpServers", "mcp-servers", "mcp_servers", "MCP-Servers"} {
		assert.True(t, config.IsExecutingFrontmatterKey(key), key)
	}
	for _, key := range []string{"tools", "description", "mcp", "model"} {
		assert.False(t, config.IsExecutingFrontmatterKey(key), key)
	}
}
