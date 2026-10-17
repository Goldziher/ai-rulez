package commands

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindConfigFilesRecursively_RootLayouts(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"a root layout at the top", map[string]string{"ai-rulez.toml": rootLayoutConfig}, []string{"ai-rulez.toml"}},
		{
			"nested roots",
			map[string]string{"a/ai-rulez.toml": rootLayoutConfig, "a/b/c/ai-rulez.toml": rootLayoutConfig},
			[]string{"a/ai-rulez.toml", "a/b/c/ai-rulez.toml"},
		},
		{
			"a config directory beside the file wins and the file is not a second root",
			map[string]string{"svc/ai-rulez.toml": rootLayoutConfig, "svc/.ai-rulez/config.toml": rootLayoutConfig},
			[]string{"svc/.ai-rulez/config.toml"},
		},
		{
			"the .config convention beside the file wins",
			map[string]string{"svc/ai-rulez.toml": rootLayoutConfig, "svc/.config/ai-rulez/config.toml": rootLayoutConfig},
			[]string{"svc/.config/ai-rulez/config.toml"},
		},
		{
			"the .agents layout beside the file wins",
			map[string]string{"svc/ai-rulez.toml": rootLayoutConfig, "svc/.agents/ai-rulez.toml": rootLayoutConfig},
			[]string{"svc/.agents/ai-rulez.toml"},
		},
		{
			"a root layout below a .ai-rulez root is its own root",
			map[string]string{".ai-rulez/config.toml": rootLayoutConfig, "packages/tool/ai-rulez.toml": rootLayoutConfig},
			[]string{".ai-rulez/config.toml", "packages/tool/ai-rulez.toml"},
		},
		{
			"pruned and hidden directories hold no root",
			map[string]string{
				"node_modules/pkg/ai-rulez.toml": rootLayoutConfig,
				"vendor/x/ai-rulez.toml":         rootLayoutConfig,
				".hidden/ai-rulez.toml":          rootLayoutConfig,
			},
			nil,
		},
		{"an ai-rulez.toml that is a directory is no root", map[string]string{"svc/ai-rulez.toml/x": "y"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, body := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), body)
			}
			chdir(t, root)
			resetAllFlags()

			// Act
			got := findConfigFilesRecursively()

			// Assert
			for i := range got {
				got[i] = filepath.ToSlash(got[i])
			}
			sort.Strings(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindConfigFilesRecursively_ExplicitConfigDirIgnoresRootLayouts(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "svc", "ai-rulez.toml"), rootLayoutConfig)
	writeFile(t, filepath.Join(root, "svc", ".custom", "config.toml"), rootLayoutConfig)
	chdir(t, root)
	resetAllFlags()
	previous := configDir
	configDir = ".custom"
	t.Cleanup(func() { configDir = previous })

	// Act
	got := findConfigFilesRecursively()

	// Assert
	assert.Equal(t, []string{filepath.Join("svc", ".custom", "config.toml")}, got)
}

func TestRecursiveGenerate_CoversRootLayoutsNextToDirectoryRoots(t *testing.T) {
	// Arrange: a monorepo with a .ai-rulez root and a single-file package below it.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "r.md"), "# r\n\nbody\n")
	writeFile(t, filepath.Join(root, "packages", "tool", "ai-rulez.toml"), rootLayoutInstalled)
	writeFile(t, filepath.Join(root, "packages", "tool", "shared", "skills", "demo", "SKILL.md"), rootLayoutSkill)
	writeFile(t, filepath.Join(root, "packages", "tool", "CLAUDE.md"), "# Hand written\n")
	chdir(t, root)
	resetAllFlags()

	// Act
	code := runRecursiveGenerate()

	// Assert
	require.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(root, ".ai-rulez", ".generated-manifest.json"))
	tool := filepath.Join(root, "packages", "tool")
	assert.FileExists(t, filepath.Join(tool, ".ai-rulez-manifest.json"))
	assert.FileExists(t, filepath.Join(tool, ".claude", "skills", "demo", "SKILL.md"))
	assert.Equal(t, "# Hand written\n", readFileString(t, filepath.Join(tool, "CLAUDE.md")))
	assert.NoDirExists(t, filepath.Join(tool, ".ai-rulez"))
}
