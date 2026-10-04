package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const staleTail = `
[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"
`

func TestGeneratePlugin_RemovesStalePluginDirectory(t *testing.T) {
	dir := newDomainsProject(t, staleTail)
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	teamA := filepath.Join(dir, "mkt", "plugins", "demo-teama")
	teamB := filepath.Join(dir, "mkt", "plugins", "demo-teamb")
	require.DirExists(t, teamA)
	require.DirExists(t, teamB)

	// A hand-made directory and an unrelated file must survive.
	handMade := filepath.Join(dir, "mkt", "plugins", "mine")
	writeDomainsFile(t, filepath.Join(handMade, "keep.txt"), "mine")
	unrelated := filepath.Join(dir, "mkt", "README.md")
	writeDomainsFile(t, unrelated, "readme")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "domains", "teamB")))
	gen := loadDomainsProject(t, dir)

	lines, err := gen.DryRunPlugin("")
	require.NoError(t, err)
	assert.Contains(t, lines, "delete-stale: mkt/plugins/demo-teamb")
	assert.DirExists(t, teamB, "dry run must not delete")

	require.Error(t, gen.VerifyPlugin(""), "verify must flag the stale directory")

	require.NoError(t, gen.GeneratePlugin(""))
	assert.NoDirExists(t, teamB)
	assert.DirExists(t, teamA)
	assert.FileExists(t, filepath.Join(handMade, "keep.txt"))
	assert.FileExists(t, unrelated)
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
}

func TestGeneratePlugin_StaleDirKeepsFilesItDidNotGenerate(t *testing.T) {
	dir := newDomainsProject(t, staleTail)
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	teamB := filepath.Join(dir, "mkt", "plugins", "demo-teamb")
	extra := filepath.Join(teamB, "notes", "mine.md")
	writeDomainsFile(t, extra, "hand written")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "domains", "teamB")))

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	assert.FileExists(t, extra, "a file ai-rulez did not generate is never deleted")
	assert.NoFileExists(t, filepath.Join(teamB, "agents", "ag.md"))
	assert.NoFileExists(t, filepath.Join(teamB, ".ai-rulez-generated.json"))
}
