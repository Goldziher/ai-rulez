package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestPolyCatalogChecksSourceAndGeneratedDrift(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "poly-hooks.toml"))
	require.NoError(t, err)
	var catalog struct {
		Hooks []struct {
			ID        string   `toml:"id"`
			Args      []string `toml:"args"`
			Files     []string `toml:"files"`
			AlwaysRun bool     `toml:"always_run"`
		} `toml:"hooks"`
	}
	require.NoError(t, toml.Unmarshal(data, &catalog))
	found := map[string]bool{}
	for _, hook := range catalog.Hooks {
		switch hook.ID {
		case "ai-rulez-validate":
			found[hook.ID] = true
			require.Equal(t, []string{"validate", "--no-local"}, hook.Args)
		case "ai-rulez-check":
			require.True(t, hook.AlwaysRun)
			found[hook.ID] = true
			require.Equal(t, []string{"generate", "--check", "--no-local"}, hook.Args)
			require.Equal(t, []string{"**"}, hook.Files)
		case "ai-rulez-plugin-verify":
			require.True(t, hook.AlwaysRun)
			found[hook.ID] = true
			require.Contains(t, hook.Files, "plugin/**")
			require.Contains(t, hook.Files, ".claude-plugin/**")
			require.Contains(t, hook.Files, ".agents/plugins/**")
		}
	}
	require.Len(t, found, 3)
}
