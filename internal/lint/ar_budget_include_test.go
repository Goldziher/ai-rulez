package lint

import (
	"strings"
	"testing"
)

const includeConfig = "\n[[includes]]\nname = \"shared\"\nsource = \"./shared\"\ninclude = [\"rules\", \"skills\"]\n"

func includedSkill(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\nBody.\n"
}

func TestLoadBudgetCountsIncludedContent(t *testing.T) {
	long := "Use when " + strings.Repeat("reviewing code ", 30)
	chain := "# Big\n" + strings.Repeat("x", 20000) + "\n"
	runRuleCases(t, []ruleCase{
		{
			name: "included skills push the codex listing past the limit", presets: `"codex"`,
			config: includeConfig + "[lint.load_budgets]\ncodex-skill-listing = 100\n",
			files:  map[string]string{"shared/skills/ext/SKILL.md": includedSkill("ext", long)},
			want:   []string{"AR964:config.toml:0"}, sev: map[string]Severity{"AR964": SeverityWarning},
		},
		{
			name: "included skills below the limit stay quiet", presets: `"codex"`,
			config: includeConfig,
			files:  map[string]string{"shared/skills/ext/SKILL.md": includedSkill("ext", "Use when testing.")},
			absent: []string{"AR964"},
		},
		{
			name: "included rules count toward the codex chain", presets: `"codex"`,
			config: includeConfig,
			files:  map[string]string{"shared/rules/a.md": chain, "shared/rules/b.md": chain},
			want:   []string{"AR964:config.toml:0"},
		},
		{
			name: "included skill over the claude listing cap is reported", presets: `"claude"`,
			config: includeConfig + "[lint.load_budgets]\nclaude-skill-listing = 100\n",
			files:  map[string]string{"shared/skills/ext/SKILL.md": includedSkill("ext", long)},
			want:   []string{"AR964:config.toml:0"},
		},
	})
}

func TestLoadBudgetMessageCountsIncludedSkills(t *testing.T) {
	long := "Use when " + strings.Repeat("reviewing code ", 30)
	msg := budgetMessages(t, ruleCase{
		presets: `"codex"`, config: includeConfig + "[lint.load_budgets]\ncodex-skill-listing = 100\n",
		skill: includedSkill("bad", "Use when authoring."),
		files: map[string]string{"shared/skills/ext/SKILL.md": includedSkill("ext", long)},
	})
	if !strings.Contains(msg, "2 skill names") || !strings.Contains(msg, "1 of them from includes") {
		t.Errorf("the message should count both skills and say one is from includes: %q", msg)
	}
}
