package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

// hookHarnessPresets are the presets whose [[hooks]] are rendered into a file of
// their own (the settings-file harnesses of docs/settings.md), each with the
// project file it writes, the setting a consumer keeps beside the generated
// hooks and a marker the hook must carry.
var hookHarnessFiles = map[string]string{
	"claude":      ".claude/settings.json",
	"codex":       ".codex/hooks.json",
	"cursor":      ".cursor/hooks.json",
	"gemini":      ".gemini/settings.json",
	"copilot":     ".github/hooks/ai-rulez.json",
	"copilot-cli": ".github/hooks/ai-rulez.json",
	"factory":     ".factory/hooks.json",
	"antigravity": ".agents/hooks.json",
	"qwen":        ".qwen/settings.json",
	"augment":     ".augment/settings.json",
	"codebuddy":   ".codebuddy/settings.json",
	"qoder":       ".qoder/settings.json",
	"commandcode": ".commandcode/settings.json",
	"letta":       ".letta/settings.json",
	"gitlab-duo":  ".gitlab/duo/hooks.json",
	"devin":       ".devin/hooks.v1.json",
	"grok":        ".grok/hooks/ai-rulez.json",
	"bob":         ".bob/settings.json",
	"cortex":      ".cortex/settings.json",
	"goose":       ".agents/plugins/ai-rulez/hooks/hooks.json",
	"deepagents":  ".deepagents/hooks.json",
	"crush":       "crush.json",
	"poolside":    ".poolside/settings.yaml",
	"reasonix":    ".reasonix/settings.json",
	"kiro":        ".kiro/hooks/ai-rulez.json",
	"vibe":        ".vibe/hooks.toml",
	"cline":       ".clinerules/hooks/PreToolUse",
}

// userOnlyHookPresets render hooks only with --user: their vendors ignore
// project-level hooks.
var userOnlyHookPresets = []string{"junie", "zcode", "hermes", "kimi"}

func hookHarnessConfig(presets []string) string {
	event := "PreToolUse"
	if len(presets) == 1 && presets[0] == "gitlab-duo" {
		event = "SessionStart" // the only event Duo CLI has
	}
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = `"` + p + `"`
	}
	return `version = "5.0"
name = "hooks"
presets = [` + strings.Join(quoted, ", ") + `]
gitignore = false

[[hooks]]
event = "` + event + `"
[[hooks.hooks]]
command = "echo guard-marker"
`
}

func captureHookWarnings(t *testing.T) *[]string {
	t.Helper()
	var warnings []string
	t.Cleanup(diag.SetDefaultSink(func(msg string, _ ...any) { warnings = append(warnings, msg) }))
	return &warnings
}

// TestGenerate_HookHarnesses generates a hook for every harness with a hooks
// file, checks the hook lands where the vendor reads it, that a second run is a
// no-op, that nothing drifts, and that clean takes the file back.
func TestGenerate_HookHarnesses(t *testing.T) {
	for preset, rel := range hookHarnessFiles {
		t.Run(preset, func(t *testing.T) {
			captureHookWarnings(t)
			root := writeProject(t, hookHarnessConfig([]string{preset}), nil)

			gen := generateProject(t, root)

			body := readProjectFile(t, root, rel)
			assert.Contains(t, body, "guard-marker")
			if preset == "cline" {
				info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
				require.NoError(t, err)
				assert.NotZero(t, info.Mode().Perm()&0o100, "a Cline hook script is executable")
			}

			generateProject(t, root)
			assert.Equal(t, body, readProjectFile(t, root, rel), "a second generate changes nothing")

			cfg, err := config.LoadConfig(context.Background(), root)
			require.NoError(t, err)
			drift, err := NewGenerator(cfg).CheckDrift("default")
			require.NoError(t, err)
			for _, d := range drift {
				assert.NotEqual(t, rel, d.Path, "the hooks file is up to date")
			}

			_, err = gen.Clean("default", CleanOptions{})
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)), "clean takes back a file ai-rulez wrote whole")
		})
	}
}

// handAuthoredHookDocs is a hooks document a consumer wrote, per harness, with a
// hook of their own and a setting that is not a hook.
var handAuthoredHookDocs = map[string]string{
	".qwen/settings.json":                       "{\n  \"model\": \"mine\",\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".factory/hooks.json":                       "{\n  \"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]\n}\n",
	".agents/hooks.json":                        "{\n  \"mine\": {\"PreToolUse\": [{\"hooks\": [{\"command\": \"my-hook\"}]}]}\n}\n",
	"crush.json":                                "{\n  \"options\": {\"debug\": true},\n  \"hooks\": {\"PreToolUse\": [{\"command\": \"my-hook\"}]}\n}\n",
	".kiro/hooks/ai-rulez.json":                 "{\n  \"version\": \"v1\",\n  \"hooks\": [{\"name\": \"mine\", \"trigger\": \"PreToolUse\", \"action\": {\"type\": \"command\", \"command\": \"my-hook\"}}]\n}\n",
	".vibe/hooks.toml":                          "# my hooks\n[[hooks]]\nname = \"mine\"\ntype = \"pre_tool\"\nmatch = \"bash\"\ncommand = \"my-hook\"\n",
	".poolside/settings.yaml":                   "# my settings\nmodel: mine\nhooks:\n  PreToolUse:\n    - name: mine\n      matcher: shell\n      command: my-hook\n",
	".devin/hooks.v1.json":                      "{\n  \"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]\n}\n",
	".deepagents/hooks.json":                    "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".reasonix/settings.json":                   "{\n  \"hooks\": {\"PreToolUse\": [{\"command\": \"my-hook\"}]}\n}\n",
	".cortex/settings.json":                     "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".bob/settings.json":                        "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".grok/hooks/ai-rulez.json":                 "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".letta/settings.json":                      "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".commandcode/settings.json":                "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".qoder/settings.json":                      "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".codebuddy/settings.json":                  "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".augment/settings.json":                    "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".gitlab/duo/hooks.json":                    "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
	".agents/plugins/ai-rulez/hooks/hooks.json": "{\n  \"hooks\": {\"PreToolUse\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"my-hook\"}]}]}\n}\n",
}

// TestGenerate_HookHarnessesKeepHandAuthoredHooks checks, per harness, that what
// the consumer wrote in the hooks document survives generate and clean, that
// generate is idempotent over it and that clean removes exactly the generated hook.
func TestGenerate_HookHarnessesKeepHandAuthoredHooks(t *testing.T) {
	for preset, rel := range hookHarnessFiles {
		existing, ok := handAuthoredHookDocs[rel]
		if !ok {
			continue
		}
		t.Run(preset, func(t *testing.T) {
			captureHookWarnings(t)
			root := writeProject(t, hookHarnessConfig([]string{preset}), map[string]string{rel: existing})

			gen := generateProject(t, root)

			body := readProjectFile(t, root, rel)
			assert.Contains(t, body, "my-hook", "the consumer's hook survives generate")
			assert.Contains(t, body, "guard-marker")
			generateProject(t, root)
			assert.Equal(t, body, readProjectFile(t, root, rel), "a second generate changes nothing")

			_, err := gen.Clean("default", CleanOptions{})
			require.NoError(t, err)
			cleaned := readProjectFile(t, root, rel)
			assert.Contains(t, cleaned, "my-hook", "the consumer's hook survives clean")
			assert.NotContains(t, cleaned, "guard-marker", "clean removes the generated hook")
		})
	}
}

// TestGenerate_ClineKeepsHandWrittenScript checks that a hook script the consumer
// wrote under the name of an event is never overwritten.
func TestGenerate_ClineKeepsHandWrittenScript(t *testing.T) {
	warnings := captureHookWarnings(t)
	const script = "#!/bin/sh\necho mine\n"
	root := writeProject(t, hookHarnessConfig([]string{"cline"}), map[string]string{".clinerules/hooks/PreToolUse": script})

	generateProject(t, root)

	assert.Equal(t, script, readProjectFile(t, root, ".clinerules/hooks/PreToolUse"))
	assert.Contains(t, strings.Join(*warnings, "\n"), "already exists and was not written by ai-rulez")
}

// TestGenerate_UserOnlyHookHarnessesSkipTheProject checks that harnesses whose
// vendor ignores project hooks write nothing there, and say so.
func TestGenerate_UserOnlyHookHarnessesSkipTheProject(t *testing.T) {
	warnings := captureHookWarnings(t)
	root := writeProject(t, hookHarnessConfig(userOnlyHookPresets), nil)

	generateProject(t, root)

	for _, rel := range []string{".junie/config.json", ".zcode/config.json", ".hermes/config.yaml", ".kimi-code/config.toml"} {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	joined := strings.Join(*warnings, "\n")
	for _, name := range userOnlyHookPresets {
		assert.Contains(t, joined, name+" ignores project-level hooks")
	}
}

// TestGenerate_HookEventWithoutEquivalentIsReported checks that an event a harness
// lacks is skipped with a warning instead of being approximated.
func TestGenerate_HookEventWithoutEquivalentIsReported(t *testing.T) {
	warnings := captureHookWarnings(t)
	cfg := hookHarnessConfig([]string{"crush"}) + "\n[[hooks]]\nevent = \"Stop\"\n[[hooks.hooks]]\ncommand = \"echo stop-marker\"\n"
	root := writeProject(t, cfg, nil)

	generateProject(t, root)

	body := readProjectFile(t, root, "crush.json")
	assert.Contains(t, body, "guard-marker")
	assert.NotContains(t, body, "stop-marker")
	assert.Contains(t, strings.Join(*warnings, "\n"), "not generated for crush: the event Stop has no equivalent")
}

// TestGenerate_HooksShareASettingsDocumentWithMCPServers checks that an MCP
// sidecar and a hooks sidecar of one document both land in it.
func TestGenerate_HooksShareASettingsDocumentWithMCPServers(t *testing.T) {
	for preset, rel := range map[string]string{"crush": "crush.json", "poolside": ".poolside/settings.yaml", "qwen": ".qwen/settings.json"} {
		t.Run(preset, func(t *testing.T) {
			captureHookWarnings(t)
			cfg := hookHarnessConfig([]string{preset}) + "\n[[mcp_servers]]\nname = \"demo\"\ncommand = \"npx\"\nargs = [\"-y\", \"demo\"]\n"
			root := writeProject(t, cfg, nil)

			gen := generateProject(t, root)

			body := readProjectFile(t, root, rel)
			assert.Contains(t, body, "guard-marker")
			assert.Contains(t, body, "demo")

			_, err := gen.Clean("default", CleanOptions{})
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
		})
	}
}

// userHookFiles is where --user writes the hooks of each harness below the home
// directory.
var userHookFiles = map[string]string{
	"qwen":        ".qwen/settings.json",
	"augment":     ".augment/settings.json",
	"codebuddy":   ".codebuddy/settings.json",
	"qoder":       ".qoder/settings.json",
	"commandcode": ".commandcode/settings.json",
	"letta":       ".letta/settings.json",
	"factory":     ".factory/hooks.json",
	"antigravity": ".gemini/config/hooks.json",
	"gitlab-duo":  ".gitlab/duo/hooks.json",
	"devin":       ".config/devin/config.json",
	"grok":        ".grok/hooks/ai-rulez.json",
	"bob":         ".bob/settings/settings.json",
	"cortex":      ".snowflake/cortex/hooks.json",
	"goose":       ".agents/plugins/ai-rulez/hooks/hooks.json",
	"deepagents":  ".deepagents/hooks.json",
	"junie":       ".junie/config.json",
	"zcode":       ".zcode/cli/config.json",
	"crush":       ".config/crush/crush.json",
	"poolside":    ".config/poolside/settings.yaml",
	"reasonix":    ".reasonix/settings.json",
	"hermes":      ".hermes/config.yaml",
	"kiro":        ".kiro/hooks/ai-rulez.json",
	"vibe":        ".vibe/hooks.toml",
	"kimi":        ".kimi-code/config.toml",
	"copilot-cli": ".copilot/hooks/ai-rulez.json",
	"cline":       "Documents/Cline/Hooks/PreToolUse",
}

// TestUser_HooksLandWhereEachHarnessReadsThem checks the --user mapping of every
// harness with user-level hooks, including the four that have no project level.
func TestUser_HooksLandWhereEachHarnessReadsThem(t *testing.T) {
	for preset, rel := range userHookFiles {
		t.Run(preset, func(t *testing.T) {
			captureHookWarnings(t)
			event := "PreToolUse"
			if preset == "gitlab-duo" {
				event = "SessionStart"
			}
			cfg := "version = \"5.0\"\nname = \"me\"\npresets = [\"" + preset + "\"]\n\n[[hooks]]\nevent = \"" + event +
				"\"\n[[hooks.hooks]]\ncommand = \"echo user-marker\"\n"
			home, gen := newUserHome(t, cfg, userPresetFixture())
			gen.SetUserEnv(noEnv)

			_, err := gen.GenerateUser("")
			require.NoError(t, err)

			assert.Contains(t, readProjectFile(t, home, rel), "user-marker")

			_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(home, filepath.FromSlash(rel)))
		})
	}
}

// TestGenerate_CopilotAndCopilotCLIAgreeOnTheirSharedFile checks that the two
// presets that write .github/hooks/ai-rulez.json produce one document.
func TestGenerate_CopilotAndCopilotCLIAgreeOnTheirSharedFile(t *testing.T) {
	captureHookWarnings(t)
	root := writeProject(t, hookHarnessConfig([]string{"copilot", "copilot-cli"}), nil)
	generateProject(t, root)
	both := readProjectFile(t, root, ".github/hooks/ai-rulez.json")

	alone := writeProject(t, hookHarnessConfig([]string{"copilot"}), nil)
	generateProject(t, alone)
	assert.Equal(t, readProjectFile(t, alone, ".github/hooks/ai-rulez.json"), both)
}

// TestGitignoreMatrix_HookDocumentsAreOwnedAndIgnored checks, per harness, that the
// matrix fixture's [[hooks]] produce the hooks file and that git ignores it: a
// document ai-rulez alone wrote is generated output, not a tracked settings file.
func TestGitignoreMatrix_HookDocumentsAreOwnedAndIgnored(t *testing.T) {
	for preset, rel := range hookHarnessFiles {
		t.Run(preset, func(t *testing.T) {
			captureHookWarnings(t)
			base, outputs := generateWithGitignore(t, preset, false)

			found := false
			for _, out := range outputs {
				if filepath.ToSlash(strings.TrimPrefix(out.Path, base+string(filepath.Separator))) == rel {
					found = true
					assert.False(t, out.PartiallyOwned, "%s holds nothing but generated hooks", rel)
				}
			}
			assert.True(t, found, "%s is among the outputs of %s", rel, preset)
			assertGitignoreMatrix(t, base, outputs)
		})
	}
}
