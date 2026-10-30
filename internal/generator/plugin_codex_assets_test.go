package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codexAssetProject writes a codex plugin whose interface icon lives in the
// content root (.ai-rulez/assets) while the output root (the project dir)
// receives the bundled copy.
func codexAssetProject(t *testing.T) string {
	t.Helper()
	dir := newDomainsProject(t, "")
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	updated := strings.Replace(string(body), `runtimes = ["claude"]`, `runtimes = ["codex"]`, 1)
	updated += `
[plugin.interface]
display_name = "Demo"
short_description = "short"
long_description = "long"
developer_name = "Demo Team"
category = "Coding"
capabilities = ["Read"]
default_prompt = ["Do it"]
composer_icon = "./assets/icon.svg"
`
	writeDomainsFile(t, cfgPath, updated)
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "assets", "icon.svg"), "v1")
	return dir
}

func TestGeneratePlugin_CodexInterfaceAssetsFollowTheContentRoot(t *testing.T) {
	dir := codexAssetProject(t)
	out := filepath.Join(dir, "assets", "icon.svg")
	source := filepath.Join(dir, ".ai-rulez", "assets", "icon.svg")

	// Source edit reaches the bundle.
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(got))
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))

	writeDomainsFile(t, source, "v2")
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	got, err = os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got), "a source edit must replace the bundled copy")

	// verify --plugin detects a bundled asset that differs from its source.
	writeDomainsFile(t, out, "stale")
	require.ErrorIs(t, loadDomainsProject(t, dir).VerifyPlugin(""), ErrPluginDrift)

	// A missing output assets dir does not break generation.
	require.NoError(t, os.RemoveAll(filepath.Dir(out)))
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	assert.FileExists(t, out)
}
