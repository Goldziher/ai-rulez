package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// includeProject writes a single-plugin agent-plugins project whose
// [plugin] include ships a launcher an MCP command points at, and generates it.
func includeProject(t *testing.T) string {
	t.Helper()
	dir := newDomainsProject(t, "")
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	updated := strings.Replace(string(body),
		`runtimes = ["claude"]`,
		"runtimes = [\"agent-plugins\"]\ninclude = [\"scripts/**\"]", 1)
	updated += `
[[plugin.mcp]]
name = "s"
command = "./scripts/mcp-launch.sh"
`
	writeDomainsFile(t, cfgPath, updated)
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "scripts", "mcp-launch.sh"), "#!/bin/sh\n")

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	return dir
}

// TestGeneratePlugin_VerifyPluginCoversIncludedFiles is the verify --plugin half
// of the fix: a changed or missing included file is drift.
func TestGeneratePlugin_VerifyPluginCoversIncludedFiles(t *testing.T) {
	dir := includeProject(t)
	launcher := filepath.Join(dir, "scripts", "mcp-launch.sh")
	require.FileExists(t, launcher, "the included launcher lands in the bundle")
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))

	writeDomainsFile(t, launcher, "changed\n")
	err := loadDomainsProject(t, dir).VerifyPlugin("")
	require.ErrorIs(t, err, ErrPluginDrift, "a changed included file is drift")

	require.NoError(t, os.Remove(launcher))
	err = loadDomainsProject(t, dir).VerifyPlugin("")
	require.ErrorIs(t, err, ErrPluginDrift, "a missing included file is drift")
	assert.NoFileExists(t, launcher)
}
