package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claudeMDNoneTable turns the claude preset into AGENTS.md only. It is a TOML
// table, so a fixture appends it after its top-level keys.
const claudeMDNoneTable = "\n[claude]\nclaude_md = \"none\"\n"

// claudeMDScopeBlock adds an api scope that renders the claude preset.
const claudeMDScopeBlock = `
[profiles]
api = ["api"]

[[scopes]]
path = "packages/api"
profile = "api"
presets = ["claude"]
`

func newClaudeMDScopeProject(t *testing.T, flag, extra string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, flag+agentsMDConfig([]string{"claude"}, "", claudeMDScopeBlock+extra))
	writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "# Api Style\n\nAPI_STYLE_BODY\n")
	return root
}

func TestClaudeMD_DefaultWritesShim(t *testing.T) {
	root := newAgentsMDPresetProject(t, "agents_md = true\n", []string{"claude"}, "")
	runAgentsMDGenerate(t, root)

	assert.Equal(t, "@AGENTS.md", afterBanner(readAgentsMDFile(t, root, "CLAUDE.md")))
	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "ALWAYS_BODY")
}

func TestClaudeMD_NoneWritesOnlyAgentsMD(t *testing.T) {
	root := newAgentsMDPresetProject(t, "agents_md = true\n", []string{"claude"}, claudeMDNoneTable)
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
	agents := readAgentsMDFile(t, root, "AGENTS.md")
	assert.Contains(t, agents, "ALWAYS_BODY")
	assert.Contains(t, agents, "OVERVIEW_BODY")
	assert.FileExists(t, filepath.Join(root, ".claude", "rules", "go-style.md"))
}

func TestClaudeMD_NoneOmitsScopeShim(t *testing.T) {
	root := newClaudeMDScopeProject(t, "agents_md = true\n", claudeMDNoneTable)
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
	assert.NoFileExists(t, filepath.Join(root, "packages", "api", "CLAUDE.md"))
	assert.Contains(t, readAgentsMDFile(t, root, "packages/api/AGENTS.md"), "API_STYLE_BODY")
}

// The switch removes the shims a previous run generated, root and scopes alike,
// even with no manifest to name them: the shim's own Content-Hash is the proof.
func TestClaudeMD_NoneRemovesGeneratedShimsWithoutManifest(t *testing.T) {
	root := newClaudeMDScopeProject(t, "agents_md = true\n", "")
	runAgentsMDGenerate(t, root)
	require.FileExists(t, filepath.Join(root, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(root, "packages", "api", "CLAUDE.md"))

	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", generatedManifestName)))

	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"claude"}, "", claudeMDScopeBlock+claudeMDNoneTable))
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
	assert.NoFileExists(t, filepath.Join(root, "packages", "api", "CLAUDE.md"))
	assert.Contains(t, readAgentsMDFile(t, root, "packages/api/AGENTS.md"), "API_STYLE_BODY")
}

// A hand-authored CLAUDE.md carries no Content-Hash, so the switch leaves it
// alone; it is the user's file, and it shadows AGENTS.md (see the docs).
func TestClaudeMD_NoneKeepsHandAuthoredClaudeMD(t *testing.T) {
	root := newAgentsMDPresetProject(t, "agents_md = true\n", []string{"claude"}, claudeMDNoneTable)
	writeAgentsMDFile(t, root, "CLAUDE.md", "# Mine\n\nHAND_BODY\n")

	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "CLAUDE.md"), "HAND_BODY")
	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "ALWAYS_BODY")
}

func TestClaudeMD_CheckReportsStaleShimsAfterSwitch(t *testing.T) {
	root := newClaudeMDScopeProject(t, "agents_md = true\n", "")
	runAgentsMDGenerate(t, root)
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", generatedManifestName)))

	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"claude"}, "", claudeMDScopeBlock+claudeMDNoneTable))
	drift, err := NewGenerator(loadAgentsMDConfig(t, root)).CheckDrift("")
	require.NoError(t, err)

	assert.Contains(t, drift, Drift{Path: "CLAUDE.md", Kind: DriftOrphan})
	assert.Contains(t, drift, Drift{Path: "packages/api/CLAUDE.md", Kind: DriftOrphan})
}

func TestClaudeMD_CleanRemovesShimWithoutManifest(t *testing.T) {
	root := newClaudeMDScopeProject(t, "agents_md = true\n", "")
	runAgentsMDGenerate(t, root)
	require.FileExists(t, filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", generatedManifestName)))

	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"claude"}, "", claudeMDScopeBlock+claudeMDNoneTable))
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
	assert.NoFileExists(t, filepath.Join(root, "packages", "api", "CLAUDE.md"))
}
