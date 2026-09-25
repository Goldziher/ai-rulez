package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookScriptManifest returns a Manifest whose SourceDir contains an executable
// script at each of scriptPaths, with the given hook actions declared under
// SessionStart.
func hookScriptManifest(t *testing.T, actions []config.HookAction, scriptPaths ...string) *Manifest {
	t.Helper()
	sourceDir := t.TempDir()
	for _, rel := range scriptPaths {
		abs := filepath.Join(sourceDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte("#!/bin/sh\nai-rulez generate\n"), 0o644))
		require.NoError(t, os.Chmod(abs, 0o755))
	}
	return &Manifest{
		Name:      "basemind",
		Version:   "1.0.0",
		SourceDir: sourceDir,
		Hooks: []config.HookGroup{{
			Event:   "SessionStart",
			Matcher: "startup|resume",
			Hooks:   actions,
		}},
	}
}

// outputsByPath indexes rendered outputs by slash-normalized path relative to root.
func outputsByPath(t *testing.T, outs []config.OutputFile, root string) map[string]config.OutputFile {
	t.Helper()
	byPath := make(map[string]config.OutputFile, len(outs))
	for _, o := range outs {
		rel, err := filepath.Rel(root, o.Path)
		require.NoError(t, err)
		byPath[filepath.ToSlash(rel)] = o
	}
	return byPath
}

func TestRenderHooksFile_BundlesScriptNextToHooksJSON(t *testing.T) {
	tests := []struct {
		name        string
		runtime     string
		hooksJSON   string
		wantScript  string
		wantCommand string
	}{
		{
			name:        "claude uses its plugin root variable",
			runtime:     config.PluginRuntimeClaude,
			hooksJSON:   "hooks/hooks.json",
			wantScript:  "hooks/bootstrap.sh",
			wantCommand: "${CLAUDE_PLUGIN_ROOT}/hooks/bootstrap.sh",
		},
		{
			name:        "cursor uses its hook root variable",
			runtime:     config.PluginRuntimeCursor,
			hooksJSON:   ".cursor-plugin/hooks/hooks.json",
			wantScript:  ".cursor-plugin/hooks/bootstrap.sh",
			wantCommand: "${CURSOR_PLUGIN_ROOT:-.}/hooks/bootstrap.sh",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := hookScriptManifest(
				t,
				[]config.HookAction{{Script: "scripts/bootstrap.sh"}},
				"scripts/bootstrap.sh",
			)
			outs, err := renderHooksFile(m, tc.runtime, filepath.Join("/out", filepath.FromSlash(tc.hooksJSON)))
			require.NoError(t, err)

			byPath := outputsByPath(t, outs, "/out")
			script, ok := byPath[tc.wantScript]
			require.True(t, ok, "expected bundled hook script at %s, got %v", tc.wantScript, byPath)
			assert.Equal(t, "#!/bin/sh\nai-rulez generate\n", string(script.RawContent))

			// Compared against the source file rather than a literal 0o755:
			// Windows has no POSIX permission bits, so a chmod there cannot
			// produce one and the contract is "carried over", not "executable".
			sourceInfo, err := os.Stat(filepath.Join(m.SourceDir, filepath.FromSlash("scripts/bootstrap.sh")))
			require.NoError(t, err)
			assert.Equal(t, sourceInfo.Mode().Perm(), script.Mode,
				"the source file's mode must be carried over, executable bit included")

			hooksDoc, ok := byPath[tc.hooksJSON]
			require.True(t, ok, "expected %s", tc.hooksJSON)
			action := firstHookAction(t, hooksDoc.RawContent)
			assert.Equal(t, tc.wantCommand, action["command"])
		})
	}
}

func TestRenderHooksFile_OmitsUnsetHandlerFields(t *testing.T) {
	m := hookScriptManifest(t, []config.HookAction{{Command: "echo hi"}})
	outs, err := renderHooksFile(m, config.PluginRuntimeClaude, "/out/hooks/hooks.json")
	require.NoError(t, err)
	require.Len(t, outs, 1)

	action := firstHookAction(t, outs[0].RawContent)
	assert.Equal(t, map[string]any{
		"type":    "command",
		"command": "echo hi",
		"async":   false,
	}, action, "unset handler fields are omitted; async always serializes")
}

func TestRenderHooksFile_RendersFullHandlerFidelity(t *testing.T) {
	m := hookScriptManifest(
		t,
		[]config.HookAction{{
			Script:  "scripts/bootstrap.sh",
			Args:    []string{"generate", "--quiet"},
			Timeout: 120,
			Async:   true,
			// Permission-rule syntax, which is what Claude Code evaluates 'if'
			// against. The renderer passes it through verbatim, so the value here is
			// only about not modeling a syntax the runtime would reject.
			If:            "Edit(*.md)",
			StatusMessage: "Bootstrapping generated AI config",
		}},
		"scripts/bootstrap.sh",
	)
	outs, err := renderHooksFile(m, config.PluginRuntimeClaude, "/out/hooks/hooks.json")
	require.NoError(t, err)

	byPath := outputsByPath(t, outs, "/out")
	action := firstHookAction(t, byPath["hooks/hooks.json"].RawContent)
	assert.Equal(t, map[string]any{
		"type":          "command",
		"command":       "${CLAUDE_PLUGIN_ROOT}/hooks/bootstrap.sh",
		"args":          []any{"generate", "--quiet"},
		"timeout":       float64(120),
		"async":         true,
		"if":            "Edit(*.md)",
		"statusMessage": "Bootstrapping generated AI config",
	}, action)
}

func TestRenderHooksFile_RejectsConflictingScriptBasenames(t *testing.T) {
	m := hookScriptManifest(
		t,
		[]config.HookAction{
			{Script: "scripts/bootstrap.sh"},
			{Script: "other/bootstrap.sh"},
		},
		"scripts/bootstrap.sh",
		"other/bootstrap.sh",
	)
	_, err := renderHooksFile(m, config.PluginRuntimeClaude, "/out/hooks/hooks.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting hook script")
}

func TestRenderHooksFile_MissingScriptIsAnError(t *testing.T) {
	m := hookScriptManifest(t, []config.HookAction{{Script: "scripts/absent.sh"}})
	_, err := renderHooksFile(m, config.PluginRuntimeClaude, "/out/hooks/hooks.json")
	require.Error(t, err)
}

// firstHookAction extracts the first action of the first SessionStart group from
// a rendered hooks.json payload.
func firstHookAction(t *testing.T, data []byte) map[string]any {
	t.Helper()
	doc := parseJSON(t, data)
	hooks, ok := doc["hooks"].(map[string]any)
	require.True(t, ok, "hooks.json must nest groups under 'hooks'")
	groups, ok := hooks["SessionStart"].([]any)
	require.True(t, ok, "expected a SessionStart group")
	require.NotEmpty(t, groups)
	actions, ok := groups[0].(map[string]any)["hooks"].([]any)
	require.True(t, ok, "expected hook actions")
	require.NotEmpty(t, actions)
	action, ok := actions[0].(map[string]any)
	require.True(t, ok)
	return action
}
