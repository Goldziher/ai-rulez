package config

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claudeConfig(s *SessionStartConfig, hooks ...HookGroup) *Config {
	return &Config{Version: "5.0", Name: "t", Presets: []Preset{{BuiltIn: "claude"}}, SessionStart: s, Hooks: hooks}
}

func TestSessionStartLaunchers(t *testing.T) {
	tests := []struct {
		name    string
		session *SessionStartConfig
		want    []string
	}{
		{name: "latest published release in order", want: []string{
			"ai-rulez session-start",
			"npx -y ai-rulez@latest session-start",
			"uvx --from ai-rulez@latest ai-rulez session-start",
		}},
		{name: "configured version pins both", session: &SessionStartConfig{Version: "5.0.0"}, want: []string{
			"ai-rulez session-start", "npx -y ai-rulez@5.0.0 session-start", "uvx --from ai-rulez==5.0.0 ai-rulez session-start",
		}},
		{name: "command is the only launcher", session: &SessionStartConfig{Command: []string{"/opt/my tools/ai-rulez"}},
			want: []string{"'/opt/my tools/ai-rulez' session-start"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := claudeConfig(tt.session)

			var got []string
			for _, l := range cfg.SessionStartLaunchers() {
				got = append(got, l.Command)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSessionStartCommand(t *testing.T) {
	t.Run("remote gate comes first and only for when remote", func(t *testing.T) {
		remote := claudeConfig(nil).SessionStartCommand()
		always := claudeConfig(&SessionStartConfig{When: SessionStartWhenAlways}).SessionStartCommand()

		assert.True(t, strings.HasPrefix(remote, "sh -c 'case \"${CLAUDE_CODE_REMOTE:-}\" in"), remote)
		assert.NotContains(t, always, "CLAUDE_CODE_REMOTE")
		assert.NotContains(t, remote, "''", "no nested single quotes")
		assert.Equal(t, 2, strings.Count(remote, "'"), "the script is one single-quoted word")
	})
	t.Run("order is path then npx then uvx and ends with exit 0", func(t *testing.T) {
		cmd := claudeConfig(nil).SessionStartCommand()

		path, npx, uvx := strings.Index(cmd, "ai-rulez session-start;"), strings.Index(cmd, "npx -y"), strings.Index(cmd, "uvx --from")
		assert.Less(t, path, npx)
		assert.Less(t, npx, uvx)
		assert.True(t, strings.HasSuffix(cmd, "exit 0'"))
	})
	t.Run("npx and uvx run from a neutral directory, the installed binary does not", func(t *testing.T) {
		cmd := claudeConfig(nil).SessionStartCommand()

		assert.Contains(t, cmd, `export CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$PWD}"`)
		assert.Contains(t, cmd, "mktemp -d")
		assert.Contains(t, cmd, "&& n npx -y ai-rulez@latest session-start;")
		assert.Contains(t, cmd, "&& n uvx --from ai-rulez@latest ai-rulez session-start;")
		assert.Contains(t, cmd, "&& ai-rulez session-start;")
		assert.NotContains(t, cmd, "&& n ai-rulez")
	})
	t.Run("a pinned version replaces latest", func(t *testing.T) {
		cmd := claudeConfig(&SessionStartConfig{Version: "5.4.1"}).SessionStartCommand()

		assert.Contains(t, cmd, "npx -y ai-rulez@5.4.1 session-start")
		assert.Contains(t, cmd, "uvx --from ai-rulez==5.4.1 ai-rulez session-start")
		assert.NotContains(t, cmd, "latest")
	})
	t.Run("command override has no shell", func(t *testing.T) {
		cmd := claudeConfig(&SessionStartConfig{Command: []string{"ai-rulez"}}).SessionStartCommand()

		assert.Equal(t, "ai-rulez session-start", cmd)
	})
}

func TestEnableSessionStartHooks(t *testing.T) {
	t.Run("adds one group, once, for claude only", func(t *testing.T) {
		cfg := claudeConfig(nil, HookGroup{Event: "Stop"})

		cfg.EnableSessionStartHooks()
		cfg.EnableSessionStartHooks()

		require.Len(t, cfg.Hooks, 2)
		g := cfg.Hooks[1]
		assert.Equal(t, "SessionStart", g.Event)
		assert.Equal(t, HookBuiltinSessionStart, g.Builtin)
		assert.Equal(t, []string{"claude"}, g.Targets)
		assert.Equal(t, SessionStartDefaultTimeoutSeconds+sessionStartHookSlackSeconds, g.Hooks[0].Timeout)
		assert.Len(t, cfg.UserHooks(), 1, "the synthesized group is not a user hook")
	})
	t.Run("hook timeout follows the configured timeout", func(t *testing.T) {
		cfg := claudeConfig(&SessionStartConfig{Timeout: 60})

		cfg.EnableSessionStartHooks()

		assert.Equal(t, 60+sessionStartHookSlackSeconds, cfg.Hooks[0].Hooks[0].Timeout)
	})
	user := HookGroup{Event: "SessionStart", Hooks: []HookAction{{Command: "npx -y ai-rulez@5.0.0 session-start"}}}
	other := HookGroup{Event: "SessionStart", Hooks: []HookAction{{Command: "./scripts/setup.sh"}}}
	for name, cfg := range map[string]*Config{
		"generate = false":                      claudeConfig(&SessionStartConfig{Generate: boolPtr(false)}),
		"user scope":                            {Presets: []Preset{{BuiltIn: "claude"}}, UserScope: true},
		"no claude preset":                      {Presets: []Preset{{BuiltIn: "cursor"}}},
		"user declared an equivalent":           claudeConfig(nil, user),
		"user declared via args":                claudeConfig(nil, HookGroup{Event: "SessionStart", Hooks: []HookAction{{Command: "ai-rulez", Args: []string{"session-start"}}}}),
		"user declared on another event is not": nil,
	} {
		if cfg == nil {
			continue
		}
		t.Run("does nothing when "+name, func(t *testing.T) {
			before := len(cfg.Hooks)

			cfg.EnableSessionStartHooks()

			assert.Len(t, cfg.Hooks, before)
		})
	}
	t.Run("an unrelated user SessionStart hook does not suppress it", func(t *testing.T) {
		cfg := claudeConfig(nil, other)

		cfg.EnableSessionStartHooks()

		assert.Len(t, cfg.Hooks, 2)
	})
	t.Run("drop removes only the synthesized group", func(t *testing.T) {
		cfg := claudeConfig(nil, other)
		cfg.EnableSessionStartHooks()

		cfg.DropSessionStartHooks()

		assert.Equal(t, []HookGroup{other}, cfg.Hooks)
	})
}

func TestConfigValidateSessionStart(t *testing.T) {
	tests := []struct {
		name    string
		session *SessionStartConfig
		wantErr string
	}{
		{name: "absent"},
		{name: "defaults", session: &SessionStartConfig{}},
		{name: "everything", session: &SessionStartConfig{When: "always", Mode: "frozen", Timeout: 30, Version: "5.4.1", Prompt: boolPtr(true), PromptText: "see {{server}}"}},
		{name: "command", session: &SessionStartConfig{Command: []string{"ai-rulez"}}},
		{name: "bad when", session: &SessionStartConfig{When: "sometimes"}, wantErr: "invalid session_start.when"},
		{name: "bad mode", session: &SessionStartConfig{Mode: "fast"}, wantErr: "invalid session_start.mode"},
		{name: "negative timeout", session: &SessionStartConfig{Timeout: -1}, wantErr: "out of range"},
		{name: "huge timeout", session: &SessionStartConfig{Timeout: 901}, wantErr: "out of range"},
		{name: "version and command", session: &SessionStartConfig{Version: "1.0.0", Command: []string{"x"}}, wantErr: "mutually exclusive"},
		{name: "empty executable", session: &SessionStartConfig{Command: []string{" "}}, wantErr: "executable must not be empty"},
		{name: "shell metacharacters in version", session: &SessionStartConfig{Version: "1;rm"}, wantErr: "not a valid version"},
		{name: "keys without generate", session: &SessionStartConfig{Generate: boolPtr(false), Mode: "plain"}, wantErr: "generate = false"},
		{name: "generate false alone", session: &SessionStartConfig{Generate: boolPtr(false)}},
		{name: "prompt text with prompt off", session: &SessionStartConfig{Prompt: boolPtr(false), PromptText: "x"}, wantErr: "requires session_start.prompt"},
		{name: "blank prompt text", session: &SessionStartConfig{PromptText: "  "}, wantErr: "must not be blank"},
		{name: "unknown variable", session: &SessionStartConfig{PromptText: "{{nope}}"}, wantErr: "unknown variable {{nope}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := claudeConfig(tt.session)

			err := cfg.Validate()

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSessionStartContext(t *testing.T) {
	serving := func() *Config {
		cfg := claudeConfig(nil)
		cfg.Presets = []Preset{{BuiltIn: "claude"}}
		cfg.ConfigDirName = ".ai-rulez"
		return cfg
	}
	t.Run("default text names the source dir", func(t *testing.T) {
		got := serving().SessionStartContext("")

		assert.Equal(t, "ai-rulez manages this repository's agent configuration. The rules, skills, agents and commands in the harness folders, "+
			"AGENTS.md and CLAUDE.md are generated from the sources in `.ai-rulez/`; a change belongs in the source, then `ai-rulez generate` "+
			"renders it, and an edit to a generated file is overwritten.\n\n"+
			"The `ai-rulez` MCP server offers tools to create, update, validate and generate this configuration.", got)
	})
	t.Run("no MCP sentence when the self server is off", func(t *testing.T) {
		cfg := serving()
		cfg.SetSelfServer(false)

		got := cfg.SessionStartContext("")

		assert.NotContains(t, got, "MCP server")
	})
	t.Run("served skills add the unlisted skills section with the server name", func(t *testing.T) {
		cfg := serving()
		cfg.SkillSources = []SkillSourceConfig{{Name: "vendor", URL: "vendor-skills"}}

		got := cfg.SessionStartContext("")

		assert.Contains(t, got, "## Unlisted skills")
		assert.Contains(t, got, "`find_skill`")
		assert.Contains(t, got, "`load_skill`")
		assert.Contains(t, got, "`ai-rulez` MCP server")
	})
	t.Run("skills_instructions = false drops that section", func(t *testing.T) {
		cfg := serving()
		cfg.SkillSources = []SkillSourceConfig{{Name: "vendor", URL: "vendor-skills"}}
		cfg.MCP = &MCPConfig{SkillsInstructions: boolPtr(false)}

		assert.NotContains(t, cfg.SessionStartContext(""), "Unlisted skills")
	})
	t.Run("prompt_text replaces the body and expands the server", func(t *testing.T) {
		cfg := serving()
		cfg.SessionStart = &SessionStartConfig{PromptText: "Use {{server}}."}

		assert.Equal(t, "Use ai-rulez.", cfg.SessionStartContext(""))
	})
	t.Run("a failure is reported in one line, even with a custom text", func(t *testing.T) {
		cfg := serving()
		cfg.SessionStart = &SessionStartConfig{PromptText: "Use {{server}}."}

		got := cfg.SessionStartContext("exit code 1\x1b[31m\nsecond")

		assert.Equal(t, "Use ai-rulez.\n\nThe `ai-rulez generate` run at session start failed (exit code 1 [31m"+" second), so generated files may be out of date.", got)
	})
	t.Run("the result is capped on a rune boundary", func(t *testing.T) {
		cfg := serving()
		cfg.SessionStart = &SessionStartConfig{PromptText: strings.Repeat("é", 5000)}

		got := cfg.SessionStartContext("")

		assert.LessOrEqual(t, len(got), SessionStartContextLimit)
		assert.True(t, strings.HasSuffix(got, "é"))
	})
	t.Run("deterministic", func(t *testing.T) {
		assert.Equal(t, serving().SessionStartContext("x"), serving().SessionStartContext("x"))
	})
}

func TestLoadConfigTOML_SessionStartSection(t *testing.T) {
	baseDir := writeTOMLProject(t, `version = "5.0"
name = "proj"
presets = ["claude"]

[session_start]
when = "always"
mode = "plain"
timeout = 45
prompt = false
`)
	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.SessionStart)
	assert.Equal(t, SessionStartWhenAlways, cfg.SessionStart.WhenOrDefault())
	assert.Equal(t, SessionStartModePlain, cfg.SessionStart.ModeOrDefault())
	assert.Equal(t, 45, cfg.SessionStart.TimeoutSecondsOrDefault())
	assert.False(t, cfg.SessionStart.PromptOn())
	assert.Empty(t, cfg.Hooks, "loading does not add the synthesized group")

	cfg.EnableSessionStartHooks()
	out, err := MarshalTOML(cfg)

	require.NoError(t, err)
	assert.Contains(t, string(out), "[session_start]")
	assert.NotContains(t, string(out), "session-start", "the synthesized hook is never written back")
}
