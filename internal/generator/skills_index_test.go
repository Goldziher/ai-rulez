package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func indexProject(t *testing.T, extraConfig string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml":                       "version = \"5.0\"\nname = \"idx\"\npresets = [\"claude\", \"codex\"]\ngitignore = false\nagents_md = false\n" + extraConfig,
		".ai-rulez/skills/alpha/SKILL.md":             "---\nname: alpha\ndescription: Use when a thing happens.\nowner: team-a\nversion: 1.2.0\n---\nbody\n",
		".ai-rulez/skills/alpha/references/r.md":      "ref\n",
		".ai-rulez/skills/beta/SKILL.md":              "---\nname: beta\ndescription: Use when another thing happens.\n---\nbody\n",
		".ai-rulez/commands/Deploy_Now.md":            "---\nname: Deploy_Now\ndescription: Deploy right now.\n---\nbody\n",
		".ai-rulez/domains/ops/skills/gamma/SKILL.md": "---\nname: gamma\ndescription: Use when ops things happen.\n---\nbody\n",
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return root
}

func generateIndexProject(t *testing.T, root string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))
}

func TestSkillsIndex_OffByDefault(t *testing.T) {
	root := indexProject(t, "")
	generateIndexProject(t, root)

	_, err := os.Stat(filepath.Join(root, ".ai-rulez", usage.IndexFileName))
	assert.True(t, os.IsNotExist(err), "no index unless [usage] skills_index is on")
}

func TestSkillsIndex_ListsEverySkillWithItsOutputs(t *testing.T) {
	root := indexProject(t, "[usage]\nskills_index = true\n")
	generateIndexProject(t, root)

	index, err := usage.LoadIndex(filepath.Join(root, ".ai-rulez", usage.IndexFileName))
	require.NoError(t, err)
	require.Len(t, index.Skills, 4)

	byID := map[string]usage.SkillRecord{}
	for _, record := range index.Skills {
		byID[record.ID] = record
	}
	alpha := byID["alpha"]
	assert.Equal(t, ".ai-rulez/skills/alpha/SKILL.md", alpha.Source)
	assert.Equal(t, "team-a", alpha.Owner)
	assert.Equal(t, "1.2.0", alpha.Version)
	assert.Regexp(t, `^blake3:[0-9a-f]{64}$`, alpha.Hash)
	assert.Equal(t, []string{".claude/skills/alpha/SKILL.md"}, alpha.Outputs["claude"])
	assert.Equal(t, []string{".agents/skills/alpha/SKILL.md"}, alpha.Outputs["codex"])
	assert.Equal(t, "ops", byID["gamma"].Domain)
	assert.Equal(t, usage.KindSkill, alpha.Kind)
	command, ok := byID["deploy-now"]
	require.True(t, ok, "a command written as a skill is indexed under its skill directory name")
	assert.Equal(t, usage.KindCommand, command.Kind)
	assert.Equal(t, []string{".claude/skills/deploy-now/SKILL.md"}, command.Outputs["claude"])
	assert.NotEqual(t, alpha.Hash, byID["beta"].Hash)

	for _, record := range index.Skills {
		for preset, paths := range record.Outputs {
			for _, path := range paths {
				assert.FileExists(t, filepath.Join(root, filepath.FromSlash(path)), "%s output of %s", preset, record.ID)
			}
		}
	}
}

func TestSkillsIndex_ByteStableAndTracksEdits(t *testing.T) {
	root := indexProject(t, "[usage]\nskills_index = true\n")
	path := filepath.Join(root, ".ai-rulez", usage.IndexFileName)

	generateIndexProject(t, root)
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	generateIndexProject(t, root)
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "the index must be byte-stable across runs")

	// Editing a bundled resource changes that skill's hash and only that one's.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "skills", "alpha", "references", "r.md"), []byte("changed\n"), 0o600))
	generateIndexProject(t, root)
	third, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEqual(t, string(first), string(third))

	before, after := indexHashes(t, first), indexHashes(t, third)
	assert.NotEqual(t, before["alpha"], after["alpha"])
	assert.Equal(t, before["beta"], after["beta"])
}

func indexHashes(t *testing.T, data []byte) map[string]string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "i.json")
	require.NoError(t, os.WriteFile(file, data, 0o600))
	index, err := usage.LoadIndex(file)
	require.NoError(t, err)
	hashes := map[string]string{}
	for _, record := range index.Skills {
		hashes[record.ID] = record.Hash
	}
	return hashes
}

func TestSkillsIndex_IsCleanedWithTheOtherOutputs(t *testing.T) {
	root := indexProject(t, "[usage]\nskills_index = true\n")
	generateIndexProject(t, root)
	path := filepath.Join(root, ".ai-rulez", usage.IndexFileName)
	require.FileExists(t, path)

	// Turning the feature off removes the stale index through the manifest.
	configPath := filepath.Join(root, ".ai-rulez", "config.toml")
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, []byte(string(raw[:len(raw)-len("[usage]\nskills_index = true\n")])), 0o600))
	generateIndexProject(t, root)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

// The skills index, the eval store and the lock must describe one skill with one
// digest, or usage cannot join with eval results. This is the regression gate.
func TestSkillsIndex_DigestEqualsTheLockAndEvalDigest(t *testing.T) {
	root := indexProject(t, "[usage]\nskills_index = true\n")
	evalCase := filepath.Join(root, ".ai-rulez", "skills", "alpha", "evals", "main.eval.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(evalCase), 0o750))
	require.NoError(t, os.WriteFile(evalCase, []byte("cases: []\n"), 0o600))
	generateIndexProject(t, root)

	index, err := usage.LoadIndex(filepath.Join(root, ".ai-rulez", usage.IndexFileName))
	require.NoError(t, err)
	indexed := map[string]string{}
	for _, record := range index.Skills {
		indexed[record.ID] = record.Digest
	}
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	snap, err := contentlock.Compute(cfg, contentlock.Options{})
	require.NoError(t, err)
	locked := map[string]string{}
	for _, item := range snap.Items {
		if item.Kind == contentlock.KindSkill {
			locked[item.ID] = item.Digest
		}
	}

	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, indexed["alpha"])
	assert.Equal(t, locked["alpha"], indexed["alpha"], "the index carries the lock's digest")
	assert.Equal(t, locked["beta"], indexed["beta"])
	fromDir, err := contentlock.SkillDirDigest(filepath.Join(root, ".ai-rulez", "skills", "alpha"))
	require.NoError(t, err)
	assert.Equal(t, fromDir, indexed["alpha"], "the eval store's digest of the directory agrees")

	require.NoError(t, os.WriteFile(evalCase, []byte("cases: []\n# edited\n"), 0o600))
	generateIndexProject(t, root)
	index, err = usage.LoadIndex(filepath.Join(root, ".ai-rulez", usage.IndexFileName))
	require.NoError(t, err)
	for _, record := range index.Skills {
		if record.ID == "alpha" {
			assert.Equal(t, indexed["alpha"], record.Digest, "editing an eval case is not editing the skill")
		}
	}
}
