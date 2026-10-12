package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// writeVersionFile writes a version source manifest under dir.
func writeVersionFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestBuildManifest_ResolvesVersionFrom(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeVersionFile(t, dir, "package.json", `{"name":"demo","version":"4.5.6"}`)
	cfg := &config.Config{BaseDir: dir, Plugin: &config.PluginAuthoring{
		Name:        "demo",
		Description: "d",
		VersionFrom: "package.json",
		Runtimes:    []string{config.PluginRuntimeClaude},
	}}

	// Act
	m, err := BuildManifest(cfg, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "4.5.6", m.Version)

	outputs, err := Generate(m, dir)
	require.NoError(t, err)
	manifest := outputByPath(t, outputs, ".claude-plugin/plugin.json")
	assert.Equal(t, "4.5.6", parseJSON(t, manifest.RawContent)["version"])

	// A version bump in the source manifest changes the bundle, which is the
	// change verify --plugin and PluginVersionDrift compare against.
	writeVersionFile(t, dir, "package.json", `{"name":"demo","version":"4.5.7"}`)
	bumped, err := BuildManifest(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, "4.5.7", bumped.Version)

	bumpedOutputs, err := Generate(bumped, dir)
	require.NoError(t, err)
	assert.NotEqual(t,
		manifest.RawContent,
		outputByPath(t, bumpedOutputs, ".claude-plugin/plugin.json").RawContent,
		"a source-manifest version bump must change the generated manifest")
}

func TestBuildManifest_ProvenanceHeaders(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	off := false
	cfg := &config.Config{BaseDir: dir, Plugin: &config.PluginAuthoring{
		Name:              "demo",
		Description:       "d",
		Version:           "1.0.0",
		Runtimes:          []string{config.PluginRuntimeClaude},
		ProvenanceHeaders: &off,
	}}

	// Act
	disabled, err := BuildManifest(cfg, nil)
	require.NoError(t, err)
	cfg.Plugin.ProvenanceHeaders = nil
	enabled, err := BuildManifest(cfg, nil)
	require.NoError(t, err)

	// Assert
	assert.False(t, disabled.writesProvenanceHeaders())
	assert.True(t, enabled.writesProvenanceHeaders())
}

// Domain plugins inherit the [plugin] block's resolved version and its
// provenance_headers switch.
func TestPlanDomainPlugins_ResolvesVersionFrom(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeVersionFile(t, dir, "Cargo.toml", "[package]\nname = \"demo\"\nversion = \"7.8.9\"\n")
	off := false
	cfg := &config.Config{
		BaseDir: dir,
		Plugin: &config.PluginAuthoring{
			Name:              "root",
			VersionFrom:       "Cargo.toml",
			Runtimes:          []string{config.PluginRuntimeClaude},
			ProvenanceHeaders: &off,
		},
		Marketplace: &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{}},
	}

	// Act
	plan, err := PlanDomainPlugins(cfg, domainTree())

	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, plan)
	for _, p := range plan {
		assert.Equal(t, "7.8.9", p.Version)
	}
	m := BuildDomainManifest(cfg, &plan[0])
	assert.Equal(t, "7.8.9", m.Version)
	assert.False(t, m.writesProvenanceHeaders())
}
