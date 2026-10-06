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
		{"devin file", ".devin/rules/x.md", "x.md", true},
		{"cline file", ".clinerules/x.md", "x.md", true},
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

func TestRulesDirSet_Add(t *testing.T) {
	tests := []struct {
		name       string
		register   string
		path       string
		wantInDir  bool
		wantRemain string
	}{
		{"file in registered dir", "custom-w3/rules", "custom-w3/rules/x.md", true, "x.md"},
		{"nested under a subproject", "custom-w3/rules/", "apps/web/custom-w3/rules/x.md", true, "x.md"},
		{"backslash and dot-slash forms", ".\\custom-w3b\\rules", "custom-w3b/rules/y.md", true, "y.md"},
		{"sibling prefix is not the folder", "custom-w3c/rules", "custom-w3c/rules-extra/x.md", false, ""},
		{"empty registration is ignored", "", "x.md", false, ""},
		{"dot registration is ignored", ".", "x.md", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := &RulesDirSet{}
			set.Add(tt.register)
			set.Add(tt.register) // idempotent

			rest, ok := set.Remainder(tt.path)

			assert.Equal(t, tt.wantInDir, ok && rest != "")
			assert.Equal(t, tt.wantRemain, rest)
		})
	}
}

func TestRulesDirSet_IsPerProject(t *testing.T) {
	// Arrange: one project registers a custom folder, another does not.
	a, b := &Config{}, &Config{}
	a.AddRulesDir("custom-a/rules")

	// Assert: the folder belongs to the first project only, and to no global state.
	if !a.InRulesDir("custom-a/rules/x.md") {
		t.Error("the project that added the folder must see it")
	}
	if b.InRulesDir("custom-a/rules/x.md") || InRulesDir("custom-a/rules/x.md") {
		t.Error("another project, and the built-in set, must not see it")
	}
	if !b.InRulesDir(".claude/rules/x.md") {
		t.Error("the built-in folders apply to every project")
	}
}

func TestRulesDirSet_IncludesTheFoldersOfEmbeddedSpecs(t *testing.T) {
	// the aiassistant spec writes split rule files into .aiassistant/rules
	if !InRulesDir(".aiassistant/rules/x.md") {
		t.Error("a split rules folder of an embedded spec is a rules folder without any registration")
	}
}
