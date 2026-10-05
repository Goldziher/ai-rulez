package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/generator/settings"
)

// sampleHooks exercises every branch of the renderers: a tool matcher with a
// per-harness override, an `if` (claude only), a script with args, a command with
// args, an event only some harnesses have, and a timeout.
func sampleHooks() []config.HookGroup {
	return []config.HookGroup{
		{
			Event:    "PreToolUse",
			Matcher:  "Bash",
			Matchers: map[string]string{"gemini": "run_shell_command", "cursor": "Shell"},
			Hooks: []config.HookAction{
				{Command: "echo guard", Timeout: 10},
				{Command: "echo push", If: "Bash(git push *)"},
			},
		},
		{
			Event: "SessionStart",
			Hooks: []config.HookAction{{Script: "scripts/boot.sh", Args: []string{"--quick", "a b"}, Timeout: 5, StatusMessage: "Booting"}},
		},
		{
			Event: "PostToolUseFailure",
			Hooks: []config.HookAction{{Command: "notify", Args: []string{"failed"}, Async: true}},
		},
		{
			Event: "Stop",
			Hooks: []config.HookAction{{Command: "echo done"}},
		},
	}
}

func sampleConfig(baseDir string) *config.Config {
	return &config.Config{BaseDir: baseDir, Hooks: sampleHooks()}
}

func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	var warnings []string
	restore := rulefiles.SetWarnSink(func(msg string, _ ...any) { warnings = append(warnings, msg) })
	t.Cleanup(restore)
	return &warnings
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with UPDATE_GOLDEN=1 to create %s", path)
	assert.Equal(t, string(want), got)
}

func TestHookKeysGolden(t *testing.T) {
	warnings := captureWarnings(t)
	for _, harness := range []string{config.HarnessClaude, config.HarnessCodex, config.HarnessCursor, config.HarnessGemini} {
		t.Run(harness, func(t *testing.T) {
			keys, err := settings.HookKeys(sampleConfig(t.TempDir()), harness, "")
			require.NoError(t, err)
			result, err := jsonmerge.Apply("", keys)
			require.NoError(t, err)
			golden(t, harness, result.Body)
		})
	}
	t.Run(config.HarnessCopilot, func(t *testing.T) {
		body, ok, err := settings.CopilotHooksDocument(sampleConfig(t.TempDir()))
		require.NoError(t, err)
		require.True(t, ok)
		golden(t, config.HarnessCopilot, body)
	})
	assert.NotEmpty(t, *warnings, "groups a harness cannot express are reported")
}

func TestHookKeysUnsupportedIsReported(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		group   config.HookGroup
		want    string
		emitted bool
	}{
		{
			name: "event with no equivalent", harness: config.HarnessCodex,
			group: config.HookGroup{Event: "FileChanged", Hooks: []config.HookAction{{Command: "x"}}},
			want:  "the event FileChanged has no equivalent",
		},
		{
			name: "claude matcher the vocabulary does not document", harness: config.HarnessGemini,
			group: config.HookGroup{Event: "PreToolUse", Matcher: "Bash|Task", Hooks: []config.HookAction{{Command: "x"}}},
			want:  `documents no equivalent of "Task"`,
		},
		{
			name: "matcher on a copilot event that ignores it", harness: config.HarnessCopilot,
			group: config.HookGroup{Event: "Stop", Matcher: "Bash", Hooks: []config.HookAction{{Command: "x"}}},
			want:  "copilot ignores a matcher on agentStop",
		},
		{
			name: "if on a harness without conditions", harness: config.HarnessCursor,
			group: config.HookGroup{Event: "PreToolUse", Hooks: []config.HookAction{{Command: "x", If: "Bash(ls)"}}},
			want:  "sets 'if'",
		},
		{
			name: "async on a harness without async", harness: config.HarnessGemini,
			group: config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Command: "x", Async: true}}},
			want:  "is async",
		},
		{
			name: "matcher copied where the vocabulary matches", harness: config.HarnessCodex,
			group:   config.HookGroup{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "x"}}},
			emitted: true,
		},
		{
			name: "per-harness override wins", harness: config.HarnessGemini,
			group:   config.HookGroup{Event: "PreToolUse", Matcher: "Bash", Matchers: map[string]string{"gemini": "run_shell_command"}, Hooks: []config.HookAction{{Command: "x"}}},
			emitted: true,
		},
		{
			name: "targets exclude the harness silently", harness: config.HarnessCodex,
			group: config.HookGroup{Event: "FileChanged", Targets: []string{"claude"}, Hooks: []config.HookAction{{Command: "x"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := captureWarnings(t)
			cfg := &config.Config{BaseDir: t.TempDir(), Hooks: []config.HookGroup{tt.group}}
			keys, err := settings.HookKeys(cfg, tt.harness, "")
			require.NoError(t, err)
			assert.Equal(t, tt.emitted, len(keys) > 0)
			if tt.want == "" {
				assert.Empty(t, *warnings)
				return
			}
			require.Len(t, *warnings, 1)
			assert.Contains(t, (*warnings)[0], tt.want)
			assert.Contains(t, (*warnings)[0], "[[hooks]] not generated for "+tt.harness)
		})
	}
}

func TestHookScriptReferences(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "SessionStart", Hooks: []config.HookAction{{Script: "scripts/boot.sh"}}}}
	tests := []struct {
		harness, want string
	}{
		{config.HarnessClaude, `"${CLAUDE_PROJECT_DIR}"/'scripts/boot.sh'`},
		{config.HarnessCodex, `"$(git rev-parse --show-toplevel)"/'scripts/boot.sh'`},
		{config.HarnessGemini, `"$GEMINI_PROJECT_DIR"/'scripts/boot.sh'`},
		{config.HarnessCursor, `'./scripts/boot.sh'`},
	}
	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
			keys, err := settings.HookKeys(&config.Config{BaseDir: t.TempDir(), Hooks: hooks}, tt.harness, "")
			require.NoError(t, err)
			result, err := jsonmerge.Apply("", keys)
			require.NoError(t, err)
			assert.Contains(t, result.Body, strings.ReplaceAll(tt.want, `"`, `\"`))
		})
	}

	t.Run("user scope addresses the user config directory", func(t *testing.T) {
		cfg := &config.Config{BaseDir: "/home/u", ConfigDir: "/home/u/.config/ai-rulez", UserScope: true, Hooks: hooks}
		keys, err := settings.HookKeys(cfg, config.HarnessClaude, "")
		require.NoError(t, err)
		result, err := jsonmerge.Apply("", keys)
		require.NoError(t, err)
		assert.Contains(t, result.Body, `'/home/u/.config/ai-rulez/scripts/boot.sh'`)
	})
}

func TestScopeRunRendersNothing(t *testing.T) {
	cfg := sampleConfig(t.TempDir())
	cfg.Permissions = &config.Permissions{Allow: []string{"Bash(ls)"}}
	cfg.Run = config.NewRunState().ForScope(&config.ScopeRun{Path: "pkg"})
	keys, err := settings.ClaudeKeys(cfg, "")
	require.NoError(t, err)
	assert.Empty(t, keys)
}

const handAuthored = `{
  "model": "opus",
  "permissions": {
    "allow": ["Bash(make test)"],
    "deny": ["Read(./secrets/**)"]
  },
  "hooks": {
    "PreToolUse": [
      {"matcher": "Write", "hooks": [{"type": "command", "command": "./lint.sh"}]}
    ],
    "Notification": [
      {"hooks": [{"type": "command", "command": "say hi"}]}
    ]
  },
  "env": {"KEEP": "1"}
}
`

func claudeConfig(dir string) *config.Config {
	return &config.Config{
		BaseDir: dir,
		Hooks: []config.HookGroup{{
			Event: "PreToolUse", Matcher: "Bash",
			Hooks: []config.HookAction{{Command: "echo guard"}},
		}},
		Permissions: &config.Permissions{Allow: []string{"Bash(git status)"}, Deny: []string{"Read(./.env)"}},
		Claude: &config.ClaudeConfig{Settings: &config.ClaudeSettings{Managed: &config.ManagedSettings{
			Env:            map[string]string{"FOO": "bar"},
			SkillOverrides: map[string]string{"init": "off"},
		}}},
	}
}

func TestClaudeKeysPreserveHandAuthoredContent(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
	require.NoError(t, os.WriteFile(doc, []byte(handAuthored), 0o644))
	cfg := claudeConfig(dir)

	keys, err := settings.ClaudeKeys(cfg, doc)
	require.NoError(t, err)
	merged, err := jsonmerge.Apply(doc, keys)
	require.NoError(t, err)
	assert.True(t, merged.PartiallyOwned)
	for _, kept := range []string{`"model": "opus"`, `Bash(make test)`, `Read(./secrets/**)`, `./lint.sh`, `say hi`, `"KEEP": "1"`} {
		assert.Contains(t, merged.Body, kept, "hand-authored content survives generate")
	}
	for _, added := range []string{`echo guard`, `Bash(git status)`, `Read(./.env)`, `"FOO": "bar"`, `"init": "off"`} {
		assert.Contains(t, merged.Body, added)
	}

	// A second run over its own output changes nothing.
	require.NoError(t, os.WriteFile(doc, []byte(merged.Body), 0o644))
	again, err := settings.ClaudeKeys(cfg, doc)
	require.NoError(t, err)
	rerun, err := jsonmerge.Apply(doc, again)
	require.NoError(t, err)
	assert.Equal(t, merged.Body, rerun.Body, "generate is idempotent")

	// clean takes back exactly what was claimed.
	clean, err := jsonmerge.Unmerge(doc, merged.Claims)
	require.NoError(t, err)
	require.True(t, clean.Changed)
	assert.False(t, clean.Empty)
	for _, gone := range []string{`echo guard`, `Bash(git status)`, `Read(./.env)`, `"FOO"`, `"init"`} {
		assert.NotContains(t, clean.Body, gone)
	}
	for _, kept := range []string{`"model": "opus"`, `Bash(make test)`, `Read(./secrets/**)`, `./lint.sh`, `say hi`, `"KEEP": "1"`} {
		assert.Contains(t, clean.Body, kept, "hand-authored content survives clean")
	}
}

func TestClaudeKeysDropElementsNoLongerConfigured(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
	require.NoError(t, os.WriteFile(doc, []byte(handAuthored), 0o644))

	cfg := claudeConfig(dir)
	keys, err := settings.ClaudeKeys(cfg, doc)
	require.NoError(t, err)
	first, err := jsonmerge.Apply(doc, keys)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(doc, []byte(first.Body), 0o644))

	// The rule changes: the old element leaves, the new one arrives, the hand-written stay.
	next := claudeConfig(dir)
	next.Permissions.Allow = []string{"Bash(git log)"}
	next.Run = config.NewRunState()
	next.Run.SetPreviousMerged(map[string][]jsonmerge.Claim{".claude/settings.json": first.Claims})
	keys, err = settings.ClaudeKeys(next, doc)
	require.NoError(t, err)
	second, err := jsonmerge.Apply(doc, keys)
	require.NoError(t, err)
	assert.Contains(t, second.Body, "Bash(git log)")
	assert.NotContains(t, second.Body, "Bash(git status)")
	assert.Contains(t, second.Body, "Bash(make test)")
}

func TestCursorVersionIsOwnedOnlyWhenAdded(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Command: "echo done"}}}}
	dir := t.TempDir()
	doc := filepath.Join(dir, ".cursor", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))

	t.Run("fresh document gets version and clean removes the file", func(t *testing.T) {
		cfg := &config.Config{BaseDir: dir, Hooks: hooks}
		keys, err := settings.HookKeys(cfg, config.HarnessCursor, doc)
		require.NoError(t, err)
		result, err := jsonmerge.Apply(doc, keys)
		require.NoError(t, err)
		assert.Contains(t, result.Body, `"version": 1`)
		assert.False(t, result.PartiallyOwned)

		require.NoError(t, os.WriteFile(doc, []byte(result.Body), 0o644))
		clean, err := jsonmerge.Unmerge(doc, result.Claims)
		require.NoError(t, err)
		assert.True(t, clean.Empty, "nothing of the consumer is left, so the file goes")
	})

	t.Run("a consumer's version is theirs", func(t *testing.T) {
		require.NoError(t, os.WriteFile(doc, []byte("{\n  \"version\": 1,\n  \"hooks\": {\"afterFileEdit\": [{\"command\": \"fmt\"}]}\n}\n"), 0o644))
		cfg := &config.Config{BaseDir: dir, Hooks: hooks}
		keys, err := settings.HookKeys(cfg, config.HarnessCursor, doc)
		require.NoError(t, err)
		result, err := jsonmerge.Apply(doc, keys)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(doc, []byte(result.Body), 0o644))
		clean, err := jsonmerge.Unmerge(doc, result.Claims)
		require.NoError(t, err)
		assert.False(t, clean.Empty)
		assert.Contains(t, clean.Body, `"version": 1`)
		assert.Contains(t, clean.Body, "afterFileEdit")
		assert.NotContains(t, clean.Body, "echo done")
	})
}

func TestSupportedEvents(t *testing.T) {
	assert.Equal(t, config.KnownHookEvents, settings.SupportedEvents(config.HarnessClaude))
	assert.Contains(t, settings.SupportedEvents(config.HarnessCodex), "PermissionRequest")
	assert.NotContains(t, settings.SupportedEvents(config.HarnessCodex), "Notification")
	native, ok := settings.NativeEvent(config.HarnessGemini, "PreToolUse")
	assert.True(t, ok)
	assert.Equal(t, "BeforeTool", native)
	assert.Nil(t, settings.SupportedEvents("windsurf"))
}

func TestUnsupportedDiagnostics(t *testing.T) {
	cfg := &config.Config{
		Presets:     []config.Preset{{BuiltIn: "claude"}, {BuiltIn: "codex"}, {BuiltIn: "windsurf"}, {BuiltIn: "mcp"}, {BuiltIn: "amp"}},
		Hooks:       sampleHooks(),
		Permissions: &config.Permissions{Allow: []string{"Bash(ls)"}},
	}
	diagnostics := settings.UnsupportedDiagnostics(cfg)
	require.Len(t, diagnostics, 2)
	assert.Contains(t, diagnostics[0], "not generated for windsurf:")
	assert.NotContains(t, diagnostics[0], "mcp")
	assert.Contains(t, diagnostics[1], "amp, windsurf")

	assert.Empty(t, settings.UnsupportedDiagnostics(&config.Config{Presets: cfg.Presets}))
}

// TestDocsEventTable pins the event table of docs/settings.md to the renderer:
// "yes" cells are the harness's own name for the Claude Code event, "-" cells are
// events it cannot express, and any other cell names the native event.
func TestDocsEventTable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "settings.md"))
	require.NoError(t, err)
	harnesses := []string{config.HarnessClaude, config.HarnessCodex, config.HarnessGemini, config.HarnessCursor, config.HarnessCopilot}

	rows := 0
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "| Claude Code event") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 1+len(harnesses) || strings.HasPrefix(cells[0], "-") || strings.HasPrefix(cells[0], "every other") {
			continue
		}
		events := strings.Split(strings.ReplaceAll(cells[0], "`", ""), " / ")
		for col, harness := range harnesses {
			cell := strings.ReplaceAll(cells[col+1], "`", "")
			natives := strings.Split(cell, " / ")
			for i, event := range events {
				native, ok := settings.NativeEvent(harness, event)
				switch cell {
				case "-":
					assert.False(t, ok, "%s should not support %s", harness, event)
				case "yes":
					assert.True(t, ok, "%s should support %s", harness, event)
					assert.Equal(t, event, native)
				default:
					assert.True(t, ok, "%s should support %s", harness, event)
					require.Less(t, i, len(natives))
					assert.Equal(t, natives[i], native, "%s native name of %s", harness, event)
				}
			}
		}
		rows++
	}
	assert.Equal(t, 12, rows, "every event row of the table is checked")

	// Every event a non-Claude harness supports has a row.
	documented := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		for _, event := range config.KnownHookEvents {
			if strings.HasPrefix(line, "| `"+event+"`") || strings.Contains(line, "`"+event+"` / ") || strings.Contains(line, " / `"+event+"`") {
				documented[event] = true
			}
		}
	}
	for _, harness := range harnesses[1:] {
		for _, event := range settings.SupportedEvents(harness) {
			assert.True(t, documented[event], "%s event %s is missing from docs/settings.md", harness, event)
		}
	}
}

func TestManagedEntryTheConsumerAlreadyHasStaysTheirs(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
	require.NoError(t, os.WriteFile(doc, []byte("{\n  \"skillOverrides\": {\"init\": \"off\"},\n  \"env\": {\"KEEP\": \"1\"}\n}\n"), 0o644))

	cfg := &config.Config{BaseDir: dir, Claude: &config.ClaudeConfig{Settings: &config.ClaudeSettings{Managed: &config.ManagedSettings{
		SkillOverrides: map[string]string{"init": "off", "legacy": "name-only"},
		Env:            map[string]string{"KEEP": "2"},
	}}}}
	keys, err := settings.ClaudeKeys(cfg, doc)
	require.NoError(t, err)
	result, err := jsonmerge.Apply(doc, keys)
	require.NoError(t, err)
	assert.Contains(t, result.Body, `"legacy": "name-only"`)
	assert.Contains(t, result.Body, `"KEEP": "2"`, "a different value is the configuration's")

	require.NoError(t, os.WriteFile(doc, []byte(result.Body), 0o644))
	clean, err := jsonmerge.Unmerge(doc, result.Claims)
	require.NoError(t, err)
	assert.Contains(t, clean.Body, `"init": "off"`, "the identical entry was the consumer's before and stays after clean")
	assert.NotContains(t, clean.Body, "legacy")
	assert.NotContains(t, clean.Body, `"KEEP"`)
}
