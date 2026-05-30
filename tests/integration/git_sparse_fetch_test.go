package integration

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/includes"
	_ "github.com/Goldziher/ai-rulez/internal/includes" // register callbacks
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}
}

// initGitRepo initializes a local git repo and returns its path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	return dir
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// addAndCommit writes relPath in repoDir and commits.
func addAndCommit(t *testing.T, repoDir, relPath, content string) {
	t.Helper()
	path := filepath.Join(repoDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	for _, args := range [][]string{{"add", relPath}, {"commit", "-m", "update"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
}

func TestIntegration_SparseIncludeFetch(t *testing.T) {
	gitAvailable(t)

	repoDir := initGitRepo(t)
	addAndCommit(t, repoDir, ".ai-rulez/rules/rule1.md",
		"---\nname: rule1\npriority: high\n---\n# Rule One")

	consumerDir := t.TempDir()
	aiRulezDir := filepath.Join(consumerDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

	configYAML := `version: "3.0"
name: consumer
presets: [claude]
includes:
  - name: shared-rules
    source: "` + fileURL(repoDir) + `"
    include: [rules]
`
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configYAML), 0o644))

	cfg, err := config.LoadConfig(context.Background(), consumerDir)
	require.NoError(t, err)

	tree, err := cfg.GetContentForProfile("default")
	require.NoError(t, err)

	var found bool
	for _, r := range tree.Rules {
		if r.Name == "rule1" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected rule1 from include to appear in content tree")
}

func TestIntegration_SparseSkillFetch(t *testing.T) {
	gitAvailable(t)

	repoDir := initGitRepo(t)
	addAndCommit(t, repoDir, "skills/mything/SKILL.md",
		"---\nname: mything\n---\n# My Thing Skill\n\nThis is the skill body.")

	consumerDir := t.TempDir()
	aiRulezDir := filepath.Join(consumerDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

	configYAML := `version: "3.0"
name: consumer
presets: [claude]
installed_skills:
  - name: mything
    source: "` + fileURL(repoDir) + `"
    path: skills/mything
`
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configYAML), 0o644))

	cfg, err := config.LoadConfig(context.Background(), consumerDir)
	require.NoError(t, err)

	var found bool
	for _, sk := range cfg.Content.Skills {
		if sk.Name == "mything" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected skill 'mything' from installed_skills to appear in content")
}

func TestIntegration_CacheInvalidation(t *testing.T) {
	gitAvailable(t)

	repoDir := initGitRepo(t)
	addAndCommit(t, repoDir, ".ai-rulez/rules/rule1.md",
		"---\nname: rule1\npriority: high\n---\n# Rule One Version 1")

	consumerDir := t.TempDir()
	aiRulezDir := filepath.Join(consumerDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

	configYAML := `version: "3.0"
name: consumer
presets: [claude]
includes:
  - name: shared-invalidation
    source: "` + fileURL(repoDir) + `"
    include: [rules]
`
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configYAML), 0o644))

	// First load — populates cache.
	cfg1, err := config.LoadConfig(context.Background(), consumerDir)
	require.NoError(t, err)
	tree1, err := cfg1.GetContentForProfile("default")
	require.NoError(t, err)

	var firstContent string
	for _, r := range tree1.Rules {
		if r.Name == "rule1" {
			firstContent = r.Content
			break
		}
	}
	require.NotEmpty(t, firstContent, "rule1 should appear after first load")

	// Push a new commit.
	addAndCommit(t, repoDir, ".ai-rulez/rules/rule1.md",
		"---\nname: rule1\npriority: high\n---\n# Rule One Version 2")

	// Second load — SHA changed, cache invalidated.
	cfg2, err := config.LoadConfig(context.Background(), consumerDir)
	require.NoError(t, err)
	tree2, err := cfg2.GetContentForProfile("default")
	require.NoError(t, err)

	var secondContent string
	for _, r := range tree2.Rules {
		if r.Name == "rule1" {
			secondContent = r.Content
			break
		}
	}
	require.NotEmpty(t, secondContent, "rule1 should appear after second load")
	assert.NotEqual(t, firstContent, secondContent, "content should have changed after cache invalidation")
	assert.Contains(t, secondContent, "Version 2")
}

func TestIntegration_SkipFetch(t *testing.T) {
	gitAvailable(t)

	repoDir := initGitRepo(t)
	addAndCommit(t, repoDir, ".ai-rulez/rules/rule1.md",
		"---\nname: rule1\npriority: high\n---\n# Rule One")

	consumerDir := t.TempDir()
	aiRulezDir := filepath.Join(consumerDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

	configYAML := `version: "3.0"
name: consumer
presets: [claude]
includes:
  - name: shared-skipfetch
    source: "` + fileURL(repoDir) + `"
    include: [rules]
`
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configYAML), 0o644))

	// First load — populates cache.
	_, err := config.LoadConfig(context.Background(), consumerDir)
	require.NoError(t, err)

	// Enable SkipFetch, restore on test exit.
	includes.SkipFetch = true
	t.Cleanup(func() { includes.SkipFetch = false })

	// Rename the repo so any git access would fail.
	renamed := repoDir + "-renamed"
	require.NoError(t, os.Rename(repoDir, renamed))
	t.Cleanup(func() { os.Rename(renamed, repoDir) }) //nolint:errcheck

	// Second load — must succeed using cached content.
	cfg2, err := config.LoadConfig(context.Background(), consumerDir)
	require.NoError(t, err)

	tree, err := cfg2.GetContentForProfile("default")
	require.NoError(t, err)

	var found bool
	for _, r := range tree.Rules {
		if r.Name == "rule1" {
			found = true
			break
		}
	}
	assert.True(t, found, "cached content should be used when SkipFetch=true")
}
