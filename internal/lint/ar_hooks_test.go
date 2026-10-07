package lint

import "testing"

func TestHookSchemaAR507(t *testing.T) {
	settings := func(hooks string) map[string]string {
		return map[string]string{".claude/settings.json": `{"hooks":` + hooks + `}`}
	}
	runRuleCases(t, []ruleCase{
		{
			name:  "unknown event with hint",
			files: settings(`{"PreToolUs":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:   "valid hooks",
			files:  settings(`{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo","timeout":10,"if":"Bash(git *)"}]}],"Stop":[{"hooks":[{"type":"command","command":"echo"}]}]}`),
			absent: []string{"AR507"},
		},
		{
			name:  "missing type",
			files: settings(`{"Stop":[{"hooks":[{"command":"echo"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "command handler without command",
			files: settings(`{"Stop":[{"hooks":[{"type":"command"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "http handler needs a url",
			files: settings(`{"Stop":[{"hooks":[{"type":"http","command":"x"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "unknown handler type",
			files: settings(`{"Stop":[{"hooks":[{"type":"shell","command":"x"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "timeout as a string",
			files: settings(`{"Stop":[{"hooks":[{"type":"command","command":"x","timeout":"10"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "negative timeout",
			files: settings(`{"Stop":[{"hooks":[{"type":"command","command":"x","timeout":-5}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "matcher on an event that ignores it",
			files: settings(`{"UserPromptSubmit":[{"matcher":"Bash","hooks":[{"type":"command","command":"x"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "if on an event that never evaluates it",
			files: settings(`{"SessionStart":[{"hooks":[{"type":"command","command":"x","if":"Bash(git *)"}]}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "group without a hooks list",
			files: settings(`{"PreToolUse":[{"matcher":"Bash"}]}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:  "events must map to lists",
			files: settings(`{"PreToolUse":{"matcher":"Bash"}}`),
			want:  []string{"AR507:settings.json:1"},
		},
		{
			name:   "no hooks key at all",
			files:  map[string]string{".claude/settings.json": `{"permissions":{"allow":["Read"]}}`},
			absent: []string{"AR507"},
		},
		{
			name:  "plugin hooks.json is checked",
			files: map[string]string{"plugins/p/hooks/hooks.json": `{"hooks":{"SesionStart":[{"hooks":[{"type":"command","command":"x"}]}]}}`},
			want:  []string{"AR507:hooks.json:1"},
		},
		{
			name: "config.toml hooks",
			config: "\n[[hooks]]\nevent = \"Stopp\"\n[[hooks.hooks]]\ncommand = \"echo\"\n\n" +
				"[[hooks]]\nevent = \"SessionStart\"\nmatcher = \"startup\"\n[[hooks.hooks]]\ncommand = \"echo\"\nif = \"Bash(git *)\"\ntimeout = -1\n",
			want: []string{"AR507:config.toml:0"},
		},
		{
			name:   "config.toml hooks that are fine",
			config: "\n[[hooks]]\nevent = \"PreToolUse\"\nmatcher = \"Bash\"\n[[hooks.hooks]]\ncommand = \"echo\"\ntimeout = 10\nif = \"Bash(git *)\"\n",
			absent: []string{"AR507"},
		},
		{
			name:   "config.toml wildcard matcher means every subject, like the JSON form",
			config: "\n[[hooks]]\nevent = \"Stop\"\nmatcher = \"*\"\n[[hooks.hooks]]\ncommand = \"echo\"\n",
			absent: []string{"AR507"},
		},
		{
			name:  "json wildcard matcher",
			files: settings(`{"Stop":[{"matcher":"*","hooks":[{"type":"command","command":"x"}]}]}`), absent: []string{"AR507"},
		},
		{
			name:   "severity off",
			files:  settings(`{"Nope":[]}`),
			config: "\n[lint.severity]\nhook-schema-invalid = \"off\"\n",
			absent: []string{"AR507"},
		},
	})
}
