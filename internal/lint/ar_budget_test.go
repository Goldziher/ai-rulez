package lint

import (
	"strings"
	"testing"
)

func TestLoadBudgetsAR964(t *testing.T) {
	long := strings.Repeat("word ", 400) // 2000 chars
	big := "# Big\n" + strings.Repeat("x", 13000) + "\n"
	lines := "# Lines\n" + strings.Repeat("line\n", 600)
	chain := "# A\n" + strings.Repeat("y", 20000) + "\n"
	runRuleCases(t, []ruleCase{
		{name: "claude listing cap", presets: `"claude"`, skill: "---\nname: bad\ndescription: Use when " + long + "\n---\nx\n", want: []string{"AR964:SKILL.md:3"}},
		{name: "claude listing cap counts when_to_use", presets: `"claude"`, skill: "---\nname: bad\ndescription: Use when testing things.\nwhen_to_use: " + long + "\n---\nx\n", want: []string{"AR964:SKILL.md:0"}},
		{name: "short description is fine", presets: `"claude"`, skill: skillDoc("", "x\n"), absent: []string{"AR964"}},
		{name: "cap not applied without the claude preset", presets: `"cursor"`, skill: "---\nname: bad\ndescription: Use when " + long + "\n---\nx\n", absent: []string{"AR964"}},
		{name: "windsurf/devin rule file", presets: `"devin"`, files: map[string]string{".ai-rulez/rules/big.md": big}, want: []string{"AR964:big.md:1"}},
		{name: "big rule is fine for claude", presets: `"claude"`, files: map[string]string{".ai-rulez/rules/big.md": big}, absent: []string{"AR964"}},
		{name: "cursor rule length", presets: `"cursor"`, config: "\n[lint.budgets.rule]\nmax_lines = -1\nmax_tokens = -1\n", files: map[string]string{".ai-rulez/rules/lines.md": lines}, want: []string{"AR964:lines.md:1"}},
		{
			name: "codex chain", presets: `"codex"`, config: "\n[lint.budgets.rule]\nmax_lines = -1\nmax_tokens = -1\n",
			files: map[string]string{".ai-rulez/rules/a.md": chain, ".ai-rulez/rules/b.md": chain}, want: []string{"AR964:config.toml:0"}, sev: map[string]Severity{"AR964": SeverityWarning},
		},
		{
			name: "codex chain near the limit is info", presets: `"codex"`, config: "\n[lint.budgets.rule]\nmax_lines = -1\nmax_tokens = -1\n",
			files: map[string]string{".ai-rulez/rules/a.md": chain, ".ai-rulez/rules/b.md": "# B\n" + strings.Repeat("y", 9500) + "\n"}, want: []string{"AR964:config.toml:0"}, sev: map[string]Severity{"AR964": SeverityInfo},
		},
		{name: "codex chain small", presets: `"codex"`, files: map[string]string{".ai-rulez/rules/a.md": "# A\nshort\n"}, absent: []string{"AR964"}},
		{name: "off", presets: `"devin"`, config: "\n[lint.severity]\nAR964 = \"off\"\n", files: map[string]string{".ai-rulez/rules/big.md": big}, absent: []string{"AR964"}},
	})
}

func TestLoadBudgetTableIsComplete(t *testing.T) {
	for _, b := range loadBudgets {
		if b.ID == "" || b.Limit <= 0 || !strings.HasPrefix(b.Source, "https://") || b.Checked == "" || b.Unit == "" {
			t.Errorf("incomplete load budget %+v", b)
		}
	}
}

func budgetMessages(t *testing.T, c ruleCase) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, c.project())
	gitAdd(t, root)
	var all []string
	for _, f := range lintDir(t, root) {
		if f.Code == CodeLoadBudget {
			all = append(all, f.Message)
		}
	}
	return strings.Join(all, "\n")
}

func TestLoadBudgetMessagesNameTheHarnessAndMarkUnverifiedFigures(t *testing.T) {
	big := "# Big\n" + strings.Repeat("x", 13000) + "\n"
	if msg := budgetMessages(t, ruleCase{presets: `"devin"`, files: map[string]string{".ai-rulez/rules/big.md": big}}); !strings.Contains(msg, "Windsurf") {
		t.Errorf("the devin-preset rule-file budget should name Windsurf: %q", msg)
	}
	skill := "---\nname: bad\ndescription: Use when " + strings.Repeat("abcdefgh ", 1000) + "\n---\nx\n"
	if msg := budgetMessages(t, ruleCase{presets: `"codex"`, skill: skill}); !strings.Contains(msg, "unverified") {
		t.Errorf("an unverified figure should say so: %q", msg)
	}
}
