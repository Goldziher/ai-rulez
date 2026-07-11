package generator

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectPluginOutputs_Monorepo verifies that a marketplace-only root config
// renders each member's bundle plus the aggregate marketplace index.
func TestCollectPluginOutputs_Monorepo(t *testing.T) {
	cfg, err := config.LoadConfig(context.Background(), "../../tests/fixtures/plugin/monorepo")
	require.NoError(t, err)

	gen := NewGenerator(cfg)
	outputs, err := gen.collectPluginOutputs("")
	require.NoError(t, err)

	byRel := make(map[string][]byte, len(outputs))
	for _, o := range outputs {
		rel, err := filepath.Rel(cfg.BaseDir, o.Path)
		require.NoError(t, err)
		body := o.RawContent
		if body == nil {
			body = []byte(o.Content)
		}
		byRel[filepath.ToSlash(rel)] = body
	}

	// Members render their own manifests; no per-member marketplace exists.
	require.Contains(t, byRel, "plugins/alpha/.claude-plugin/plugin.json")
	require.Contains(t, byRel, "plugins/alpha/.factory-plugin/plugin.json")
	require.Contains(t, byRel, "plugins/beta/.claude-plugin/plugin.json")
	assert.NotContains(t, byRel, "plugins/alpha/.claude-plugin/marketplace.json")

	// Root aggregate marketplace lists both members with monorepo sources.
	var mkt map[string]any
	require.NoError(t, json.Unmarshal(byRel[".claude-plugin/marketplace.json"], &mkt))
	assert.Equal(t, "acme", mkt["name"])
	plugins := mkt["plugins"].([]any)
	require.Len(t, plugins, 2)

	sources := map[string]string{}
	for _, p := range plugins {
		e := p.(map[string]any)
		sources[e["name"].(string)] = e["source"].(string)
	}
	assert.Equal(t, "./plugins/alpha", sources["alpha"])
	assert.Equal(t, "./plugins/beta", sources["beta"])

	var codexMarket map[string]any
	require.NoError(t, json.Unmarshal(byRel[".agents/plugins/marketplace.json"], &codexMarket))
	assert.Equal(t, "acme", codexMarket["name"])
	assert.Equal(t, "acme", codexMarket["interface"].(map[string]any)["displayName"])
	codexPlugins := codexMarket["plugins"].([]any)
	require.Len(t, codexPlugins, 2)
	alphaEntry := codexPlugins[0].(map[string]any)
	assert.Equal(t, "local", alphaEntry["source"].(map[string]any)["source"])
	assert.Equal(t, "./plugins/alpha", alphaEntry["source"].(map[string]any)["path"])
	assert.Equal(t, "AVAILABLE", alphaEntry["policy"].(map[string]any)["installation"])
	assert.Equal(t, "ON_INSTALL", alphaEntry["policy"].(map[string]any)["authentication"])
	assert.Equal(t, "Developer Tools", alphaEntry["category"])

	// Member manifest carries its own version.
	var alpha map[string]any
	require.NoError(t, json.Unmarshal(byRel["plugins/alpha/.claude-plugin/plugin.json"], &alpha))
	assert.Equal(t, "1.0.0", alpha["version"])
}
