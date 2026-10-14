package commands

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

func TestFindConfigFilesRecursively_NestedAgentsConfigs(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "ai-rulez.toml"), "version = \"5.0\"\nname = \"root\"\n")
	writeFile(t, filepath.Join(root, "svc-a", ".agents", "ai-rulez.toml"), "version = \"5.0\"\nname = \"a\"\n")
	writeFile(t, filepath.Join(root, "svc-b", ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"b\"\n")
	// An .agents directory with no ai-rulez.toml is another tool's, not a root.
	writeFile(t, filepath.Join(root, "svc-c", ".agents", "skills", "x", "SKILL.md"), "x")
	// Skills under a root's .agents are content, never a nested root.
	writeFile(t, filepath.Join(root, ".agents", "skills", "x", ".agents", "ai-rulez.toml"), "x = 1")
	chdir(t, root)

	// Act
	got := findConfigFilesRecursively()
	sort.Strings(got)

	// Assert
	assert.Equal(t, []string{
		filepath.Join(".agents", "ai-rulez.toml"),
		filepath.Join("svc-a", ".agents", "ai-rulez.toml"),
		filepath.Join("svc-b", ".ai-rulez", "config.toml"),
	}, got)
}

func TestRunRecursiveGenerate_NestedAgentsProjects(t *testing.T) {
	// Arrange
	root := t.TempDir()
	rootMD := "# Root\n\nROOT_TEXT\n"
	nestedMD := "# Service\n\nSERVICE_TEXT\n"
	skill := "---\nname: review\ndescription: Review\n---\n\nSKILL_TEXT\n"
	writeFile(t, filepath.Join(root, ".agents", "ai-rulez.toml"), "version = \"5.0\"\nname = \"root\"\npresets = [\"claude\"]\ngitignore = false\n")
	writeFile(t, filepath.Join(root, "AGENTS.md"), rootMD)
	writeFile(t, filepath.Join(root, "svc", ".agents", "ai-rulez.toml"), "version = \"5.0\"\nname = \"svc\"\npresets = [\"claude\"]\ngitignore = false\n")
	writeFile(t, filepath.Join(root, "svc", "AGENTS.md"), nestedMD)
	writeFile(t, filepath.Join(root, "svc", ".agents", "skills", "review", "SKILL.md"), skill)
	chdir(t, root)
	progress.SetQuiet(true)
	t.Cleanup(func() {
		progress.SetQuiet(false)
		pluginMode, dryRun, profile = false, false, ""
	})
	pluginMode, dryRun, profile = false, false, ""

	// Act
	code := runRecursiveGenerate()

	// Assert
	require.Equal(t, 0, code)
	rootClaude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(rootClaude), "ROOT_TEXT")
	assert.NotContains(t, string(rootClaude), "SERVICE_TEXT", "a nested project's AGENTS.md belongs to that project")
	svcClaude, err := os.ReadFile(filepath.Join(root, "svc", "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(svcClaude), "SERVICE_TEXT")
	assert.FileExists(t, filepath.Join(root, "svc", ".claude", "skills", "review", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(root, ".claude", "skills", "review", "SKILL.md"))
	for rel, want := range map[string]string{
		"AGENTS.md":                          rootMD,
		"svc/AGENTS.md":                      nestedMD,
		"svc/.agents/skills/review/SKILL.md": skill,
	} {
		got, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		require.NoError(t, readErr)
		assert.Equal(t, want, string(got), "%s is source and must not change", rel)
	}
}
