package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

var snapshotProject = map[string]string{
	".ai-rulez/config.toml":                     "version = \"4.0\"\nname = \"snap\"\npresets = [\"claude\"]\n",
	".ai-rulez/rules/style.md":                  "---\npriority: high\n---\n# Style\nBe concise.\n",
	".ai-rulez/context/arch.md":                 "# Architecture\n",
	".ai-rulez/skills/review/SKILL.md":          "---\nname: review\ndescription: Review code\n---\nBody\n",
	".ai-rulez/skills/review/references/api.md": "# API\n",
	".ai-rulez/domains/backend/rules/db.md":     "# DB\n",
	".ai-rulez/agents/helper.md":                "---\nname: helper\ndescription: Helps\n---\nHelp.\n",
}

func commitProject(t *testing.T, dir string) string {
	t.Helper()
	for name, content := range snapshotProject {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	git := func(args ...string) []byte {
		all := append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		cmd := exec.Command("git", all...) //nolint:gosec // fixed test arguments
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return out
	}
	git("init", "-q", ".")
	git("add", "-A")
	git("commit", "-q", "-m", "project")
	return string(git("rev-parse", "HEAD"))[:40]
}

// The same project loads to the same content from the disk, from memory and from
// a commit, which is what lets a service plan a commit it never checked out.
func TestTheSameProjectLoadsIdenticallyFromEveryWorkspace(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	commit := commitProject(t, dir)
	disk, err := workspace.OS(dir)
	require.NoError(t, err)
	mem := workspace.NewMem(disk.Root())
	for name, content := range snapshotProject {
		mem.Set(name, content, 0o644)
	}
	snapshot, err := workspace.GitSnapshot(t.Context(), dir, commit, nil)
	require.NoError(t, err)

	// Act
	var loaded []*config.Config
	for _, ws := range []workspace.Workspace{disk, mem, snapshot} {
		cfg, err := config.LoadConfig(t.Context(), ".", config.WithWorkspace(ws), config.WithoutRemote())
		require.NoError(t, err)
		loaded = append(loaded, cfg)
	}

	// Assert
	assert.NotEmpty(t, loaded[0].Content.Rules)
	require.Len(t, loaded[0].Content.Skills, 1)
	assert.Len(t, loaded[0].Content.Skills[0].Resources, 1)
	for i, name := range []string{"memory", "git snapshot"} {
		assert.Equal(t, loaded[0].BaseDir, loaded[i+1].BaseDir, name)
		assert.Equal(t, loaded[0].Content, loaded[i+1].Content, name)
		assert.Equal(t, loaded[0].Name, loaded[i+1].Name, name)
	}
}

func TestASnapshotIgnoresWhatChangedOnDiskAfterTheCommit(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	commit := commitProject(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "rules", "style.md"), []byte("# Edited on disk\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "rules", "untracked.md"), []byte("# Untracked\n"), 0o644))
	snapshot, err := workspace.GitSnapshot(t.Context(), dir, commit, nil)
	require.NoError(t, err)

	// Act
	cfg, err := config.LoadConfig(t.Context(), ".", config.WithWorkspace(snapshot), config.WithoutRemote())

	// Assert
	require.NoError(t, err)
	var names []string
	for _, r := range cfg.Content.Rules {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"style"}, names)
	assert.Contains(t, cfg.Content.Rules[0].Content, "Be concise.")
}
