package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

func TestGenerate_NamePrefixAppliesToTheInstalledSkillDirectory(t *testing.T) {
	tests := []struct {
		name     string
		presets  string
		wantDirs []string // relative to the project, one SKILL.md each
	}{
		{
			name: "claude own tree", presets: `["claude"]`,
			wantDirs: []string{".claude/skills/a-shared", ".claude/skills/b-shared"},
		},
		{
			name: "shared tree", presets: `["codex"]`,
			wantDirs: []string{".agents/skills/a-shared", ".agents/skills/b-shared"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: two sources whose skill directories have the same name
			root := t.TempDir()
			files := map[string]string{
				".ai-rulez/config.toml": "version = \"5.0\"\nname = \"pfx\"\ngitignore = false\npresets = " + tt.presets + "\n" +
					"[[installed_skills]]\nname = \"one\"\nsource = \"vendor/a\"\npath = \"skills\"\nname_prefix = \"a-\"\n" +
					"[[installed_skills]]\nname = \"two\"\nsource = \"vendor/b\"\npath = \"skills\"\nname_prefix = \"b-\"\n",
				"vendor/a/skills/shared/SKILL.md": "---\ndescription: From A\n---\nBODY-A\n",
				"vendor/b/skills/shared/SKILL.md": "---\ndescription: From B\n---\nBODY-B\n",
			}
			for rel, content := range files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			}
			cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(includes.Resolvers("", nil)))
			require.NoError(t, err)

			// Act
			require.NoError(t, NewGenerator(cfg).Generate(""))

			// Assert
			for i, dir := range tt.wantDirs {
				body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "SKILL.md"))
				require.NoError(t, err, dir)
				assert.Contains(t, string(body), "name: "+filepath.Base(dir), "the directory and the name agree")
				assert.Contains(t, string(body), "BODY-"+string('A'+rune(i)))
			}
			assert.NoDirExists(t, filepath.Join(root, ".claude", "skills", "shared"))
			assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "shared"))
		})
	}
}

func TestGenerate_RootLayoutPrunesTheDirectoryOfARenamedInstalledSkill(t *testing.T) {
	// Arrange: a project without a config folder, generated under one prefix
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	toml := func(prefix string) string {
		return "version = \"5.0\"\nname = \"pfx\"\ngitignore = false\npresets = [\"claude\"]\n" +
			"[[installed_skills]]\nname = \"one\"\nsource = \"vendor/a\"\npath = \"skills\"\nname_prefix = \"" + prefix + "\"\n"
	}
	generate := func() {
		cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(includes.Resolvers("", nil)))
		require.NoError(t, err)
		require.True(t, cfg.Contentless())
		require.NoError(t, NewGenerator(cfg).Generate(""))
	}
	write("vendor/a/skills/pdf/SKILL.md", "---\ndescription: Pdf\n---\nBODY\n")
	write("ai-rulez.toml", toml("v-"))
	generate()
	require.FileExists(t, filepath.Join(root, ".claude", "skills", "v-pdf", "SKILL.md"))

	// Act
	write("ai-rulez.toml", toml("w-"))
	generate()

	// Assert
	assert.FileExists(t, filepath.Join(root, ".claude", "skills", "w-pdf", "SKILL.md"))
	assert.NoDirExists(t, filepath.Join(root, ".claude", "skills", "v-pdf"), "the stale directory is pruned")
}
