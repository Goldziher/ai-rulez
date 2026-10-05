package lint

import "testing"

func TestFrontmatterValueAR304(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "effort typo", skill: skillDoc("effort: hgih\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "effort ok", skill: skillDoc("effort: xhigh\n", "x\n"), absent: []string{"AR304"}},
		{name: "context must be fork", skill: skillDoc("context: subagent\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "context fork ok", skill: skillDoc("context: fork\n", "x\n"), absent: []string{"AR304"}},
		{name: "shell", skill: skillDoc("shell: zsh\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "quoted boolean", skill: skillDoc("disable-model-invocation: \"true\"\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "yes is not a boolean", skill: skillDoc("user-invocable: yes\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "real boolean", skill: skillDoc("disable-model-invocation: true\nuser-invocable: false\n", "x\n"), absent: []string{"AR304"}},
		{name: "model typo gets a hint", skill: skillDoc("model: sonet\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "model aliases and ids", skill: skillDoc("model: claude-sonnet-4-5\n", "x\n"), absent: []string{"AR304"}},
		{name: "model from another vendor passes", skill: skillDoc("model: gpt-5.1-codex\n", "x\n"), absent: []string{"AR304"}},
		{name: "model with the 1m suffix", skill: skillDoc("model: opus[1m]\n", "x\n"), absent: []string{"AR304"}},
		{name: "paths as a mapping", skill: skillDoc("paths:\n  a: b\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "paths with a number entry", skill: skillDoc("paths:\n  - src/**\n  - 3\n", "x\n"), want: []string{"AR304:SKILL.md:4"}},
		{name: "paths list ok", skill: skillDoc("paths:\n  - \"src/**\"\n", "x\n"), absent: []string{"AR304"}},
		{name: "paths comma string ok", skill: skillDoc("paths: \"src/**, lib/**\"\n", "x\n"), absent: []string{"AR304"}},
		{name: "description as a number", skill: "---\nname: bad\ndescription: 12345\n---\nx\n", want: []string{"AR304:SKILL.md:3"}},
		{
			name: "agent enums and ints",
			files: map[string]string{".ai-rulez/agents/rev.md": "---\ndescription: Reviews code changes for the billing team carefully.\n" +
				"permissionMode: bypass\nmemory: global\nmaxTurns: lots\nbackground: \"no\"\ncolor: teal\nisolation: branch\n---\nbody\n"},
			want: []string{"AR304:rev.md:3", "AR304:rev.md:4", "AR304:rev.md:5", "AR304:rev.md:6", "AR304:rev.md:7", "AR304:rev.md:8"},
		},
		{
			name: "agent valid values",
			files: map[string]string{".ai-rulez/agents/rev.md": "---\ndescription: Reviews code changes for the billing team carefully.\n" +
				"permissionMode: acceptEdits\nmemory: project\nmaxTurns: 12\nbackground: true\ncolor: cyan\n---\nbody\n"},
			absent: []string{"AR304"},
		},
		{
			name:   "fence and prose that show the key are not frontmatter",
			skill:  skillDoc("", "```yaml\neffort: nonsense\n```\nSet `effort: nonsense` in frontmatter.\n"),
			absent: []string{"AR304"},
		},
		{
			name:   "inline ignore",
			skill:  skillDoc("# ai-rulez-lint-ignore: AR304\neffort: nonsense\n", "x\n"),
			absent: []string{"AR304"},
		},
		{
			name: "severity override to error", skill: skillDoc("effort: nope\n", "x\n"),
			config: "\n[lint.severity]\nAR304 = \"error\"\n", want: []string{"AR304:SKILL.md:4"}, sev: map[string]Severity{"AR304": SeverityError},
		},
		{
			name: "severity off", skill: skillDoc("effort: nope\n", "x\n"),
			config: "\n[lint.severity]\nfrontmatter-value-invalid = \"off\"\n", absent: []string{"AR304"},
		},
	})
}

func TestToolNamesAR305(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "unknown tool with hint", skill: skillDoc("allowed-tools: Read, Bsh\n", "x\n"), want: []string{"AR305:SKILL.md:4"}},
		{name: "wrong case", skill: skillDoc("allowed-tools: bash, Read\n", "x\n"), want: []string{"AR305:SKILL.md:4"}},
		{name: "known tools and bash patterns", skill: skillDoc("allowed-tools: Read Grep Bash(git add:*) Bash(npm run test:*) WebFetch(domain:example.com)\n", "x\n"), absent: []string{"AR305"}},
		{name: "mcp tools are valid", skill: skillDoc("allowed-tools:\n  - mcp__github__create_issue\n  - mcp__github\n  - mcp__plugin_slack_slack__slack_send_message\n  - \"mcp__github__*\"\n", "x\n"), absent: []string{"AR305"}},
		{name: "malformed mcp name", skill: skillDoc("allowed-tools:\n  - mcp__\n", "x\n"), want: []string{"AR305:SKILL.md:4"}},
		{name: "single underscore mcp", skill: skillDoc("allowed-tools: mcp_github_create\n", "x\n"), want: []string{"AR305:SKILL.md:4"}},
		{name: "allowed and disallowed", skill: skillDoc("allowed-tools: Read Write\ndisallowed-tools: Write\n", "x\n"), want: []string{"AR305:SKILL.md:5"}},
		{name: "narrow allow with broad deny is fine", skill: skillDoc("allowed-tools: Bash(git status)\ndisallowed-tools: Bash(rm:*)\n", "x\n"), absent: []string{"AR305"}},
		{name: "unbalanced parentheses", skill: skillDoc("allowed-tools: Bash(git add:*\n", "x\n"), want: []string{"AR305:SKILL.md:4"}},
		{
			name: "agent tools", files: map[string]string{".ai-rulez/agents/rev.md": "---\ndescription: Reviews code changes for the billing team carefully.\ntools: Read, Grep, Agent(worker), Taks\ndisallowedTools: Grep\n---\nbody\n"},
			want: []string{"AR305:rev.md:3", "AR305:rev.md:4"},
		},
		{name: "known_names whitelists a tool", config: "\n[lint]\nknown_names = [\"CustomTool\"]\n", skill: skillDoc("allowed-tools: CustomTool\n", "x\n"), absent: []string{"AR305"}},
		{name: "tool names are checked for the claude preset", config: "", skill: skillDoc("allowed-tools: run_shell_command\n", "x\n"), want: []string{"AR305:SKILL.md:4"}},
	})
}
