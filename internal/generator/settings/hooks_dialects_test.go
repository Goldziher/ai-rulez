package settings_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

// dialectCase is one harness rendered through the generic hook renderer: the
// document it is merged into and the scope it is rendered in.
type dialectCase struct {
	harness string
	doc     string
	user    bool
}

var dialectCases = []dialectCase{
	{config.HarnessQwen, ".qwen/settings.json", false},
	{config.HarnessAugment, ".augment/settings.json", false},
	{config.HarnessCodeBuddy, ".codebuddy/settings.json", false},
	{config.HarnessQoder, ".qoder/settings.json", false},
	{config.HarnessCommandCode, ".commandcode/settings.json", false},
	{config.HarnessLetta, ".letta/settings.json", false},
	{config.HarnessFactory, ".factory/hooks.json", false},
	{config.HarnessAntigravity, ".agents/hooks.json", false},
	{config.HarnessGitLabDuo, ".gitlab/duo/hooks.json", false},
	{config.HarnessDevin, ".devin/hooks.v1.json", false},
	{config.HarnessGrok, ".grok/hooks/ai-rulez.json", false},
	{config.HarnessBob, ".bob/settings.json", false},
	{config.HarnessCortex, ".cortex/settings.json", false},
	{config.HarnessGoose, ".agents/plugins/ai-rulez/hooks/hooks.json", false},
	{config.HarnessDeepAgents, ".deepagents/hooks.json", false},
	{config.HarnessJunie, ".junie/config.json", true},
	{config.HarnessZCode, ".zcode/config.json", true},
	{config.HarnessCrush, "crush.json", false},
	{config.HarnessPoolside, ".poolside/settings.yaml", false},
	{config.HarnessReasonix, ".reasonix/settings.json", false},
	{config.HarnessHermes, ".hermes/config.yaml", true},
	{config.HarnessKiro, ".kiro/hooks/ai-rulez.json", false},
	{config.HarnessVibe, ".vibe/hooks.toml", false},
	{config.HarnessKimi, ".kimi-code/config.toml", true},
	{config.HarnessDevin + "/user", ".config/devin/config.json", true},
	{config.HarnessCortex + "/user", ".snowflake/cortex/hooks.json", true},
}

// toolMatchers is the matcher override of the harnesses without a documented tool
// vocabulary (internal/toolnames); the others render the group's Claude matcher
// through their vocabulary, which the goldens pin.
var toolMatchers = map[string]string{
	config.HarnessCommandCode: "shell", config.HarnessBob: "execute_command",
	config.HarnessReasonix: "bash", config.HarnessHermes: "terminal", config.HarnessKimi: "Shell",
	config.HarnessGitLabDuo: "startup",
}

// dialectHooks exercises a matcher with a per-harness override, a script with
// args, a timeout and an event most harnesses lack.
func dialectHooks(harness string) []config.HookGroup {
	matchers := map[string]string{}
	if m, ok := toolMatchers[harness]; ok {
		matchers[harness] = m
	}
	groups := []config.HookGroup{
		{
			Event: "PreToolUse", Matcher: "Bash", Matchers: matchers,
			Hooks: []config.HookAction{{Command: "echo guard", Timeout: 10}, {Command: "audit", Args: []string{"--strict", "a b"}}},
		},
		{Event: "SessionStart", Hooks: []config.HookAction{{Script: "scripts/boot.sh", StatusMessage: "Booting"}}},
		{Event: "Stop", Hooks: []config.HookAction{{Command: "echo done", Timeout: 5}}},
	}
	if harness == config.HarnessGitLabDuo {
		groups[1].Matcher = "startup"
		groups[1].Matchers = map[string]string{harness: "startup"}
	}
	return groups
}

func renderDialect(t *testing.T, c dialectCase, hooks []config.HookGroup, doc string) jsonmerge.Result {
	t.Helper()
	cfg := &config.Config{
		BaseDir: filepath.Dir(doc), ConfigDir: "/home/u/.ai-rulez", UserScope: c.user, Hooks: hooks,
		Run: config.NewRunState(),
	}
	harness := strings.TrimSuffix(c.harness, "/user")
	keys, err := settings.HookKeys(cfg, harness, doc)
	require.NoError(t, err)
	format, ok := docmerge.FormatFromPath(doc)
	require.True(t, ok)
	result, err := docmerge.Apply(doc, format, keys)
	require.NoError(t, err)
	return result
}

func dialectGolden(t *testing.T, c dialectCase, got string) {
	t.Helper()
	name := strings.ReplaceAll(c.harness, "/", "-")
	path := filepath.Join("testdata", "dialects", name+".golden"+filepath.Ext(c.doc))
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with UPDATE_GOLDEN=1 to create %s", path)
	assert.Equal(t, string(want), got)
}

// TestDialectHooksGolden pins the exact document of every harness rendered
// through the generic renderer.
func TestDialectHooksGolden(t *testing.T) {
	captureWarnings(t)
	for _, c := range dialectCases {
		t.Run(c.harness, func(t *testing.T) {
			doc := filepath.Join(t.TempDir(), filepath.FromSlash(c.doc))
			result := renderDialect(t, c, dialectHooks(strings.TrimSuffix(c.harness, "/user")), doc)
			require.NotEmpty(t, result.Body)
			assert.False(t, result.PartiallyOwned, "a fresh document is wholly ai-rulez's")
			dialectGolden(t, c, result.Body)
		})
	}
}

// TestDialectHooksRoundTrip checks, for every harness, that hand-written content
// of the same document survives generate, that a second generate changes nothing
// and that clean takes back exactly what was written.
func TestDialectHooksRoundTrip(t *testing.T) {
	captureWarnings(t)
	for _, c := range dialectCases {
		t.Run(c.harness, func(t *testing.T) {
			harness := strings.TrimSuffix(c.harness, "/user")
			doc := filepath.Join(t.TempDir(), filepath.FromSlash(c.doc))
			require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
			format, _ := docmerge.FormatFromPath(doc)

			// The first generate creates the document; the consumer then adds a
			// setting of their own beside what ai-rulez wrote.
			first := renderDialect(t, c, dialectHooks(harness), doc)
			require.NoError(t, os.WriteFile(doc, []byte(first.Body), 0o644))
			mine := map[docmerge.Format]string{
				docmerge.FormatJSON: "mine", docmerge.FormatTOML: "mine", docmerge.FormatYAML: "mine",
			}[format]
			applied, err := docmerge.Apply(doc, format, []jsonmerge.OwnedKey{{Name: mine, Value: "kept"}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(doc, []byte(applied.Body), 0o644))

			// Generate again: the result is the document as it is.
			cfg := &config.Config{BaseDir: filepath.Dir(doc), ConfigDir: "/home/u/.ai-rulez", UserScope: c.user,
				Hooks: dialectHooks(harness), Run: config.NewRunState()}
			cfg.Run.SetPreviousMerged(map[string][]jsonmerge.Claim{documentKey(doc, cfg): first.Claims})
			keys, err := settings.HookKeys(cfg, harness, doc)
			require.NoError(t, err)
			second, err := docmerge.Apply(doc, format, keys)
			require.NoError(t, err)
			assert.Equal(t, applied.Body, second.Body, "a second generate changes nothing")
			assert.True(t, second.PartiallyOwned, "the consumer's setting makes the document shared")

			// Clean removes ours and leaves theirs.
			cleaned, err := docmerge.Unmerge(doc, format, second.Claims)
			require.NoError(t, err)
			assert.False(t, cleaned.Empty)
			assert.Contains(t, cleaned.Body, "kept")
			assert.NotContains(t, cleaned.Body, "echo guard")
			assert.NotContains(t, cleaned.Body, "echo done")
		})
	}
}

func documentKey(doc string, cfg *config.Config) string {
	rel, err := filepath.Rel(cfg.BaseDir, doc)
	if err != nil {
		return doc
	}
	return filepath.ToSlash(rel)
}

func TestDialectHooksUnsupportedIsReported(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		group   config.HookGroup
		user    bool
		want    string
		emitted bool
	}{
		{"event with no equivalent", config.HarnessAugment,
			config.HookGroup{Event: "PermissionRequest", Hooks: []config.HookAction{{Command: "x"}}},
			false, "the event PermissionRequest has no equivalent", false},
		{"claude tool name without a vocabulary", config.HarnessBob,
			config.HookGroup{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "x"}}},
			false, `the matcher "Bash" of the PreToolUse group names Claude Code tools`, false},
		{"claude tool name the vocabulary does not document", config.HarnessFactory,
			config.HookGroup{Event: "PreToolUse", Matcher: "Bash|NotebookEdit", Hooks: []config.HookAction{{Command: "x"}}},
			false, `documents no equivalent of "NotebookEdit"`, false},
		{"claude tool name translated through the vocabulary", config.HarnessFactory,
			config.HookGroup{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "x"}}},
			false, "", true},
		{"override wins", config.HarnessFactory,
			config.HookGroup{Event: "PreToolUse", Matcher: "Bash", Matchers: map[string]string{"factory": "Execute"}, Hooks: []config.HookAction{{Command: "x"}}},
			false, "", true},
		{"passthrough vocabulary", config.HarnessQwen,
			config.HookGroup{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "x"}}},
			false, "", true},
		{"matcher on an event that ignores it", config.HarnessQwen,
			config.HookGroup{Event: "Stop", Matcher: "x", Hooks: []config.HookAction{{Command: "x"}}},
			false, "qwen ignores a matcher on Stop", false},
		{"if without a condition field", config.HarnessQwen,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Command: "x", If: "Bash(ls)"}}},
			false, "sets 'if'", false},
		{"async without an async field", config.HarnessKiro,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Command: "x", Async: true}}},
			false, "is async", false},
		{"non-command handler type", config.HarnessQwen,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Type: "http", Command: "x"}}},
			false, `has type "http"`, false},
		{"script where the harness documents no project root", config.HarnessAugment,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Script: "s.sh"}}},
			false, "documents no way to address a project file", false},
		{"script is fine in user scope", config.HarnessAugment,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Script: "s.sh"}}},
			true, "", true},
		{"project hooks of a user-only harness", config.HarnessJunie,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Command: "x"}}},
			false, "junie ignores project-level hooks", false},
		{"user hooks of a user-only harness", config.HarnessJunie,
			config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Command: "x"}}},
			true, "", true},
		{"matcher of gitlab-duo is a session source", config.HarnessGitLabDuo,
			config.HookGroup{Event: "SessionStart", Matchers: map[string]string{"gitlab-duo": "startup"}, Hooks: []config.HookAction{{Command: "x"}}},
			false, "", true},
		{"targets exclude the harness silently", config.HarnessQwen,
			config.HookGroup{Event: "Stop", Targets: []string{"claude"}, Hooks: []config.HookAction{{Command: "x"}}},
			false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := captureWarnings(t)
			cfg := &config.Config{BaseDir: t.TempDir(), UserScope: tt.user, ConfigDir: "/home/u/.ai-rulez",
				Hooks: []config.HookGroup{tt.group}, Run: config.NewRunState()}

			keys, err := settings.HookKeys(cfg, tt.harness, "")

			require.NoError(t, err)
			assert.Equal(t, tt.emitted, len(keys) > 0)
			if tt.want == "" {
				for _, w := range *warnings {
					assert.NotContains(t, w, "not generated for "+tt.harness+":")
				}
				return
			}
			require.NotEmpty(t, *warnings)
			joined := strings.Join(*warnings, "\n")
			assert.Contains(t, joined, tt.want)
			assert.Contains(t, joined, "[[hooks]] not generated for "+tt.harness)
		})
	}
}

func TestDialectHookScriptReferences(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Script: "scripts/done.sh"}}}}
	tests := []struct{ harness, want string }{
		{config.HarnessQwen, `"$QWEN_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessCodeBuddy, `"$CODEBUDDY_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessQoder, `"$QODER_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessCommandCode, `"$COMMANDCODE_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessFactory, `"$FACTORY_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessDevin, `"$DEVIN_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessGrok, `"$GROK_WORKSPACE_ROOT"/'scripts/done.sh'`},
		{config.HarnessCortex, `"$CORTEX_PROJECT_DIR"/'scripts/done.sh'`},
		{config.HarnessDeepAgents, `"${CLAUDE_PROJECT_DIR}"/'scripts/done.sh'`},
		{config.HarnessLetta, `'./scripts/done.sh'`},
		{config.HarnessAntigravity, `'./scripts/done.sh'`},
		{config.HarnessBob, `'./scripts/done.sh'`},
		{config.HarnessKiro, `'./scripts/done.sh'`},
	}
	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
			keys, err := settings.HookKeys(&config.Config{BaseDir: t.TempDir(), Hooks: hooks, Run: config.NewRunState()}, tt.harness, "")
			require.NoError(t, err)
			require.NotEmpty(t, keys)
			result, err := jsonmerge.Apply("", keys)
			require.NoError(t, err)
			assert.Contains(t, result.Body, strings.ReplaceAll(tt.want, `"`, `\"`))
		})
	}
}

func TestDialectTimeoutUnits(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Command: "x", Timeout: 7}}}}
	tests := []struct {
		harness, want string
		user          bool
	}{
		{config.HarnessQwen, `"timeout": 7`, false},
		{config.HarnessAugment, `"timeout": 7000`, false},
		{config.HarnessLetta, `"timeout": 7000`, false},
		{config.HarnessReasonix, `"timeout": 7000`, false},
		{config.HarnessZCode, `"timeoutMs": 7000`, true},
		{config.HarnessKiro, `"timeout": 7`, false},
	}
	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
			cfg := &config.Config{BaseDir: t.TempDir(), UserScope: tt.user, Hooks: hooks, Run: config.NewRunState()}
			keys, err := settings.HookKeys(cfg, tt.harness, "")
			require.NoError(t, err)
			result, err := jsonmerge.Apply("", keys)
			require.NoError(t, err)
			assert.Contains(t, result.Body, tt.want)
		})
	}
}

func TestDialectSupportedEvents(t *testing.T) {
	assert.Equal(t, []string{"SessionStart"}, settings.SupportedEvents(config.HarnessGitLabDuo))
	assert.Equal(t, []string{"PreToolUse"}, settings.SupportedEvents(config.HarnessCrush))
	assert.Equal(t, []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop"}, settings.SupportedEvents(config.HarnessCommandCode))
	native, ok := settings.NativeEvent(config.HarnessDevin, "PostCompact")
	assert.True(t, ok)
	assert.Equal(t, "PostCompaction", native)
	native, ok = settings.NativeEvent(config.HarnessVibe, "Stop")
	assert.True(t, ok)
	assert.Equal(t, "post_agent", native)
}

func TestDialectNamesAreHookHarnesses(t *testing.T) {
	for _, c := range dialectCases {
		harness := strings.TrimSuffix(c.harness, "/user")
		assert.True(t, settings.HasHookDialect(harness), harness)
		assert.Contains(t, config.HookHarnesses, harness)
	}
	assert.False(t, settings.HasHookDialect("nope"))
	assert.True(t, settings.HookDialectOwnsFile(config.HarnessCopilotCLI))
	assert.False(t, settings.HookDialectOwnsFile(config.HarnessQwen))
}

func TestZCodeEnablesHooksOnlyWhenAbsent(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Command: "x"}}}}
	doc := filepath.Join(t.TempDir(), ".zcode", "cli", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
	require.NoError(t, os.WriteFile(doc, []byte("{\n  \"hooks\": {\"enabled\": false}\n}\n"), 0o644))
	cfg := &config.Config{BaseDir: filepath.Dir(doc), UserScope: true, Hooks: hooks, Run: config.NewRunState()}

	keys, err := settings.HookKeys(cfg, config.HarnessZCode, doc)
	require.NoError(t, err)
	result, err := jsonmerge.Apply(doc, keys)
	require.NoError(t, err)

	assert.Contains(t, result.Body, `"enabled": false`, "a consumer's switch is theirs")
}

func TestClineHookScripts(t *testing.T) {
	warnings := captureWarnings(t)
	cfg := &config.Config{BaseDir: t.TempDir(), Hooks: []config.HookGroup{
		{Event: "PreToolUse", Hooks: []config.HookAction{{Command: "echo one"}, {Script: "scripts/two.sh", Args: []string{"a b"}}}},
		{Event: "SessionStart", Hooks: []config.HookAction{{Command: "echo start"}}},
		{Event: "Stop", Matcher: "x", Hooks: []config.HookAction{{Command: "echo never"}}},
		{Event: "PreCompact", Hooks: []config.HookAction{{Command: "echo slow", Timeout: 3}}},
		{Event: "FileChanged", Hooks: []config.HookAction{{Command: "echo never"}}},
	}}

	scripts := settings.ClineHookScripts(cfg)

	require.Len(t, scripts, 2)
	assert.Equal(t, "PreToolUse", scripts[0].Name)
	assert.Equal(t, "TaskStart", scripts[1].Name)
	assert.Equal(t, "#!/bin/sh\n"+
		"# Generated by ai-rulez (PreToolUse hook): edit [[hooks]] in .ai-rulez/config.toml, not this file.\n"+
		"# Every command receives the hook input on stdin; the last one's output is the hook's output,\n"+
		"# and the first command that fails stops the rest.\n"+
		"root=$(cd \"$(dirname \"$0\")/../..\" && pwd)\n"+
		"input=$(head -c 16777216)\n"+
		"printf '%s\\n' \"$input\" | { echo one\n} >/dev/null || exit $?\n"+
		"printf '%s\\n' \"$input\" | { \"$root\"/'scripts/two.sh' 'a b'\n}\n", scripts[0].Body)
	joined := strings.Join(*warnings, "\n")
	assert.Contains(t, joined, "Cline hooks have no matcher")
	assert.Contains(t, joined, "sets a timeout")
	assert.Contains(t, joined, "the event FileChanged has no equivalent")
}

// TestDocsDialectEventTable pins the "Harness | Events" table of docs/settings.md
// to the renderer's event tables.
func TestDocsDialectEventTable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "settings.md"))
	require.NoError(t, err)
	cell := regexp.MustCompile("`([A-Za-z]+)`(?: \\(`([A-Za-z_]+)`\\))?")

	inTable, rows := false, map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "| Harness | Events |"):
			inTable = true
			continue
		case !inTable:
			continue
		case !strings.HasPrefix(line, "|"):
			inTable = false
			continue
		case strings.HasPrefix(line, "| ----"):
			continue
		}
		cols := strings.SplitN(strings.Trim(line, "| "), "|", 2)
		harness := strings.Trim(strings.TrimSpace(cols[0]), "`")
		rows[harness] = true
		if harness == config.HarnessCline {
			continue
		}
		var documented []string
		for _, m := range cell.FindAllStringSubmatch(cols[1], -1) {
			native := m[2]
			if native == "" {
				native = m[1]
			}
			want, ok := settings.NativeEvent(harness, m[1])
			assert.True(t, ok, "%s documents %s, which the renderer does not support", harness, m[1])
			assert.Equal(t, want, native, "%s: native name of %s", harness, m[1])
			documented = append(documented, m[1])
		}
		assert.Equal(t, settings.SupportedEvents(harness), documented, "%s: supported events", harness)
	}
	for _, c := range dialectCases {
		harness := strings.TrimSuffix(c.harness, "/user")
		assert.True(t, rows[harness], "docs/settings.md lists the events of %s", harness)
	}
	assert.True(t, rows[config.HarnessCopilotCLI])
	assert.True(t, rows[config.HarnessCline])
}
