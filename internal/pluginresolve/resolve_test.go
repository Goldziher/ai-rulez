package pluginresolve_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestResolve_MarketplaceRelativePlugin(t *testing.T) {
	// Arrange
	isolate(t)
	repo := plugintest.NewRepo(t, plugintest.Merge(
		plugintest.ClaudeMarketplace("acme", `{"name": "tools", "source": "./plugins/tools"}`),
		plugintest.Prefix("plugins/tools", plugintest.ClaudePlugin("tools")),
	))
	project := plugintest.Project(t, `
[[installed_marketplaces]]
name = "acme"
source = "`+repo.URL()+`"

[[installed_plugins]]
name = "acme-tools"
marketplace = "acme"
plugin = "tools"
enable_hooks = true
`)

	// Act
	cfg := mustLoad(t, project)

	// Assert
	assert.Equal(t, []string{"review-tools"}, skillNames(cfg))
	assert.Equal(t, []string{"helper"}, fileNames(cfg.Content.Agents))
	assert.Equal(t, []string{"deploy"}, fileNames(cfg.Content.Commands))
	require.Len(t, cfg.Hooks, 1)
	assert.Equal(t, "installed-plugin", cfg.Hooks[0].Builtin)
	require.Contains(t, cfg.MCPServers, "tools-docs")
	_ = testutil.GitIdentityName
}
