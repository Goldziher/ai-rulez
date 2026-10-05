package lint

import "testing"

func TestImportsAR210(t *testing.T) {
	chain := map[string]string{}
	for i := 1; i <= 6; i++ {
		next := ""
		if i < 6 {
			next = "@c" + string(rune('0'+i+1)) + ".md\n"
		}
		chain[".ai-rulez/context/c"+string(rune('0'+i))+".md"] = "# C\n" + next
	}
	chain[".ai-rulez/rules/start.md"] = "# Start\nSee @../context/c1.md for more.\n"
	// c1 is next to the rule's sibling directory: imports in c-files resolve beside them.
	runRuleCases(t, []ruleCase{
		{name: "missing import", files: map[string]string{".ai-rulez/rules/a.md": "# A\nUse @docs/missing.md here.\n"}, want: []string{"AR210:a.md:2"}},
		{name: "existing import", files: map[string]string{".ai-rulez/rules/a.md": "# A\nUse @docs/git.md here.\n", "docs/git.md": "# git\n"}, absent: []string{"AR210"}},
		{name: "relative import", files: map[string]string{".ai-rulez/rules/a.md": "# A\n@./b.md\n", ".ai-rulez/rules/b.md": "# B\n"}, absent: []string{"AR210"}},
		{name: "circular", files: map[string]string{".ai-rulez/rules/a.md": "# A\n@./b.md\n", ".ai-rulez/rules/b.md": "# B\n@./a.md\n"}, want: []string{"AR210:a.md:2", "AR210:b.md:2"}},
		{name: "chain of six hops", files: chain, want: []string{"AR210:start.md:2"}},
		{
			name: "chain of five hops is loaded",
			files: map[string]string{
				".ai-rulez/rules/a.md": "# A\n@./b.md\n", ".ai-rulez/rules/b.md": "@./c.md\n", ".ai-rulez/rules/c.md": "@./d.md\n",
				".ai-rulez/rules/d.md": "@./e.md\n", ".ai-rulez/rules/e.md": "@./f.md\n", ".ai-rulez/rules/f.md": "# end\n",
			},
			absent: []string{"AR210"},
		},
		{
			name: "false positive traps",
			files: map[string]string{".ai-rulez/rules/a.md": "# A\nPing @alice and mail bob@example.com.\nInstall `@types/node` and @angular/core.\n" +
				"```\n@docs/in-a-fence.md\n```\nUse @~/.claude/me.md or @/etc/thing.md.\nThe @decorator and @user.name handle.\n"},
			absent: []string{"AR210"},
		},
		{name: "inline ignore", files: map[string]string{".ai-rulez/rules/a.md": "# A\n<!-- ai-rulez-lint-ignore: AR210 -->\n@docs/missing.md\n"}, absent: []string{"AR210"}},
		{name: "severity override", config: "\n[lint.severity]\nAR210 = \"warning\"\n", files: map[string]string{".ai-rulez/rules/a.md": "# A\n@docs/missing.md\n"}, want: []string{"AR210:a.md:2"}, sev: map[string]Severity{"AR210": SeverityWarning}},
	})
}
