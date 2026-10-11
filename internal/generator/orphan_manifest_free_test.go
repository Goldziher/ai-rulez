package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// orphanProject writes a project with one agent, one skill and a bundled
// reference/script asset, plus a rule, generating CLAUDE.md and the claude
// outputs. It is the shape a project that commits its output has.
func orphanProject(t *testing.T, presets string) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"5.0\"\nname = \"orphans\"\npresets = ["+presets+"]\n"+v4Defaults+hdr("full")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "style.md"),
		[]byte("---\npriority: high\n---\n# Style\n\nUse tabs.\n"), 0o644))
	agentDir := filepath.Join(configDir, "agents")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "reviewer.md"),
		[]byte("---\nname: reviewer\ndescription: Review things.\n---\n# Reviewer\n\nBody.\n"), 0o644))
	skillDir := filepath.Join(configDir, "skills", "alpha", "scripts")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: Use alpha.\n---\n# alpha\n\nBody.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755))
	return dir
}

// freshWorktree drops the generated manifests, the state a committed-output
// project has in a fresh checkout (the manifests are gitignored).
func freshWorktree(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{generatedManifestName, generatedLocalManifestName} {
		err := os.Remove(filepath.Join(dir, ".ai-rulez", name))
		if err != nil {
			require.True(t, os.IsNotExist(err), name)
		}
	}
}

func assetPath(dir string) string {
	return filepath.Join(dir, ".claude", "skills", "alpha", "scripts", "run.sh")
}

// #332: a committed output carrying the ai-rulez header is overwritten without
// --force in a fresh worktree (no manifest), and a header-less bundled asset
// whose source changed is overwritten too: its rendered sibling proves ai-rulez
// owns the skill directory.
func TestGenerate_OverwritesCommittedOutputsWithoutTheManifest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	skill := filepath.Join(dir, ".claude", "skills", "alpha", "SKILL.md")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: Use alpha.\n---\n# alpha\n\nEdited body.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "skills", "alpha", "scripts", "run.sh"),
		[]byte("#!/bin/sh\necho edited\n"), 0o755))
	freshWorktree(t, dir)

	// Act
	err := loadHashesProject(t, dir).Generate("")

	// Assert
	require.NoError(t, err, "the committed-output workflow needs no manifest and no --force")
	assert.Contains(t, readFileString(t, skill), "Edited body.")
	assert.Contains(t, readFileString(t, assetPath(dir)), "echo edited")
}

// #332: a hand-written file with no ai-rulez header is still refused.
func TestGenerate_StillRefusesAHandWrittenFileWithoutTheManifest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	freshWorktree(t, dir)
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))

	// Act
	err := loadHashesProject(t, dir).Generate("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE.md")
	assert.Contains(t, err.Error(), "--force")
	assert.Equal(t, handWritten, readFileString(t, claude), "the hand-written file is untouched")
}

// #332: a symlinked output is still refused, even with the header proof present.
func TestGenerate_StillRefusesASymlinkedOutputWithoutTheManifest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	freshWorktree(t, dir)
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.Remove(claude))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "elsewhere.md"), []byte(handWritten), 0o644))
	testutil.SymlinkOrSkip(t, filepath.Join(dir, "elsewhere.md"), claude)

	// Act
	err := loadHashesProject(t, dir).Generate("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	info, lerr := os.Lstat(claude)
	require.NoError(t, lerr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is the user's")
}

// #333: without a manifest, an agent whose source was removed is reported as an
// orphan by --check and removed by generate.
func TestGenerate_RemovesAndReportsAStaleAgentWithoutTheManifest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	require.FileExists(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", "agents", "reviewer.md")))
	freshWorktree(t, dir)

	// Act: --check reports the orphan, then generate removes it.
	drift, err := loadHashesProject(t, dir).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, drift, Drift{Path: ".claude/agents/reviewer.md", Kind: DriftOrphan})
	require.FileExists(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"), "--check must not write")
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"))
}

// #333: a hand-written agent (no header) under a generated directory is not
// removed by the orphan pass.
func TestGenerate_KeepsAHandWrittenAgentWithoutTheManifest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", "agents", "reviewer.md")))
	freshWorktree(t, dir)
	mine := filepath.Join(dir, ".claude", "agents", "mine.md")
	require.NoError(t, os.WriteFile(mine, []byte("# my agent\n\nKeep me.\n"), 0o644))

	// Act
	require.NoError(t, loadHashesProject(t, dir).Generate(""))

	// Assert
	assert.FileExists(t, mine)
	assert.Equal(t, "# my agent\n\nKeep me.\n", readFileString(t, mine))
}

// #328: a bundled asset the manifest recorded, whose source is gone, is
// reported by --check and removed by generate even when the machine-local
// manifest (which holds the digest) is absent, as in a fresh checkout.
func TestGenerate_RemovesAndReportsARecordedBundledAsset(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "skills", "alpha")))
	require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", generatedLocalManifestName)))

	// Act: --check reports the orphan, then generate removes it.
	drift, err := loadHashesProject(t, dir).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, drift, Drift{Path: ".claude/skills/alpha/scripts/run.sh", Kind: DriftOrphan})
	require.FileExists(t, assetPath(dir), "--check must not write")
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	assert.NoFileExists(t, assetPath(dir))
}

// #328: with no manifest at all a header-less asset cannot be proven and is left
// alone, and --check does not report it.
func TestGenerate_LeavesHeaderlessAssetsAloneWithNoManifest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "skills", "alpha")))
	freshWorktree(t, dir)

	// Act
	drift, err := loadHashesProject(t, dir).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, drift, Drift{Path: ".claude/skills/alpha/scripts/run.sh", Kind: DriftOrphan})
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	assert.FileExists(t, assetPath(dir), "an unprovable asset is left for the user")
}

// #327 follow-through: a plugin-authoring root that becomes empty writes no
// instruction file, so the previously written root file is removed as an orphan
// and reported.
func TestGenerate_RemovesTheRootFileWhenTheContentTreeBecomesEmpty(t *testing.T) {
	quietWarnings(t)
	// Arrange: a plugin-authoring root with one rule writes CLAUDE.md.
	dir := t.TempDir()
	writeRootConfig(t, dir, pluginOnlyRootConfig("agents_md = false\n", ""))
	writeAgentsMDFile(t, dir, ".ai-rulez/rules/style.md", "# Style\n\nUse tabs.\n")
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	require.FileExists(t, filepath.Join(dir, "CLAUDE.md"))

	// The rule goes, leaving a plugin-authoring root with no content.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "rules")))
	freshWorktree(t, dir)

	// Act
	drift, err := loadHashesProject(t, dir).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, drift, Drift{Path: "CLAUDE.md", Kind: DriftOrphan})
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))
}

// #328 trust boundary: a forged asset entry whose digest does not match the file
// does not authorize deletion, so a hand-written file under a skills directory
// survives.
func TestGenerate_ForgedAssetEntryDoesNotDeleteAUserFile(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := orphanProject(t, `"claude"`)
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	mine := filepath.Join(dir, ".claude", "skills", "mine", "notes.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(mine), 0o755))
	body := "# my notes\n\nKeep me.\n"
	require.NoError(t, os.WriteFile(mine, []byte(body), 0o644))
	forgeManifestAssets(t, dir, map[string]string{".claude/skills/mine/notes.md": "0000000000000000"})

	// Act
	require.NoError(t, loadHashesProject(t, dir).Generate(""))

	// Assert
	assert.Equal(t, body, readFileString(t, mine))
}

// #350 regression: generate --plugin writes its bundle into the project tree
// and stamps each file with the same generated header, so the manifest-free
// orphan walk must neither report a committed bundle in --check nor delete it
// in generate.
func TestGenerate_LeavesACommittedPluginBundleAlone(t *testing.T) {
	quietWarnings(t)
	// Arrange: a root [plugin] whose skill carries a bundled reference asset.
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "skills", "alpha", "references"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"5.0\"\nname = \"bundle\"\ngitignore = false\npresets = [\"claude\"]\n\n"+
			"[plugin]\nname = \"bundle\"\nversion = \"1.0.0\"\ndescription = \"d\"\nruntimes = [\"claude\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: Use alpha.\n---\n# alpha\n\nBody.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "skills", "alpha", "references", "api.md"),
		[]byte("# API\n\nReference.\n"), 0o644))
	require.NoError(t, loadHashesProject(t, dir).GeneratePlugin(""))
	bundled := filepath.Join(dir, "skills", "alpha", "references", "api.md")
	require.FileExists(t, bundled)
	freshWorktree(t, dir)

	// Act: --check must not call the committed bundle file an orphan.
	drift, err := loadHashesProject(t, dir).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, drift, Drift{Path: "skills/alpha/references/api.md", Kind: DriftOrphan})
	require.FileExists(t, bundled, "--check must not write")
	require.NoError(t, loadHashesProject(t, dir).Generate(""))
	assert.FileExists(t, bundled, "generate must not delete a committed plugin bundle")
}

// forgeManifestAssets sets the committed manifest's asset digests, the way a
// hostile commit to .ai-rulez/.generated-manifest.json would.
func forgeManifestAssets(t *testing.T, dir string, assets map[string]string) {
	t.Helper()
	path := filepath.Join(dir, ".ai-rulez", generatedManifestName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["assets"] = assets
	out, err := json.MarshalIndent(raw, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}
