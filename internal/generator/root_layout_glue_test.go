package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

const rootGlueConfig = "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n"

func writeRootGlueFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// rootGlueGenerator loads the root-layout project at root.
func rootGlueGenerator(t *testing.T, root string) (*Generator, *config.Config) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(includes.Resolvers("", nil)))
	require.NoError(t, err)
	require.True(t, cfg.Contentless())
	return NewGenerator(cfg), cfg
}

func TestRootLayoutGlue_NamesAndGlobs(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeRootGlueFile(t, root, "ai-rulez.toml", rootGlueConfig)
	g, cfg := rootGlueGenerator(t, root)

	// Act and Assert
	assert.Equal(t, "ai-rulez.local.*", g.localOverlayGlob())
	assert.Equal(t, ".ai-rulez-local.*", g.localLockGlob())
	assert.Equal(t, filepath.Join(cfg.ConfigDir, ".ai-rulez-local.d", ".role-skill-overrides.json"), g.roleLedgerPath())
	assert.True(t, g.shouldSkipPath("ai-rulez.toml"), "the configuration file is never ignored as an output")
	assert.False(t, g.shouldSkipPath(".ai-rulez/x"), "no config directory exists to protect")
	assert.False(t, g.shouldSkipPath(".claude/skills/demo/SKILL.md"))
}

func TestDirectoryLayoutGlue_NamesAndGlobs(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeRootGlueFile(t, root, ".ai-rulez/config.toml", rootGlueConfig)
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)
	g := NewGenerator(cfg)

	// Act and Assert
	assert.Equal(t, "config.local.*", g.localOverlayGlob())
	assert.Equal(t, ".ai-rulez/.config.local.*", g.localLockGlob())
	assert.Equal(t, filepath.Join(cfg.ConfigDir, "local", ".role-skill-overrides.json"), g.roleLedgerPath())
	assert.True(t, g.shouldSkipPath(".ai-rulez/rules/r.md"))
	assert.False(t, g.shouldSkipPath("ai-rulez.toml"))
}

func TestRootLayoutGlue_LocalInputsOnDisk(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"nothing", nil, false},
		{"an overlay", map[string]string{"ai-rulez.local.toml": "description = \"x\"\n"}, true},
		{"a local directory is not a content tree here", map[string]string{"local/rules/r.md": "# R\n"}, false},
		{"a config.local file of the directory layout is not the overlay", map[string]string{"config.local.toml": "x = 1\n"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeRootGlueFile(t, root, "ai-rulez.toml", rootGlueConfig)
			for rel, body := range tt.files {
				writeRootGlueFile(t, root, rel, body)
			}
			g, _ := rootGlueGenerator(t, root)

			// Act
			got := g.localInputsOnDisk()

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRootLayoutGlue_LocalGitignorePatterns(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"no local files", nil, nil},
		{"an overlay", map[string]string{"ai-rulez.local.toml": "description = \"x\"\n"}, []string{"ai-rulez.local.*"}},
		{
			"an overlay and its lock file",
			map[string]string{"ai-rulez.local.toml": "description = \"x\"\n", ".ai-rulez-local.lock": ""},
			[]string{"ai-rulez.local.*", ".ai-rulez-local.*"},
		},
		{"a local directory is not ignored as content", map[string]string{"local/rules/r.md": "# R\n"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeRootGlueFile(t, root, "ai-rulez.toml", rootGlueConfig)
			for rel, body := range tt.files {
				writeRootGlueFile(t, root, rel, body)
			}
			g, _ := rootGlueGenerator(t, root)

			// Act
			got := g.localGitignorePatternsOnDisk()

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRootLayoutGlue_OutputNameLists(t *testing.T) {
	assert.Equal(t,
		[]string{".generated-manifest.json", ".generated-manifest.local.json", ".ai-rulez-manifest.json", ".ai-rulez-manifest.local.json"},
		GeneratedManifestNames())
	assert.Equal(t,
		[]string{".generated-manifest.json", ".generated-manifest.local.json", roles.FileName, ".ai-rulez-manifest.json", ".ai-rulez-manifest.local.json"},
		ConfigDirOutputNames())
}

func TestRootLayoutGenerate_ManagedGitignoreBlock(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeRootGlueFile(t, root, "ai-rulez.toml", rootGlueConfig+"gitignore = true\n\n[[installed_skills]]\nname = \"demo\"\nsource = \"./shared\"\n")
	writeRootGlueFile(t, root, "shared/skills/demo/SKILL.md", "---\nname: demo\ndescription: A demo skill installed from a local source for tests\n---\n\nDo it.\n")
	writeRootGlueFile(t, root, "ai-rulez.local.toml", "description = \"mine\"\n")
	writeRootGlueFile(t, root, "CLAUDE.md", "# Mine\n")
	writeRootGlueFile(t, root, "AGENTS.md", "# Mine\n")
	g, _ := rootGlueGenerator(t, root)

	// Act
	require.NoError(t, g.Generate(""))

	// Assert
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	lines := strings.Split(string(body), "\n")
	for _, want := range []string{".claude/skills/", ".ai-rulez-manifest.json", ".ai-rulez-manifest.local.json", "ai-rulez.local.*"} {
		assert.Contains(t, lines, want)
	}
	for _, unwanted := range []string{"ai-rulez.toml", "CLAUDE.md", "AGENTS.md", "ai-rulez.lock", ".ai-rulez/"} {
		assert.NotContains(t, lines, unwanted)
	}
	assert.Equal(t, "# Mine\n", readFile(t, filepath.Join(root, "CLAUDE.md")))
	assert.Equal(t, "# Mine\n", readFile(t, filepath.Join(root, "AGENTS.md")))
}
