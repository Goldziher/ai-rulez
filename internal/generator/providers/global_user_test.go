package providers_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
)

func TestProjectLayout(t *testing.T) {
	t.Parallel()

	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	layout := gen.ProjectLayout()
	assert.Equal(t, "CLAUDE.md", layout.RootFile)
	assert.Equal(t, ".claude/skills", layout.SkillsDir)
	assert.Equal(t, ".claude/agents", layout.AgentsDir)
	assert.Equal(t, ".claude/rules", layout.RulesDir)
	assert.Equal(t, ".claude/skills", layout.CommandsDir)
}

// TestUserOnlySkills: Hermes keeps skills in a user-level store and has no project
// folder for them, so only a user-scope run renders them, at the store's path.
func TestUserOnlySkills(t *testing.T) {
	t.Parallel()

	gen, err := providers.LoadBuiltin("hermes")
	require.NoError(t, err)
	assert.Equal(t, ".hermes/skills", gen.ProjectLayout().SkillsDir)

	content := &config.ContentTree{Skills: []config.ContentFile{{
		Name: "triage", Path: "/c/skills/triage/SKILL.md", Content: "Triage body.",
		Metadata: &config.Metadata{Extra: map[string]string{"description": "Triage issues. Use when asked."}},
	}}}
	tests := []struct {
		name      string
		userScope bool
		want      bool
	}{
		{"a project run writes no skills", false, false},
		{"a user-scope run writes the store", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputs, err := gen.Generate(content, "/base", &config.Config{Name: "me", BaseDir: "/base", UserScope: tt.userScope})
			require.NoError(t, err)
			assert.Equal(t, tt.want, hasOutputPathSuffix(outputs, ".hermes/skills/triage/SKILL.md"))
		})
	}
}

func TestGlobalSkillReaders(t *testing.T) {
	t.Parallel()

	spec := loadSpec(t, `
name = "reader"
[global]
skills_dir = ".r/skills"
skill_readers = [".r/skills", ".agents/skills"]
skill_precedence = "workspace wins"
`).Spec
	got := spec.GlobalPaths(filepath.FromSlash("/home/me"), func(string) string { return "" })
	require.NotNil(t, got)
	assert.Equal(t, []string{filepath.FromSlash("/home/me/.r/skills"), filepath.FromSlash("/home/me/.agents/skills")}, got.SkillReaders)
	assert.Equal(t, "workspace wins", got.SkillPrecedence)

	_, err := providers.LoadProviderSpec([]byte("name = \"bad\"\n[global]\nskills_dir = \"s\"\nskill_readers = [\"../x\"]\n"), "spec.toml", providers.FormatAuto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill_readers")
}
