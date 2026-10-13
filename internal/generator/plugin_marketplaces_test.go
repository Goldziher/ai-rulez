package generator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoMarketplacesTail = twoPluginsTail + `
[[marketplaces_authoring]]
name = "acme-public"
description = "Published plugins"
plugins = ["macos-notifications", "moon-ear"]
output_dir = "marketplaces/public"

[[marketplaces_authoring]]
name = "acme-internal"
plugins = ["moon-ear"]
output_dir = "marketplaces/internal"
`

func TestCollectPluginOutputs_MarketplacesAuthoring(t *testing.T) {
	tests := []struct {
		name       string
		names      []string
		wantIndex  map[string][]string
		wantFiles  []string
		wantAbsent []string
	}{
		{
			name:  "each marketplace gets its own index, sidecar and bundles",
			names: nil,
			wantIndex: map[string][]string{
				"marketplaces/public/.claude-plugin/marketplace.json":   {"macos-notifications", "moon-ear"},
				"marketplaces/internal/.claude-plugin/marketplace.json": {"moon-ear"},
			},
			wantFiles: []string{
				"marketplaces/public/.ai-rulez-generated.json",
				"marketplaces/internal/.ai-rulez-generated.json",
				"marketplaces/public/plugins/macos-notifications/skills/notify-a/SKILL.md",
				"marketplaces/public/plugins/moon-ear/skills/moon-a/SKILL.md",
				"marketplaces/internal/plugins/moon-ear/skills/moon-a/SKILL.md",
			},
			wantAbsent: []string{
				"marketplaces/internal/plugins/macos-notifications/skills/notify-a/SKILL.md",
				".claude-plugin/marketplace.json",
			},
		},
		{
			name:       "a name limits the run to one marketplace",
			names:      []string{"acme-internal"},
			wantIndex:  map[string][]string{"marketplaces/internal/.claude-plugin/marketplace.json": {"moon-ear"}},
			wantFiles:  []string{"marketplaces/internal/.ai-rulez-generated.json"},
			wantAbsent: []string{"marketplaces/public/.claude-plugin/marketplace.json", "marketplaces/public/plugins/moon-ear/skills/moon-a/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDefsProject(t, twoMarketplacesTail, nil)
			gen := loadDomainsProject(t, dir).WithMarketplaceNames(tt.names)

			// Act
			outputs, err := gen.collectPluginOutputs("")

			// Assert
			require.NoError(t, err)
			byRel := outputsByRel(t, dir, outputs)
			for rel, wantNames := range tt.wantIndex {
				names, _ := marketplaceIndex(t, byRel, rel)
				assert.Equal(t, wantNames, names, rel)
			}
			for _, want := range tt.wantFiles {
				assert.Contains(t, byRel, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, byRel, absent)
			}
		})
	}
}

func TestCollectPluginOutputs_MarketplacesAuthoring_Errors(t *testing.T) {
	tests := []struct {
		name    string
		tail    string
		names   []string
		wantErr string
	}{
		{
			name: "a marketplace lists a plugin no definition provides",
			tail: twoPluginsTail + `
[[marketplaces_authoring]]
name = "pub"
plugins = ["ghost"]
`,
			wantErr: `marketplace "pub" lists plugin "ghost"`,
		},
		{
			name:    "an unknown --marketplace-name lists the marketplaces",
			tail:    twoMarketplacesTail,
			names:   []string{"ghost"},
			wantErr: "acme-internal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDefsProject(t, tt.tail, nil)
			cfgGen := loadDomainsProjectUnvalidated(t, dir).WithMarketplaceNames(tt.names)

			// Act
			_, err := cfgGen.collectPluginOutputs("")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCollectPluginOutputs_MarketplacesFromDefinitionFiles(t *testing.T) {
	// Arrange
	dir := newDefsProject(t, twoPluginsTail, map[string]string{
		"marketplaces/public.toml": "plugins = [\"macos-notifications\", \"moon-ear\"]\noutput_dir = \"out/public\"\n",
	})
	gen := loadDomainsProject(t, dir)

	// Act
	outputs, err := gen.collectPluginOutputs("")

	// Assert
	require.NoError(t, err)
	byRel := outputsByRel(t, dir, outputs)
	names, _ := marketplaceIndex(t, byRel, "out/public/.claude-plugin/marketplace.json")
	assert.Equal(t, []string{"macos-notifications", "moon-ear"}, names)
}

func TestVerifyPlugin_MarketplacesAuthoring_NamesTheMarketplacePerResult(t *testing.T) {
	// Arrange
	dir := newDefsProject(t, twoMarketplacesTail, nil)
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	require.NoError(t, gen.VerifyPlugin(""))
	plugins, marketplaces := gen.PluginStatuses(nil)
	assert.Len(t, plugins, 3, "moon-ear is listed by both marketplaces")
	assert.Len(t, marketplaces, 2)

	// A hand edit in the internal marketplace's copy of moon-ear.
	skill := filepath.Join(dir, "marketplaces", "internal", "plugins", "moon-ear", "skills", "moon-a", "SKILL.md")
	require.NoError(t, os.WriteFile(skill, []byte("edited"), 0o600))

	// Act
	err := gen.VerifyPlugin("")

	// Assert
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPluginDrift))
	drift := PluginDriftOf(err)
	require.NotEmpty(t, drift)
	for _, item := range drift {
		assert.Equal(t, "acme-internal", item.Marketplace, item.Path)
		assert.Equal(t, "moon-ear", item.Plugin, item.Path)
	}
	_, marketplaces = gen.PluginStatuses(drift)
	status := map[string]string{}
	for _, m := range marketplaces {
		status[m.Name] = m.Status
	}
	assert.Equal(t, map[string]string{"acme-public": "ok", "acme-internal": "drift"}, status)
}

func TestGeneratePlugin_MarketplacesAuthoring_SameOutputDirIsAnError(t *testing.T) {
	// Arrange: two marketplaces writing one directory would overwrite each other's index.
	tail := twoPluginsTail + `
[[marketplaces_authoring]]
name = "one"
plugins = ["moon-ear"]
output_dir = "out"

[[marketplaces_authoring]]
name = "two"
plugins = ["moon-ear"]
output_dir = "out"
`
	dir := newDefsProject(t, tail, nil)
	gen := loadDomainsProjectUnvalidated(t, dir)

	// Act
	_, err := gen.collectPluginOutputs("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two plugin outputs are written to")
}
