package hookplugins_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/hookplugins"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/opencodev1"
)

// baseHooks is the fixture of the goldens: a blocking tool matcher, a post-tool
// matcher, a session start with a source matcher and a stop.
func baseHooks() []config.HookGroup {
	return []config.HookGroup{
		{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "echo guard", Timeout: 10}}},
		{Event: "PostToolUse", Matcher: "Edit|Write", Hooks: []config.HookAction{{Command: "echo format"}}},
		{Event: "SessionStart", Matcher: "startup", Hooks: []config.HookAction{{Script: "scripts/boot.sh", Args: []string{"--quick", "a b"}}}},
		{Event: "Stop", Hooks: []config.HookAction{{Command: "echo done", Async: true}}},
	}
}

// extendedHooks adds the events only some dialects have.
func extendedHooks() []config.HookGroup {
	return append(baseHooks(),
		config.HookGroup{Event: "UserPromptSubmit", Hooks: []config.HookAction{{Command: "echo prompt"}}},
		config.HookGroup{Event: "PostToolUseFailure", Hooks: []config.HookAction{{Command: "echo failed"}}},
		config.HookGroup{Event: "PreCompact", Hooks: []config.HookAction{{Command: "echo before"}}},
		config.HookGroup{Event: "PostCompact", Hooks: []config.HookAction{{Command: "echo after"}}},
		config.HookGroup{Event: "SessionEnd", Hooks: []config.HookAction{{Command: "echo bye"}}},
	)
}

func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	rulefiles.ResetDowngrades()
	var warnings []string
	restore := rulefiles.SetWarnSink(func(msg string, _ ...any) { warnings = append(warnings, msg) })
	t.Cleanup(restore)
	return &warnings
}

func render(t *testing.T, hooks []config.HookGroup, harness string, flavor hookplugins.Flavor) string {
	t.Helper()
	body, ok, err := hookplugins.Render(&config.Config{Hooks: hooks}, harness, flavor)
	require.NoError(t, err)
	require.True(t, ok)
	return body
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with UPDATE_GOLDEN=1 to create %s", path)
	assert.Equal(t, string(want), got)
}

func TestRenderGolden(t *testing.T) {
	captureWarnings(t)
	tests := []struct {
		name    string
		hooks   []config.HookGroup
		harness string
		flavor  hookplugins.Flavor
		file    string
	}{
		{"opencode", baseHooks(), config.HarnessOpencode, hookplugins.FlavorOpencode, "opencode.golden.js"},
		{"kilo", baseHooks(), config.HarnessKilo, hookplugins.FlavorOpencodeV1, "kilo.golden.js"},
		{"pi", baseHooks(), config.HarnessPi, hookplugins.FlavorPi, "pi.golden.ts"},
		{"pi extended", extendedHooks(), config.HarnessPi, hookplugins.FlavorPi, "pi_extended.golden.ts"},
		{"amp", baseHooks(), config.HarnessAmp, hookplugins.FlavorAmp, "amp.golden.ts"},
		{"amp extended", extendedHooks(), config.HarnessAmp, hookplugins.FlavorAmp, "amp_extended.golden.ts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			got := render(t, tt.hooks, tt.harness, tt.flavor)

			// Assert
			golden(t, tt.file, got)
			assert.Equal(t, got, render(t, tt.hooks, tt.harness, tt.flavor), "rendering is deterministic")
		})
	}
}

func TestRenderEmbedsHooksAsOneJSONLiteral(t *testing.T) {
	captureWarnings(t)
	command := "echo \"a\" 'b'\nprintf `c` ${D} \\n </script>  "
	hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Command: command}}}}
	for _, flavor := range hookplugins.Flavors {
		t.Run(string(flavor), func(t *testing.T) {
			body := render(t, hooks, "h", flavor)

			literal := between(t, body, "const HOOK_CONFIG = ", ";\n\nconst DEFAULT_TIMEOUT_MS")
			var parsed struct {
				Hooks map[string][]struct {
					Command string `json:"command"`
				} `json:"hooks"`
			}
			require.NoError(t, json.Unmarshal([]byte(literal), &parsed))
			assert.Equal(t, command, parsed.Hooks["Stop"][0].Command)
			// The text of the command appears nowhere outside the literal.
			assert.NotContains(t, strings.Replace(body, literal, "", 1), "printf")
			assert.NotContains(t, body, " ", "line separators are escaped")
		})
	}
}

func between(t *testing.T, s, start, end string) string {
	t.Helper()
	i := strings.Index(s, start)
	require.GreaterOrEqual(t, i, 0)
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	require.GreaterOrEqual(t, j, 0)
	return rest[:j]
}

func TestRenderSkipsWhatTheHarnessCannotExpress(t *testing.T) {
	tests := []struct {
		name   string
		group  config.HookGroup
		flavor hookplugins.Flavor
		warns  string
	}{
		{"unsupported event", config.HookGroup{Event: "SubagentStop", Hooks: []config.HookAction{{Command: "x"}}},
			hookplugins.FlavorPi, "SubagentStop has no equivalent"},
		{"matcher without a subject", config.HookGroup{Event: "SessionEnd", Matcher: "logout", Hooks: []config.HookAction{{Command: "x"}}},
			hookplugins.FlavorPi, "reports nothing to match"},
		{"if condition", config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Command: "x", If: "Bash(git *)"}}},
			hookplugins.FlavorAmp, "sets 'if'"},
		{"non command handler", config.HookGroup{Event: "Stop", Hooks: []config.HookAction{{Type: "prompt", Command: "x"}}},
			hookplugins.FlavorAmp, `the type "prompt"`},
		{"event of another dialect", config.HookGroup{Event: "UserPromptSubmit", Hooks: []config.HookAction{{Command: "x"}}},
			hookplugins.FlavorOpencode, "UserPromptSubmit has no equivalent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := captureWarnings(t)

			_, ok, err := hookplugins.Render(&config.Config{Hooks: []config.HookGroup{tt.group}}, "h", tt.flavor)

			require.NoError(t, err)
			assert.False(t, ok, "nothing is left to generate")
			require.Len(t, *warnings, 1)
			assert.Contains(t, (*warnings)[0], tt.warns)
		})
	}
}

func TestRenderHonoursTargetsAndMatcherOverrides(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{
		{Event: "Stop", Targets: []string{config.HarnessCursor}, Hooks: []config.HookAction{{Command: "only-cursor"}}},
		{Event: "PreToolUse", Matcher: "Bash", Matchers: map[string]string{"amp": "Bash|edit_file"},
			Hooks: []config.HookAction{{Command: "guard"}}},
		{Event: "PreToolUse", Matcher: "*", Hooks: []config.HookAction{{Command: "all"}}},
	}

	body := render(t, hooks, config.HarnessAmp, hookplugins.FlavorAmp)

	assert.NotContains(t, body, "only-cursor")
	assert.Contains(t, body, `"matcher": "Bash|edit_file"`)
	assert.Equal(t, 1, strings.Count(body, `"matcher"`), "the * matcher selects every tool and is not written")
}

func TestRenderNothingForNoApplicableHook(t *testing.T) {
	captureWarnings(t)
	for name, cfg := range map[string]*config.Config{
		"nil":      nil,
		"no hooks": {},
		"scope":    {Hooks: baseHooks(), Run: &config.RunState{Scope: &config.ScopeRun{}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, ok, err := hookplugins.Render(cfg, config.HarnessPi, hookplugins.FlavorPi)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	}
}

func TestRenderRejectsUnknownFlavor(t *testing.T) {
	_, _, err := hookplugins.Render(&config.Config{Hooks: baseHooks()}, "h", "nope")
	require.Error(t, err)
}

func TestUserScopeScriptUsesTheConfigDirectory(t *testing.T) {
	captureWarnings(t)
	cfg := &config.Config{UserScope: true, ConfigDir: "/home/u/.config/ai-rulez", Hooks: []config.HookGroup{
		{Event: "Stop", Hooks: []config.HookAction{{Script: "hooks/stop.sh"}}},
	}}

	body, ok, err := hookplugins.Render(cfg, config.HarnessPi, hookplugins.FlavorPi)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, body, `"command": "'/home/u/.config/ai-rulez/hooks/stop.sh'"`)
}

func TestOpencodeModuleIsNotAV1Plugin(t *testing.T) {
	captureWarnings(t)
	body := render(t, baseHooks(), config.HarnessOpencode, hookplugins.FlavorOpencode)
	assert.False(t, opencodev1.IsV1Plugin(body), "the module carries a v2 id and setup")
}

func TestSupportedEvents(t *testing.T) {
	assert.Equal(t, []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop", "PostCompact"},
		hookplugins.SupportedEvents(hookplugins.FlavorOpencode))
	native, ok := hookplugins.NativeEvent(hookplugins.FlavorPi, "PreToolUse")
	assert.True(t, ok)
	assert.Equal(t, "tool_call", native)
	_, ok = hookplugins.NativeEvent(hookplugins.FlavorAmp, "PreCompact")
	assert.False(t, ok)
	assert.Nil(t, hookplugins.SupportedEvents("nope"))
}

func TestFlavorFor(t *testing.T) {
	for harness, want := range map[string]hookplugins.Flavor{
		"opencode": hookplugins.FlavorOpencode, "kilo": hookplugins.FlavorOpencodeV1,
		"mimocode": hookplugins.FlavorOpencodeV1, "pi": hookplugins.FlavorPi, "amp": hookplugins.FlavorAmp,
	} {
		got, ok := hookplugins.FlavorFor(harness)
		assert.True(t, ok, harness)
		assert.Equal(t, want, got, harness)
	}
	_, ok := hookplugins.FlavorFor("claude")
	assert.False(t, ok)
	assert.True(t, hookplugins.IsFlavor("pi"))
	assert.False(t, hookplugins.IsFlavor("kilo"))
}

// --- Behavior, with the generated module loaded by node and a fake host. ---

func requireNode(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the hook commands of these tests need a POSIX shell")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	return node
}

type call map[string]any

// driverTarget names the harness a module is rendered for and how the driver hosts it.
type driverTarget struct {
	harness      string
	flavor       hookplugins.Flavor
	driverFlavor string
	ext          string
}

var (
	piTarget  = driverTarget{config.HarnessPi, hookplugins.FlavorPi, "pi", ".ts"}
	ampTarget = driverTarget{config.HarnessAmp, hookplugins.FlavorAmp, "amp", ".ts"}
)

// runDriver renders the module, runs the calls against it with node and returns the
// blocking reason of each call ("" for none).
func runDriver(t *testing.T, hooks []config.HookGroup, target driverTarget, calls []call) []string {
	t.Helper()
	harness, flavor, driverFlavor, ext := target.harness, target.flavor, target.driverFlavor, target.ext
	node := requireNode(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	require.NoError(t, os.Mkdir(out, 0o755))
	body := render(t, hooks, harness, flavor)
	module := filepath.Join(dir, "plugin"+ext)
	require.NoError(t, os.WriteFile(module, []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"type":"module"}`), 0o644))
	driver, err := filepath.Abs(filepath.Join("testdata", "driver.mjs"))
	require.NoError(t, err)
	encoded, err := json.Marshal(calls)
	require.NoError(t, err)

	cmd := exec.Command(node, driver, module, driverFlavor, dir, string(encoded))
	cmd.Env = append(os.Environ(), "AI_RULEZ_TEST_OUT="+out)
	output, err := cmd.CombinedOutput()
	if err != nil && strings.Contains(string(output), "ERR_UNKNOWN_FILE_EXTENSION") {
		t.Skip("this node cannot run TypeScript")
	}
	require.NoError(t, err, string(output))
	var raw []*string
	require.NoError(t, json.Unmarshal([]byte(lastLine(string(output))), &raw), string(output))
	t.Setenv("AI_RULEZ_TEST_DIR", dir)
	results := make([]string, len(raw))
	for i, r := range raw {
		if r != nil {
			results[i] = *r
		}
	}
	return results
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func readOut(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("AI_RULEZ_TEST_DIR"), "out", name))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

const (
	writeIn   = `cat > "$AI_RULEZ_TEST_OUT/%s.json"`
	denyOnTwo = `cat > "$AI_RULEZ_TEST_OUT/pre.json"; echo "denied: bad" >&2; exit 2`
)

func behaviourHooks() []config.HookGroup {
	return []config.HookGroup{
		{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: denyOnTwo}}},
		{Event: "PostToolUse", Matcher: "Edit|Write", Hooks: []config.HookAction{{Command: strings.Replace(writeIn, "%s", "post", 1)}}},
		{Event: "SessionStart", Matcher: "startup", Hooks: []config.HookAction{{Command: strings.Replace(writeIn, "%s", "start", 1)}}},
		{Event: "Stop", Hooks: []config.HookAction{{Command: strings.Replace(writeIn, "%s", "stop", 1)}}},
		// Tests the escaping: quotes, a backtick, ${} and a newline reach the shell as written.
		{Event: "Stop", Hooks: []config.HookAction{{Command: "printf one > \"$AI_RULEZ_TEST_OUT/n1\"\n" +
			`printf '%s' 'it'"'"'s ` + "`x`" + ` ${y} \n "z"' > "$AI_RULEZ_TEST_OUT/esc"`}}},
	}
}

func TestBehaviour(t *testing.T) {
	captureWarnings(t)
	tests := []struct {
		name         string
		harness      string
		flavor       hookplugins.Flavor
		driverFlavor string
		ext          string
		// Native names of the tools the harness runs: shell, edit.
		shell, edit string
	}{
		{"opencode server", config.HarnessOpencode, hookplugins.FlavorOpencode, "opencode-v1", ".js", "bash", "edit"},
		{"opencode setup", config.HarnessOpencode, hookplugins.FlavorOpencode, "opencode-v2", ".js", "bash", "edit"},
		{"kilo", config.HarnessKilo, hookplugins.FlavorOpencodeV1, "opencode-v1", ".js", "bash", "edit"},
		{"pi", config.HarnessPi, hookplugins.FlavorPi, "pi", ".ts", "bash", "edit"},
		{"amp", config.HarnessAmp, hookplugins.FlavorAmp, "amp", ".ts", "Bash", "edit_file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v2 := tt.driverFlavor == "opencode-v2"
			calls := []call{
				{"kind": "tool_before", "tool": tt.shell, "input": map[string]any{"command": "rm -rf x"}},
				{"kind": "tool_before", "tool": "read", "input": map[string]any{"path": "a.go"}},
				{"kind": "tool_after", "tool": tt.edit, "input": map[string]any{"filePath": "a.go"}},
			}
			if !v2 {
				calls = append(calls, call{"kind": "session_start"}, call{"kind": "stop"})
			}

			results := runDriver(t, behaviourHooks(), driverTarget{tt.harness, tt.flavor, tt.driverFlavor, tt.ext}, calls)

			assert.Equal(t, "denied: bad", results[0], "exit code 2 of a pre-tool hook blocks the call")
			var pre map[string]any
			require.NoError(t, json.Unmarshal([]byte(readOut(t, "pre.json")), &pre))
			assert.Equal(t, "PreToolUse", pre["hook_event_name"])
			assert.Equal(t, "Bash", pre["tool_name"], "the Claude Code name of the tool")
			assert.Equal(t, tt.shell, pre["harness_tool_name"])
			assert.Equal(t, map[string]any{"command": "rm -rf x"}, pre["tool_input"])
			assert.Equal(t, "s1", pre["session_id"])
			assert.NotEmpty(t, pre["cwd"])
			assert.Empty(t, results[1], "a tool the matcher does not select is not blocked")
			var post map[string]any
			require.NoError(t, json.Unmarshal([]byte(readOut(t, "post.json")), &post))
			assert.Equal(t, "Edit", post["tool_name"])
			assert.Equal(t, "a.go", post["tool_input"].(map[string]any)["file_path"], "the file path is named as in Claude Code")
			if v2 {
				return
			}
			assert.Contains(t, readOut(t, "start.json"), `"hook_event_name":"SessionStart"`)
			assert.Contains(t, readOut(t, "start.json"), `"source":"startup"`)
			assert.Contains(t, readOut(t, "stop.json"), `"hook_event_name":"Stop"`)
			assert.Equal(t, "one", readOut(t, "n1"))
			assert.Equal(t, "it's `x` ${y} \\n \"z\"", readOut(t, "esc"))
		})
	}
}

func TestBehaviourV2SessionEvents(t *testing.T) {
	captureWarnings(t)

	runDriver(t, behaviourHooks(), driverTarget{config.HarnessOpencode, hookplugins.FlavorOpencode, "opencode-v2", ".js"}, []call{})

	assert.Contains(t, readOut(t, "start.json"), `"hook_event_name":"SessionStart"`)
	assert.Contains(t, readOut(t, "stop.json"), `"hook_event_name":"Stop"`)
}

func TestBehaviourBlockingRules(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{
		{Event: "PreToolUse", Hooks: []config.HookAction{
			{Command: `echo "not a block" >&2; exit 1`},
			{Command: `printf '{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"json says no"}}'`},
		}},
		{Event: "PostToolUse", Hooks: []config.HookAction{{Command: `echo "post" >&2; exit 2`}}},
	}
	results := runDriver(t, hooks, piTarget, []call{
		{"kind": "tool_before", "tool": "bash", "input": map[string]any{}},
		{"kind": "tool_after", "tool": "bash", "input": map[string]any{}},
	})

	assert.Equal(t, "json says no", results[0], "a non-zero exit other than 2 does not block; a JSON deny does")
	assert.Empty(t, results[1], "a post-tool hook cannot block")
}

func TestBehaviourTimeoutDoesNotBlock(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "PreToolUse", Hooks: []config.HookAction{
		{Command: `sleep 5; exit 2`, Timeout: 1},
	}}}

	results := runDriver(t, hooks, ampTarget, []call{
		{"kind": "tool_before", "tool": "Bash", "input": map[string]any{}},
	})

	assert.Empty(t, results[0])
}

func TestBehaviourPiPromptAndFailureEvents(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{
		{Event: "UserPromptSubmit", Hooks: []config.HookAction{{Command: `cat > "$AI_RULEZ_TEST_OUT/prompt.json"; exit 2`}}},
		{Event: "PostToolUse", Hooks: []config.HookAction{{Command: `touch "$AI_RULEZ_TEST_OUT/ok"`}}},
		{Event: "PostToolUseFailure", Hooks: []config.HookAction{{Command: `touch "$AI_RULEZ_TEST_OUT/failed"`}}},
	}

	results := runDriver(t, hooks, piTarget, []call{
		{"kind": "prompt", "text": "hello"},
		{"kind": "tool_after", "tool": "bash", "input": map[string]any{}, "error": true},
	})

	assert.Equal(t, "handled", results[0], "exit code 2 cancels the prompt")
	assert.Contains(t, readOut(t, "prompt.json"), `"prompt":"hello"`)
	assert.Error(t, fileExists(filepath.Join(os.Getenv("AI_RULEZ_TEST_DIR"), "out", "ok")),
		"PostToolUse does not run for a failed call")
	assert.NoError(t, fileExists(filepath.Join(os.Getenv("AI_RULEZ_TEST_DIR"), "out", "failed")))
}

func fileExists(path string) error {
	_, err := os.Stat(path)
	return err
}

// TestSyntaxOfGeneratedModules parses every dialect. The TypeScript modules carry a
// single annotation, the type of the host object, so they are checked with tsc when
// it is installed and otherwise with the annotation stripped by node.
func TestSyntaxOfGeneratedModules(t *testing.T) {
	captureWarnings(t)
	node := requireNode(t)
	tsc, _ := exec.LookPath("tsc")
	tests := []struct {
		name, harness string
		typescript    bool
		flavor        hookplugins.Flavor
	}{
		{"opencode", config.HarnessOpencode, false, hookplugins.FlavorOpencode},
		{"kilo", config.HarnessKilo, false, hookplugins.FlavorOpencodeV1},
		{"pi", config.HarnessPi, true, hookplugins.FlavorPi},
		{"amp", config.HarnessAmp, true, hookplugins.FlavorAmp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			body := render(t, extendedHooks(), tt.harness, tt.flavor)
			if tt.typescript && tsc != "" {
				file := filepath.Join(dir, "plugin.ts")
				require.NoError(t, os.WriteFile(file, []byte(body), 0o644))
				out, err := exec.Command(tsc, "--noEmit", "--skipLibCheck", "--target", "es2022", "--module", "esnext", file).CombinedOutput()
				assert.NoError(t, err, string(out))
			}
			// The annotation is the only TypeScript syntax in the module.
			file := filepath.Join(dir, "plugin.mjs")
			require.NoError(t, os.WriteFile(file, []byte(strings.ReplaceAll(body, ": any", "")), 0o644))
			out, err := exec.Command(node, "--check", file).CombinedOutput()
			assert.NoError(t, err, string(out))
		})
	}
}

func TestIsModulePath(t *testing.T) {
	tests := map[string]bool{
		hookplugins.OpencodePath:                  true,
		hookplugins.KiloPath:                      true,
		hookplugins.PiPath:                        true,
		hookplugins.AmpPath:                       true,
		"pkg/.opencode/plugins/ai-rulez-hooks.js": true,
		".opencode/plugins/mine.js":               false,
		".opencode/ai-rulez-hooks.js":             false,
		".opencode/plugins":                       false,
	}
	for rel, want := range tests {
		assert.Equal(t, want, hookplugins.IsModulePath(rel), rel)
	}
}

func TestUserScopeConfigDirIsShellEscaped(t *testing.T) {
	captureWarnings(t)
	cfg := &config.Config{UserScope: true, ConfigDir: "/home/u/$(id) `x`/o'x", Hooks: []config.HookGroup{
		{Event: "Stop", Hooks: []config.HookAction{{Script: "hooks/stop.sh"}}},
	}}

	body, ok, err := hookplugins.Render(cfg, config.HarnessPi, hookplugins.FlavorPi)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, body, `"command": "'/home/u/$(id) `+"`x`"+`/o'\\''x/hooks/stop.sh'"`)
}

func TestUnsafeScriptIsSkippedWithAWarning(t *testing.T) {
	for _, script := range []string{"a b.sh", "x;id", "$(id)", "`id`", "a\nb", "a'b", "a\\b"} {
		t.Run(script, func(t *testing.T) {
			warnings := captureWarnings(t)
			hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Script: script}}}}

			_, ok, err := hookplugins.Render(&config.Config{Hooks: hooks}, "h", hookplugins.FlavorPi)

			require.NoError(t, err)
			assert.False(t, ok)
			require.Len(t, *warnings, 1)
			assert.Contains(t, (*warnings)[0], "unsafe")
		})
	}
}

func TestProjectScriptIsSingleQuoted(t *testing.T) {
	captureWarnings(t)
	hooks := []config.HookGroup{{Event: "Stop", Hooks: []config.HookAction{{Script: "scripts/stop.sh"}}}}

	body := render(t, hooks, config.HarnessPi, hookplugins.FlavorPi)

	assert.Contains(t, body, `"command": "'./scripts/stop.sh'"`)
}

// A matcher the runtime cannot evaluate would match nothing, so a blocking hook
// guarded by it would silently never run. Such a group is skipped at generation
// time, loudly.
func TestInvalidMatchersAreSkippedWithAWarning(t *testing.T) {
	tests := []struct {
		name, matcher string
	}{
		{"unbalanced open group", "(Bash"},
		{"unbalanced close group escaping the anchor", "Bash)|(Read"},
		{"unclosed class", "[abc"},
		{"dangling quantifier", "*Bash"},
		{"go-only inline flags", "(?i)bash"},
		{"go-only named group", "(?P<n>Bash)"},
		{"go-only end anchor", `Bash\z`},
		{"posix class", "[[:alpha:]]+"},
		{"lookahead", "Bash(?=x)"},
		{"lookbehind", "(?<=x)Bash"},
		{"backreference", `(a)\1`},
		{"unicode property", `\p{L}+`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := captureWarnings(t)
			hooks := []config.HookGroup{{Event: "PreToolUse", Matcher: tt.matcher, Hooks: []config.HookAction{{Command: "guard"}}}}

			_, ok, err := hookplugins.Render(&config.Config{Hooks: hooks}, config.HarnessAmp, hookplugins.FlavorAmp)

			require.NoError(t, err)
			assert.False(t, ok, "the group must not be generated")
			require.Len(t, *warnings, 1)
			assert.Contains(t, (*warnings)[0], "matcher")
			assert.Contains(t, (*warnings)[0], "not a valid")
		})
	}
}

func TestValidMatchersAreKept(t *testing.T) {
	for _, matcher := range []string{"Bash", "Edit|Write", "mcp__.*", "(Read|Grep)", "[A-Z]+[a-z]*", `\w+`, "Bash?"} {
		t.Run(matcher, func(t *testing.T) {
			captureWarnings(t)
			hooks := []config.HookGroup{{Event: "PreToolUse", Matcher: matcher, Hooks: []config.HookAction{{Command: "guard"}}}}

			body := render(t, hooks, config.HarnessAmp, hookplugins.FlavorAmp)

			assert.Contains(t, body, `"guard"`)
		})
	}
}

func TestMayWriteModuleChecksProvenance(t *testing.T) {
	dir := t.TempDir()
	generated := render(t, baseHooks(), config.HarnessPi, hookplugins.FlavorPi)
	tests := []struct {
		name     string
		content  *string
		want     bool
		wantWarn bool
	}{
		{"absent", nil, true, false},
		{"generated", &generated, true, false},
		{"hand written", ptr("export default () => {};\n"), false, true},
		{"marker below the header", ptr("export default () => {};\n// Generated by ai-rulez from the [[hooks]]\n"), false, true},
		{"empty", ptr(""), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := captureWarnings(t)
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".ts")
			if tt.content != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.content), 0o644))
			}

			got := hookplugins.MayWriteModule(path)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantWarn, len(*warnings) == 1, "%v", *warnings)
		})
	}
}

func ptr(s string) *string { return &s }
