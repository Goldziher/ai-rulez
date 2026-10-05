package lint

import "testing"

func TestAutoInvocationAR013(t *testing.T) {
	script := map[string]string{".ai-rulez/skills/bad/scripts/run.sh": "#!/bin/sh\necho hi\n"}
	runRuleCases(t, []ruleCase{
		{name: "broad bash with scripts", files: script, skill: skillDoc("allowed-tools: Bash, Read\n", "x\n"), want: []string{"AR013:SKILL.md:4"}},
		{name: "bash star with scripts", files: script, skill: skillDoc("allowed-tools: \"Bash(*)\"\n", "x\n"), want: []string{"AR013:SKILL.md:4"}},
		{name: "model invocation disabled", files: script, skill: skillDoc("allowed-tools: Bash\ndisable-model-invocation: true\n", "x\n"), absent: []string{"AR013"}},
		{name: "narrow bash", files: script, skill: skillDoc("allowed-tools: Bash(git status:*)\n", "x\n"), absent: []string{"AR013"}},
		{name: "broad bash without scripts", skill: skillDoc("allowed-tools: Bash\n", "x\n"), absent: []string{"AR013"}},
		{name: "scripts without allowed-tools", files: script, skill: skillDoc("", "x\n"), absent: []string{"AR013"}},
		{name: "allowed_tools exemption", files: script, config: "\n[lint.security]\nallowed_tools = [\"Bash\"]\n", skill: skillDoc("allowed-tools: Bash\n", "x\n"), absent: []string{"AR013"}},
		{
			name:  "subagent bypassPermissions",
			files: map[string]string{".ai-rulez/agents/rev.md": "---\ndescription: Reviews code changes for the billing team carefully.\npermissionMode: bypassPermissions\n---\nbody\n"},
			want:  []string{"AR013:rev.md:3"},
		},
		{
			name:   "subagent acceptEdits",
			files:  map[string]string{".ai-rulez/agents/rev.md": "---\ndescription: Reviews code changes for the billing team carefully.\npermissionMode: acceptEdits\n---\nbody\n"},
			absent: []string{"AR013"},
		},
		{
			name:   "prose about bypassPermissions is fine",
			skill:  skillDoc("", "Never set permissionMode: bypassPermissions.\n"),
			absent: []string{"AR013"},
		},
	})
}
