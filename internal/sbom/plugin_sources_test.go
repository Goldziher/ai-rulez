package sbom_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

func TestBuild_ListsInstalledMarketplacesAndPlugins(t *testing.T) {
	// Arrange
	f := newFixture(t, `version = "5.0"
name = "p"
presets = ["claude"]

[[installed_marketplaces]]
name = "acme"
source = "https://github.com/acme/plugins"
ref = "main"

[[installed_plugins]]
name = "tools"
marketplace = "acme"

[[installed_plugins]]
name = "direct"
source = "https://github.com/acme/direct"
profiles = ["other"]
`, nil)
	f.write("ai-rulez.lock", `version = 1

[[marketplace]]
name = "acme"
source = "https://github.com/acme/plugins"
ref = "main"
commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
digest = "sha256:1111"

[[plugin]]
name = "tools"
source = "marketplace:acme"
commit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
digest = "sha256:2222"
origin = "https://github.com/acme/tools#plugins/tools@main"
`)

	// Act
	cfg, err := config.LoadConfig(config.WithOfflineIncludes(context.Background()), f.dir, config.WithoutLocal(), config.WithResolvers(pluginresolve.Resolvers("", nil)))
	require.NoError(t, err)
	bom, err := sbom.Build(cfg, "9.9.9", sbom.Options{})
	require.NoError(t, err)

	// Assert
	found := map[string]string{}
	for i := range bom.Components {
		c := &bom.Components[i]
		for _, p := range c.Properties {
			if p.Name == "ai-rulez:source-kind" {
				found[c.Name] = p.Value
			}
		}
	}
	require.Contains(t, found, "acme")
	assert.Equal(t, "marketplace", found["acme"])
	assert.Equal(t, "plugin", found["tools"])
	assert.Equal(t, "plugin", found["direct"])
}
