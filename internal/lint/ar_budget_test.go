package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadBudgetIgnoresOnlyExcludedUnmanagedContent(t *testing.T) {
	for _, kind := range []string{"rule", "skill"} {
		for _, tc := range []struct {
			name                string
			configured, ignored bool
			want                bool
		}{
			{"unmanaged included", false, false, true},
			{"unmanaged ignored", false, true, false},
			{"configured ignored", true, true, true},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				prefix := ".claude/"
				if tc.configured {
					prefix = ".ai-rulez/"
				}
				path := prefix + "rules/large.md"
				body := "# Large\n" + strings.Repeat("content ", 30)
				if kind == "skill" {
					path = prefix + "skills/large/SKILL.md"
					body = "---\nname: large\ndescription: Use when " + strings.Repeat("reviewing code ", 30) + "\n---\nBody.\n"
				}
				cfg := "version = \"5.0\"\nname = \"budget\"\npresets = [\"codex\"]\n[lint]\n"
				if tc.ignored {
					cfg += "ignore_paths = [\"" + path + "\"]\n"
				}
				cfg += "[lint.load_budgets]\ncodex-agents-chain = 100\ncodex-skill-listing = 100\n"
				writeFiles(t, root, map[string]string{".ai-rulez/config.toml": cfg, path: body})
				findings, _ := lintWithTargets(t, root, TargetSpec{Detect: true})
				assert.Equal(t, tc.want, has(findings, CodeLoadBudget, "config.toml", 0), dump(findings))
			})
		}
	}
}

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

func TestCodexSkillListingCountsEachSkillNameOnce(t *testing.T) {
	skill := func(name string) string {
		// name, a space and the description are 100 chars, the whole budget below.
		return "---\nname: " + name + "\ndescription: Use when " + strings.Repeat("x", 100-len(name)-len(" Use when ")) + "\n---\nBody.\n"
	}
	cfg := "version = \"5.0\"\nname = \"budget\"\npresets = [\"codex\"]\n[lint.load_budgets]\ncodex-skill-listing = 100\n"
	tests := []struct {
		name  string
		extra map[string]string
		want  string
	}{
		{"authored only", nil, "the 1 skill names and descriptions add up to 100 of 100 chars"},
		{"identical mirror", map[string]string{"skills/alpha/SKILL.md": skill("alpha")}, "the 1 skill names and descriptions add up to 100 of 100 chars"},
		{"mirror and plugin copies", map[string]string{
			"skills/alpha/SKILL.md":                  skill("alpha"),
			"plugin/.ai-rulez/skills/alpha/SKILL.md": skill("alpha"),
			"plugin/.claude/skills/alpha/SKILL.md":   skill("alpha"),
		}, "the 1 skill names and descriptions add up to 100 of 100 chars"},
		{"another skill counts", map[string]string{".agents/skills/gamma/SKILL.md": skill("gamma")}, "the 2 skill names and descriptions add up to 200 chars"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{".ai-rulez/config.toml": cfg, ".ai-rulez/skills/alpha/SKILL.md": skill("alpha")}
			for k, v := range tt.extra {
				files[k] = v
			}
			writeFiles(t, root, files)
			findings, _ := lintWithTargets(t, root, TargetSpec{Detect: true})
			var got []string
			for _, f := range findings {
				if f.Code == CodeLoadBudget {
					got = append(got, f.Message)
				}
			}
			if assert.Len(t, got, 1, dump(findings)) {
				assert.Contains(t, got[0], tt.want)
			}
		})
	}
}
