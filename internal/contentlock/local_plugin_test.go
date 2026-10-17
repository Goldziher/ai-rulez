package contentlock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCompute_PinsLocalPluginAndMarketplaceTrees(t *testing.T) {
	tests := []struct {
		name      string
		configure func(f *fixture)
		kind      string
		id        string
		edit      string
	}{
		{
			name: "plugin hooks file",
			configure: func(f *fixture) {
				f.cfg.InstalledPlugins = []config.InstalledPluginConfig{{Name: "tools", Source: "./vendor/tools", EnableHooks: true}}
			},
			kind: KindLocalPlugin, id: "tools", edit: "vendor/tools/hooks/hooks.json",
		},
		{
			name: "plugin script",
			configure: func(f *fixture) {
				f.cfg.InstalledPlugins = []config.InstalledPluginConfig{{Name: "tools", Source: "./vendor/tools"}}
			},
			kind: KindLocalPlugin, id: "tools", edit: "vendor/tools/scripts/run.sh",
		},
		{
			name: "plugin in a subdirectory",
			configure: func(f *fixture) {
				f.cfg.InstalledPlugins = []config.InstalledPluginConfig{{Name: "tools", Source: "./vendor", Path: "tools"}}
			},
			kind: KindLocalPlugin, id: "tools", edit: "vendor/tools/hooks/hooks.json",
		},
		{
			name: "marketplace tree",
			configure: func(f *fixture) {
				f.cfg.InstalledMarketplaces = []config.InstalledMarketplaceConfig{{Name: "mk", Source: "./vendor"}}
				f.cfg.InstalledPlugins = []config.InstalledPluginConfig{{Name: "tools", Marketplace: "mk"}}
			},
			kind: KindLocalMarketplace, id: "mk", edit: "vendor/tools/hooks/hooks.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t)
			writeAt(t, f.root, "vendor/tools/hooks/hooks.json", `{"hooks":{}}`)
			writeAt(t, f.root, "vendor/tools/scripts/run.sh", "echo one\n")
			tt.configure(f)

			// Act
			before := localIncludeSnapshot(t, f)
			writeAt(t, f.root, tt.edit, "changed\n")
			after := localIncludeSnapshot(t, f)

			// Assert
			require.Empty(t, before.Problems)
			require.Len(t, before.Items, 1)
			assert.Equal(t, tt.kind, before.Items[0].Kind)
			assert.Equal(t, tt.id, before.Items[0].ID)
			require.Len(t, after.Items, 1)
			assert.NotEqual(t, before.Items[0].Digest, after.Items[0].Digest)
		})
	}
}

func TestCompute_LocalPluginSkipsRemoteAndReportsMissing(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.cfg.InstalledPlugins = []config.InstalledPluginConfig{
		{Name: "remote", Source: "owner/repo"},
		{Name: "gone", Source: "./vendor/gone"},
	}
	f.cfg.InstalledMarketplaces = []config.InstalledMarketplaceConfig{{Name: "m", Source: "https://github.com/o/r.git"}}

	// Act
	snap := localIncludeSnapshot(t, f)

	// Assert
	assert.Empty(t, snap.Items)
	require.Len(t, snap.Problems, 1)
	assert.Contains(t, snap.Problems[0], "gone")
}
