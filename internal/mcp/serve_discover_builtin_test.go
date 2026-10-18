package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const builtinTestingConfig = "version = \"5.0\"\nname = \"p\"\ngitignore = false\npresets = [\"claude\"]\nbuiltins = [\"testing\"]\n"

const repoTestingSkill = "---\nname: testing-conventions\ndescription: Use when writing tests in this repository only\n---\n\n# project testing conventions\n"

// A repository skill beats a builtin of the same name, the way a skill under
// .ai-rulez/skills does; an ai-rulez-defined skill still beats the repository one.
func TestDiscoverSkills_RepositorySkillBeatsBuiltin(t *testing.T) {
	tests := []struct {
		name          string
		includeStatic bool
		ownSkill      bool
		wantBody      string
		wantSource    string
		wantShadowed  string
	}{
		{name: "builtin served", includeStatic: true, wantBody: "# project testing conventions", wantSource: ".claude/skills/testing-conventions"},
		{name: "builtin only static", wantBody: "# project testing conventions", wantSource: ".claude/skills/testing-conventions"},
		{name: "ai-rulez skill still wins", includeStatic: true, ownSkill: true, wantBody: "# own testing conventions", wantSource: "skills/testing-conventions", wantShadowed: "ai-rulez"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{}
			if tt.ownSkill {
				files["skills/testing-conventions/SKILL.md"] = "---\nname: testing-conventions\ndescription: Use when writing tests, the ai-rulez copy\n---\n\n# own testing conventions\n"
			}
			root := project(t, builtinTestingConfig, files)
			writeFile(t, root, ".claude/skills/testing-conventions/SKILL.md", repoTestingSkill)
			log := &testutil.LogRecorder{}

			// Act
			srv := newServerFor(t, &ServeSetup{WorkDir: root, DiscoverSkills: "repo", IncludeStatic: tt.includeStatic, Log: log})

			// Assert
			got, ok := srv.Catalog().Lookup("testing-conventions")
			require.True(t, ok)
			assert.Contains(t, string(got.Files[0].Content), tt.wantBody)
			assert.Contains(t, got.Source, tt.wantSource)
			count := 0
			for _, s := range srv.Catalog().Skills() {
				if s.Name == "testing-conventions" {
					count++
				}
			}
			assert.Equal(t, 1, count, "the name is served once")
			report := srv.Catalog().Discovery()
			require.NotNil(t, report)
			if tt.wantShadowed == "" {
				assert.Empty(t, report.Shadowed)
				assert.NotContains(t, log.String(), "is shadowed")
				return
			}
			require.Len(t, report.Shadowed, 1)
			assert.Contains(t, report.Shadowed[0].ShadowedBy, tt.wantShadowed)
		})
	}
}
