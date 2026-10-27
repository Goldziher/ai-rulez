package lint

import (
	"strings"
	"testing"
)

const fallbackBudgetConfig = "\n[rules]\nscoped_fallback = \"skill\"\n[lint.load_budgets]\ncodex-skill-listing = 150\n"

func scopedRuleDoc(extraFront string) string {
	return "---\nglobs: [\"src/**/*.go\"]\n" + extraFront + "---\n# Go\nUse small functions.\n"
}

func TestLoadBudgetCountsScopedFallbackSkills(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{
			name: "fallback skills past the limit warn", presets: `"codex"`, config: fallbackBudgetConfig,
			files: map[string]string{".ai-rulez/rules/go.md": scopedRuleDoc(""), ".ai-rulez/rules/ts.md": strings.Replace(scopedRuleDoc(""), "src/**/*.go", "web/**/*.ts", 1)},
			want:  []string{"AR964:config.toml:0"}, sev: map[string]Severity{"AR964": SeverityWarning},
		},
		{
			name: "inline fallback adds no skills", presets: `"codex"`,
			config: "\n[lint.load_budgets]\ncodex-skill-listing = 150\n",
			files:  map[string]string{".ai-rulez/rules/go.md": scopedRuleDoc("")}, absent: []string{"AR964"},
		},
		{
			name: "opted-out items are not counted", presets: `"codex"`, config: fallbackBudgetConfig,
			files: map[string]string{".ai-rulez/rules/go.md": scopedRuleDoc("fallback: false\n")}, absent: []string{"AR964"},
		},
		{
			name: "grouping shrinks the listing", presets: `"codex"`,
			config: "\n[rules]\nscoped_fallback = \"skill\"\nscoped_fallback_group = \"directory\"\n[lint.load_budgets]\ncodex-skill-listing = 250\n",
			files: map[string]string{
				".ai-rulez/rules/a.md": scopedRuleDoc(""),
				".ai-rulez/rules/b.md": scopedRuleDoc(""),
				".ai-rulez/rules/c.md": scopedRuleDoc(""),
			},
			absent: []string{"AR964"},
		},
	})
}

func TestLoadBudgetMessageSeparatesScopedFallbackSkills(t *testing.T) {
	msg := budgetMessages(t, ruleCase{
		presets: `"codex"`, config: fallbackBudgetConfig,
		files: map[string]string{".ai-rulez/rules/go.md": scopedRuleDoc(""), ".ai-rulez/rules/ts.md": scopedRuleDoc("")},
	})
	if !strings.Contains(msg, "2 of them scoped-fallback skills") {
		t.Errorf("the message should count the fallback skills separately: %q", msg)
	}
}

func TestFallbackFrontmatterMustBeABoolean(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "string value", files: map[string]string{".ai-rulez/rules/go.md": scopedRuleDoc("fallback: maybe\n")}, want: []string{"AR304:go.md:0"}},
		{name: "boolean value", files: map[string]string{".ai-rulez/rules/go.md": scopedRuleDoc("fallback: false\n")}, absent: []string{"AR304", "AR303"}},
	})
}
