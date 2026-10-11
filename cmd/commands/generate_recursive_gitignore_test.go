package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

func gitignoreFlag(b bool) *bool { return &b }

// TestApplyGitignoreOverride pins the --gitignore policy: a single root keeps
// the previous behavior (the flag turns managed ignores on), while a recursive
// run never silently contradicts a root's explicit `gitignore = false` (#316).
func TestApplyGitignoreOverride(t *testing.T) {
	tests := []struct {
		name      string
		flag      bool
		recursive bool
		gitignore *bool
		want      bool
	}{
		{"flag off leaves the config alone", false, false, gitignoreFlag(false), false},
		{"single root: flag overrides explicit false", true, false, gitignoreFlag(false), true},
		{"single root: flag enables an unset value", true, false, nil, true},
		{"recursive: explicit false is respected", true, true, gitignoreFlag(false), false},
		{"recursive: unset value is enabled", true, true, nil, true},
		{"recursive: explicit true stays true", true, true, gitignoreFlag(true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { updateGitignore, recursive = false, false })
			updateGitignore, recursive = tt.flag, tt.recursive
			cfg := &config.Config{Gitignore: tt.gitignore}

			applyGitignoreOverride(cfg)

			assert.Equal(t, tt.want, cfg.ShouldUpdateGitignore())
		})
	}
}

// TestRecursiveGenerate_GitignoreIsOneToplevelBlock is the end-to-end
// regression test for #316: `generate --recursive --gitignore` writes one
// managed block at the git toplevel with each root's patterns relative to it,
// creates no .gitignore inside a nested root, and leaves a root whose config
// says `gitignore = false` out of the block.
func TestRecursiveGenerate_GitignoreIsOneToplevelBlock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", root, "init", "-q").Run()) //nolint:gosec // test

	// Root "a" opts into managed ignores and generates a skill.
	writeFile(t, filepath.Join(root, "a", ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"a\"\npresets = [\"claude\"]\ngitignore = true\nagents_md = false\n")
	writeFile(t, filepath.Join(root, "a", ".ai-rulez", "skills", "demo", "SKILL.md"),
		"---\nname: demo\ndescription: Demo.\n---\n\nDo it.\n")
	// Root "b" explicitly commits its outputs.
	writeFile(t, filepath.Join(root, "b", ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"b\"\npresets = [\"claude\"]\ngitignore = false\nagents_md = false\n")
	chdir(t, root)

	progress.SetQuiet(true)
	t.Cleanup(func() {
		progress.SetQuiet(false)
		pluginMode, dryRun, profile, recursive, updateGitignore = false, false, "", false, false
	})
	pluginMode, dryRun, profile, recursive, updateGitignore = false, false, "", true, true

	code := runRecursiveGenerate()
	require.Equal(t, 0, code)

	// One block at the toplevel, holding root a's patterns relative to it.
	content := readIgnoreFile(t, filepath.Join(root, ".gitignore"))
	assert.Contains(t, content, "a/CLAUDE.md")
	assert.Contains(t, content, "a/.claude/skills/")

	// Root b's outputs are committed on purpose: nothing of it may be listed.
	assert.NotContains(t, content, "b/CLAUDE.md")
	assert.NotContains(t, content, "b/.claude")

	// No .gitignore was created inside either nested root.
	for _, dir := range []string{"a", "b"} {
		_, err := os.Stat(filepath.Join(root, dir, ".gitignore"))
		assert.True(t, os.IsNotExist(err), "%s must not gain a .gitignore", dir)
	}
}

// TestRecursiveGenerate_DryRunDoesNotTouchTheSharedBlock pins that a preview
// writes nothing, leaving an existing toplevel block untouched.
func TestRecursiveGenerate_DryRunDoesNotTouchTheSharedBlock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", root, "init", "-q").Run()) //nolint:gosec // test
	writeFile(t, filepath.Join(root, "a", ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"a\"\npresets = [\"claude\"]\ngitignore = true\nagents_md = false\n")
	block := "# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)\na/CLAUDE.md\n# END ai-rulez\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(block), 0o644))
	chdir(t, root)

	progress.SetQuiet(true)
	t.Cleanup(func() {
		progress.SetQuiet(false)
		pluginMode, dryRun, profile, recursive, updateGitignore = false, false, "", false, false
	})
	pluginMode, dryRun, profile, recursive, updateGitignore = false, true, "", true, false

	require.Equal(t, 0, runRecursiveGenerate())

	assert.Equal(t, block, readIgnoreFile(t, filepath.Join(root, ".gitignore")))
}

func readIgnoreFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}
