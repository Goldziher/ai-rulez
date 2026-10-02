package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Input is a project-relative path. Both "/" and "\" separators and a leading
// "./" are accepted.
func TestRulesDirRemainder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		rest string
		ok   bool
	}{
		{"claude file", ".claude/rules/x.md", "x.md", true},
		{"cursor file", ".cursor/rules/x.mdc", "x.mdc", true},
		{"windsurf file", ".windsurf/rules/x.md", "x.md", true},
		{"cline file", ".clinerules/x.md", "x.md", true},
		{"continue file", ".continue/rules/x.md", "x.md", true},
		{"agents file", ".agents/rules/x.md", "x.md", true},
		{"junie file", ".junie/rules/x.md", "x.md", true},
		{"copilot file", ".github/instructions/x.instructions.md", "x.instructions.md", true},
		{"nested file", ".claude/rules/sub/x.md", "sub/x.md", true},
		{"dir itself", ".claude/rules", "", true},
		{"dir itself trailing slash", ".claude/rules/", "", true},
		{"nested scope file", "apps/web/.claude/rules/x.md", "x.md", true},
		{"nested scope dir", "apps/web/.claude/rules", "", true},
		{"dot slash prefix", "./.claude/rules/x.md", "x.md", true},
		{"windows separators", `.claude\rules\x.md`, "x.md", true},
		{"windows nested scope", `apps\web\.claude\rules\x.md`, "x.md", true},
		{"claude skills", ".claude/skills/x/SKILL.md", "", false},
		{"claude root file", ".claude/settings.json", "", false},
		{"similar prefix", ".claude/rules-old/x.md", "", false},
		{"similar segment", "my.claude/rules/x.md", "", false},
		{"plain path", "docs/rules/x.md", "", false},
		{"github agents", ".github/agents/a.md", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rest, ok := RulesDirRemainder(tt.path)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.rest, rest)
			assert.Equal(t, tt.ok && tt.rest != "", InRulesDir(tt.path))
		})
	}
}
