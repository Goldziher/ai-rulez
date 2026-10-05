package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Plugin bundles are distributable, so a member's machine-local overlay must
// never leak into them.
func TestCollectPluginOutputs_MonorepoIgnoresMemberLocalOverlay(t *testing.T) {
	// Arrange
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS("../../tests/fixtures/plugin/monorepo")))
	overlay := filepath.Join(root, "plugins", "alpha", ".ai-rulez", "config.local.toml")
	require.NoError(t, os.WriteFile(overlay, []byte("[plugin]\nversion = \"9.9.9\"\n"), 0o600))

	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal())
	require.NoError(t, err)

	// Act
	outputs, err := NewGenerator(cfg).collectPluginOutputs("")
	require.NoError(t, err)

	// Assert
	var manifest map[string]any
	for _, o := range outputs {
		if filepath.ToSlash(o.Path) == filepath.ToSlash(filepath.Join(root, "plugins/alpha/.claude-plugin/plugin.json")) {
			body := o.RawContent
			if body == nil {
				body = []byte(o.Content)
			}
			require.NoError(t, json.Unmarshal(body, &manifest))
		}
	}
	require.NotNil(t, manifest)
	assert.Equal(t, "1.0.0", manifest["version"])
}
