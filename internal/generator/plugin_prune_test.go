package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePluginPrunesRemovedSkills(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "rename"}[rename], func(t *testing.T) {
			dir := newDomainsProject(t, "")
			gen := loadDomainsProject(t, dir)
			require.NoError(t, gen.GeneratePlugin(""))
			old := filepath.Join(dir, "skills/core-s/SKILL.md")
			handwritten := filepath.Join(dir, "skills/manual/SKILL.md")
			writeDomainsFile(t, handwritten, "hand-written\n")
			source := filepath.Join(dir, ".ai-rulez/skills/core-s")
			if rename {
				require.NoError(t, os.Rename(source, filepath.Join(dir, ".ai-rulez/skills/renamed")))
			} else {
				require.NoError(t, os.RemoveAll(source))
			}
			gen = loadDomainsProject(t, dir)
			lines, err := gen.DryRunPlugin("")
			require.NoError(t, err)
			assert.Contains(t, strings.Join(lines, "\n"), "delete-stale: skills/core-s/SKILL.md")
			require.FileExists(t, old, "dry run must not delete")
			require.NoError(t, gen.GeneratePlugin(""))
			assert.NoFileExists(t, old)
			assert.NoDirExists(t, filepath.Dir(old))
			data, err := os.ReadFile(handwritten)
			require.NoError(t, err)
			assert.Equal(t, "hand-written\n", string(data))
			if rename {
				assert.FileExists(t, filepath.Join(dir, "skills/renamed/SKILL.md"))
			}
			require.NoError(t, gen.VerifyPlugin(""))
			require.NoError(t, gen.GeneratePlugin(""))
		})
	}
}

func TestGeneratePluginPreservesEditedObsoleteOutput(t *testing.T) {
	dir := newDomainsProject(t, "")
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	sidecar := filepath.Join(dir, ".ai-rulez-generated.json")
	before, err := os.ReadFile(sidecar)
	require.NoError(t, err)
	old := filepath.Join(dir, "skills/core-s/SKILL.md")
	writeDomainsFile(t, old, "edited by user\n")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez/skills/core-s")))
	gen = loadDomainsProject(t, dir)
	err = gen.GeneratePlugin("")
	require.ErrorContains(t, err, "modified obsolete plugin output")
	data, err := os.ReadFile(old)
	require.NoError(t, err)
	assert.Equal(t, "edited by user\n", string(data))
	after, err := os.ReadFile(sidecar)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestGeneratePluginPrunesDomainBundleOutputs(t *testing.T) {
	dir := newDomainsProject(t, "\n[marketplace]\nname = \"demo\"\noutput_dir = \"mkt\"\n\n[marketplace.from_domains]\nenabled = true\nruntimes = [\"claude\"]\n")
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	root := filepath.Join(dir, "mkt/plugins/teama")
	old := filepath.Join(root, "skills/a-s/SKILL.md")
	require.FileExists(t, old)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez/domains/teamA/skills/a-s")))
	gen = loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	assert.NoFileExists(t, old)
	assert.FileExists(t, filepath.Join(root, "commands/do-it.md"))
	assert.FileExists(t, filepath.Join(dir, "mkt/plugins/teamb/skills/b-s/SKILL.md"))
	require.NoError(t, gen.VerifyPlugin(""))
}
