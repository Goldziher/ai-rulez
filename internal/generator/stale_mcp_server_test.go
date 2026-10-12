package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPlainMCPRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	initGitRepo(t, root)
	dir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`version = "5.0"
name = "stale"
presets = ["claude"]

[[mcp_servers]]
name = "foo"
command = "echo"
args = ["x"]
`), 0o644))
	return root
}

func removeServerBlock(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`version = "5.0"
name = "stale"
presets = ["claude"]
`), 0o644))
}

func TestGenerate_RemovedMCPServerLeavesWhenThisMachineRecordedIt(t *testing.T) {
	root := newPlainMCPRepo(t)
	generateRepo(t, root)

	removeServerBlock(t, root)
	generateRepo(t, root)

	got, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), `"foo"`)
}

func TestGenerate_RemovedMCPServerStaysWithoutALocalRecordButIsRemembered(t *testing.T) {
	root := newPlainMCPRepo(t)
	generateRepo(t, root)

	committed, err := os.ReadFile(filepath.Join(root, ".ai-rulez", generatedManifestName))
	require.NoError(t, err)
	assert.Contains(t, string(committed), `"servers"`)
	assert.Contains(t, string(committed), `"foo"`)

	// A fresh clone has the committed manifest but not the machine-local one.
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json")))
	removeServerBlock(t, root)
	generateRepo(t, root)

	got, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(got), `"foo"`, "an entry without a local record is never removed on the strength of a committed file")

	after, err := os.ReadFile(filepath.Join(root, ".ai-rulez", generatedManifestName))
	require.NoError(t, err)
	assert.NotContains(t, string(after), `"foo"`, "the hint follows what the config renders now")
}
