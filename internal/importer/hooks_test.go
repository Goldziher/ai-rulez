package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hookOf(event, matcher string, targets []string, commands ...string) config.HookGroup {
	g := config.HookGroup{Event: event, Matcher: matcher, Targets: targets}
	for _, c := range commands {
		g.Hooks = append(g.Hooks, config.HookAction{Command: c})
	}
	return g
}

func TestNativePlan_Hooks(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []config.HookGroup
	}{
		{
			name: "claude settings keep the matcher and handler fields",
			files: map[string]string{".claude/settings.json": `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
				{"type":"command","command":"echo guard","timeout":10},
				{"type":"command","command":"echo push","if":"Bash(git push *)","async":true,"statusMessage":"Checking"}]}]}}`},
			want: []config.HookGroup{{
				Event: "PreToolUse", Matcher: "Bash", Targets: []string{"claude"},
				Hooks: []config.HookAction{
					{Command: "echo guard", Timeout: 10},
					{Command: "echo push", If: "Bash(git push *)", Async: true, StatusMessage: "Checking"},
				},
			}},
		},
		{
			name:  "codex hooks.json is claude-shaped",
			files: map[string]string{".codex/hooks.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo done"}]}]}}`},
			want:  []config.HookGroup{hookOf("Stop", "", []string{"codex"}, "echo done")},
		},
		{
			name: "gemini events and millisecond timeouts are translated, the matcher keeps gemini's names",
			files: map[string]string{".gemini/settings.json": `{"hooks":{"BeforeTool":[{"matcher":"run_shell_command","hooks":[
				{"type":"command","command":"echo guard","timeout":2500}]}],"AfterAgent":[{"hooks":[{"type":"command","command":"echo done"}]}]}}`},
			want: []config.HookGroup{
				{Event: "PreToolUse", Matchers: map[string]string{"gemini": "run_shell_command"}, Targets: []string{"gemini"},
					Hooks: []config.HookAction{{Command: "echo guard", Timeout: 3}}},
				hookOf("Stop", "", []string{"gemini"}, "echo done"),
			},
		},
		{
			name: "cursor flat entries with camelCase events",
			files: map[string]string{".cursor/hooks.json": `{"version":1,"hooks":{"preToolUse":[{"command":"echo guard","timeout":4,"matcher":"Shell"}],
				"beforeSubmitPrompt":[{"command":"echo prompt"}]}}`},
			want: []config.HookGroup{
				{Event: "PreToolUse", Matchers: map[string]string{"cursor": "Shell"}, Targets: []string{"cursor"},
					Hooks: []config.HookAction{{Command: "echo guard", Timeout: 4}}},
				hookOf("UserPromptSubmit", "", []string{"cursor"}, "echo prompt"),
			},
		},
		{
			name: "copilot reads bash and timeoutSec, and skips the file ai-rulez writes",
			files: map[string]string{
				".github/hooks/team.json":     `{"version":1,"hooks":{"agentStop":[{"type":"command","bash":"echo done","timeoutSec":7}]}}`,
				".github/hooks/ai-rulez.json": `{"version":1,"hooks":{"sessionStart":[{"type":"command","bash":"echo generated"}]}}`,
			},
			want: []config.HookGroup{{Event: "Stop", Targets: []string{"copilot"}, Hooks: []config.HookAction{{Command: "echo done", Timeout: 7}}}},
		},
		{
			name: "the same hook in two tools is declared once for both",
			files: map[string]string{
				".claude/settings.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo done"}]}]}}`,
				".codex/hooks.json":     `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo done"}]}]}}`,
			},
			want: []config.HookGroup{hookOf("Stop", "", []string{"claude", "codex"}, "echo done")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fsys := mapFS(tt.files)
			// Act
			p := planOf(t, nativeImporter{}, fsys, Options{})
			// Assert
			assert.Equal(t, tt.want, p.Hooks)
		})
	}
}

func TestNativePlan_HookProblemsAreReported(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		status Status
		source string
	}{
		{"prompt hook", map[string]string{".claude/settings.json": `{"hooks":{"Stop":[{"hooks":[{"type":"prompt","prompt":"x"}]}]}}`}, StatusUnsupported, ".claude/settings.json"},
		{"cursor event without an ai-rulez name", map[string]string{".cursor/hooks.json": `{"hooks":{"afterFileEdit":[{"command":"fmt"}]}}`}, StatusUnsupported, ".cursor/hooks.json"},
		{"guard hook", map[string]string{".claude/settings.json": `{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"npx -y ai-rulez@latest guard"}]}]}}`}, StatusDropped, ".claude/settings.json"},
		{"inline codex hooks", map[string]string{".codex/config.toml": "[hooks]\nx = 1\n"}, StatusNeedsAction, ".codex/config.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planOf(t, nativeImporter{}, mapFS(tt.files), Options{})
			assert.Empty(t, p.Hooks)
			assert.NotNil(t, findingFor(p, tt.status, tt.source, ""), "%+v", p.Findings)
		})
	}
}

func TestNativePlan_HandlerKeysWithoutAnEquivalentAreReportedNotLost(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{".claude/settings.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"x","shell":"powershell"}]}]}}`})
	// Act
	p := planOf(t, nativeImporter{}, fsys, Options{})
	// Assert
	assert.Equal(t, []config.HookGroup{hookOf("Stop", "", []string{"claude"}, "x")}, p.Hooks)
	f := findingFor(p, StatusDropped, ".claude/settings.json", "hooks.Stop[0].hooks[0]")
	require.NotNil(t, f)
	assert.Contains(t, f.Reason, "shell")
}

func TestNativePlan_Permissions(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  config.Permissions
	}{
		{
			name: "claude settings rules verbatim",
			files: map[string]string{".claude/settings.json": `{"permissions":{"allow":["Bash(npm run test:*)"],"ask":["Bash(git push:*)"],
				"deny":["Read(./.env)"],"defaultMode":"plan"}}`},
			want: config.Permissions{Allow: []string{"Bash(npm run test:*)"}, Ask: []string{"Bash(git push:*)"}, Deny: []string{"Read(./.env)"}},
		},
		{
			name:  "cursor rules are rewritten to claude syntax",
			files: map[string]string{".cursor/cli.json": `{"permissions":{"allow":["Shell(ls)","Write(src/**)","Mcp(github:create_issue)"],"deny":["Read(.env)","Mcp(evil:*)"]}}`},
			want:  config.Permissions{Allow: []string{"Bash(ls)", "Edit(src/**)", "mcp__github__create_issue"}, Deny: []string{"Read(.env)", "mcp__evil"}},
		},
		{
			name:  "a cursor shell deny covers every command that starts with the base",
			files: map[string]string{".cursor/cli.json": `{"permissions":{"allow":["Shell(ls)"],"deny":["Shell(rm)","Shell(git push)","Shell(curl *)"]}}`},
			want:  config.Permissions{Allow: []string{"Bash(ls)"}, Deny: []string{"Bash(curl *)", "Bash(git push:*)", "Bash(rm:*)"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planOf(t, nativeImporter{}, mapFS(tt.files), Options{})
			assert.Equal(t, tt.want.Allow, p.Permissions.Allow)
			assert.Equal(t, tt.want.Ask, p.Permissions.Ask)
			assert.Equal(t, tt.want.Deny, p.Permissions.Deny)
		})
	}
}

func TestNativePlan_UnmappedPermissionFilesAreReported(t *testing.T) {
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{
		".gemini/settings.json":       `{"tools":{"allowed":["run_shell_command(ls)"]}}`,
		".codex/rules/ai-rulez.rules": "prefix_rule(pattern=[\"ls\"], decision=\"allow\")\n",
	}), Options{})
	assert.NotNil(t, findingFor(p, StatusUnsupported, ".gemini/settings.json", "tools"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, ".codex/rules", ""))
}

func TestRulesyncPlan_HooksAndPermissions(t *testing.T) {
	// Arrange
	fsys := mapFS(map[string]string{
		"rulesync.jsonc": `{"targets":["claudecode"]}`,
		".rulesync/hooks.jsonc": `{"version":1,"hooks":{
			"sessionStart":[{"type":"command","command":"git pull","timeout":20}],
			"preToolUse":[{"matcher":"Bash","command":".rulesync/hooks/confirm.sh"}],
			"afterFileEdit":[{"command":"fmt"}]},
			"claudecode":{"hooks":{"worktreeCreate":[{"command":"echo wt"}]}},
			"opencode":{"hooks":{"stop":[{"command":"echo oc"}]}}}`,
		".rulesync/permissions.jsonc": `{"permission":{"bash":{"git status":"allow","rm *":"deny","*":"ask"},
			"write":{"src/**":"allow"},"read":{".env":"deny"},"*":{"*":"deny"}},"claudecode":{"permission":{"bash":{"x":"allow"}}}}`,
	})
	// Act
	p := planOf(t, rulesyncImporter{}, fsys, Options{})
	// Assert
	assert.Equal(t, []config.HookGroup{
		hookOf("PreToolUse", "Bash", nil, ".rulesync/hooks/confirm.sh"),
		{Event: "SessionStart", Hooks: []config.HookAction{{Command: "git pull", Timeout: 20}}},
		hookOf("WorktreeCreate", "", []string{"claude"}, "echo wt"),
	}, p.Hooks)
	assert.Equal(t, []string{"Bash(git status)", "Edit(src/**)"}, p.Permissions.Allow)
	assert.Equal(t, []string{"Bash"}, p.Permissions.Ask)
	assert.Equal(t, []string{"Bash(rm *)", "Read(.env)"}, p.Permissions.Deny)
	assert.NotNil(t, findingFor(p, StatusUnsupported, ".rulesync/hooks.jsonc", "hooks"), "afterFileEdit has no ai-rulez event")
	assert.NotNil(t, findingFor(p, StatusUnsupported, ".rulesync/hooks.jsonc", "opencode.hooks"))
	assert.NotNil(t, findingFor(p, StatusNeedsAction, ".rulesync/hooks.jsonc", "hooks.preToolUse[0].hooks[0].command"))
	assert.NotNil(t, findingFor(p, StatusUnsupported, ".rulesync/permissions.jsonc", "permission.*"))
	assert.NotNil(t, findingFor(p, StatusDropped, ".rulesync/permissions.jsonc", "claudecode"))
}

const hookProject = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo guard","timeout":10}]}]},
"permissions":{"allow":["Bash(npm run test:*)"],"ask":["Bash(git push:*)"],"deny":["Read(./.env)"]}}`

func convertHooks(t *testing.T, opts ConvertOptions, files map[string]string) (dir string, report *Report, cfg string) {
	t.Helper()
	dir = t.TempDir()
	writeTree(t, dir, files)
	opts.Source, opts.Write = dir, true
	report, err := Convert(context.Background(), opts)
	require.NoError(t, err)
	data, _ := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	return dir, report, string(data)
}

func TestConvert_HooksAreWrittenDisabled(t *testing.T) {
	// Arrange / Act
	_, report, cfg := convertHooks(t, ConvertOptions{}, map[string]string{".claude/settings.json": hookProject, "CLAUDE.md": "x\n"})
	// Assert
	assert.Contains(t, cfg, "# [[hooks]]")
	assert.Contains(t, cfg, "# command = 'echo guard'")
	assert.NotContains(t, cfg, "\n[[hooks]]", "a hook must not be live unless --enable-hooks")
	assert.Contains(t, cfg, "# allow = ['Bash(npm run test:*)']", "an allow rule is held back")
	assert.Contains(t, cfg, "\n[permissions]\n", "deny and ask are live: they only narrow")
	assert.Contains(t, cfg, "deny = ['Read(./.env)']")
	assert.NotContains(t, cfg, "\nallow = ")
	assert.NotNil(t, findingNamed(report, StatusNeedsAction, "(hooks)"))
	assert.NotNil(t, findingNamed(report, StatusNeedsAction, "(permissions)"))

	loaded, err := config.DecodeTOMLConfig([]byte(cfg), "config.toml")
	require.NoError(t, err)
	assert.Empty(t, loaded.Hooks)
	require.NotNil(t, loaded.Permissions)
	assert.Empty(t, loaded.Permissions.Allow)
}

func findingNamed(r *Report, status Status, source string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].Status == status && r.Findings[i].Source == source {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestConvert_EnableFlagsWriteLiveHooksAndRules(t *testing.T) {
	// Arrange / Act
	_, _, cfg := convertHooks(t, ConvertOptions{EnableHooks: true, EnablePermissions: true},
		map[string]string{".claude/settings.json": hookProject, "CLAUDE.md": "x\n"})
	// Assert
	loaded, err := config.DecodeTOMLConfig([]byte(cfg), "config.toml")
	require.NoError(t, err)
	require.Len(t, loaded.Hooks, 1)
	assert.Equal(t, "PreToolUse", loaded.Hooks[0].Event)
	assert.Equal(t, []string{"claude"}, loaded.Hooks[0].Targets)
	assert.Equal(t, []string{"Bash(npm run test:*)"}, loaded.Permissions.Allow)
	assert.NotContains(t, cfg, "DISABLED")
}

func TestConvert_DisabledBlockIsIdempotent(t *testing.T) {
	// Arrange
	dir, _, first := convertHooks(t, ConvertOptions{}, map[string]string{".claude/settings.json": hookProject, "CLAUDE.md": "x\n"})
	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})
	require.NoError(t, err)
	second, _ := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	// Assert
	assert.Equal(t, first, string(second))
	for _, f := range report.Files {
		assert.Equal(t, ActionUnchanged, f.Action, f.Path)
	}
}

func TestConvert_MergesLiveHooksIntoAnExistingConfig(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".claude/settings.json": hookProject, "CLAUDE.md": "x\n",
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"mine\"\npresets = [\"claude\"]\n\n[permissions]\ndeny = [\"Read(./secrets)\"]\n",
	})
	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, EnableHooks: true})
	require.NoError(t, err)
	// Assert
	data, _ := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	loaded, err := config.DecodeTOMLConfig(data, "config.toml")
	require.NoError(t, err)
	assert.Len(t, loaded.Hooks, 1)
	assert.Equal(t, []string{"Read(./.env)", "Read(./secrets)"}, sortedCopy(loaded.Permissions.Deny))
	assert.True(t, strings.Contains(string(data), "# allow = ["), "the allow rule stays commented without --enable-permissions")
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestConvert_HookWithASecretBlocksTheWrite(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"CLAUDE.md":             "x\n",
		".claude/settings.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"curl -H 'Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz0123456789' https://x.example"}]}]}}`,
	})
	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})
	// Assert
	require.NoError(t, err)
	assert.True(t, report.Security.Blocked, "a disabled hook is still scanned")
	assert.False(t, report.Written)
}

func TestConvert_HooksAreNamedWhenTheConfigIsAV3File(t *testing.T) {
	// Arrange: config.toml cannot be written next to a V3 config, so the manual step lists what to add.
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".claude/settings.json": hookProject, "CLAUDE.md": "x\n",
		".ai-rulez/config.yaml": "version: 3\nname: old\n",
	})
	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})
	// Assert
	require.NoError(t, err)
	f := findingNamed(report, StatusNeedsAction, "config.yaml")
	require.NotNil(t, f)
	assert.Contains(t, f.Reason, "[[hooks]]")
	assert.Contains(t, f.Reason, "[permissions]")
}

func TestConvert_OnlyHooksIsConvertible(t *testing.T) {
	// Arrange / Act
	_, report, cfg := convertHooks(t, ConvertOptions{}, map[string]string{".claude/settings.json": hookProject})
	// Assert
	assert.True(t, report.Written)
	assert.Contains(t, cfg, "# [[hooks]]")
}
