package settings_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

// TestHookMatcherTranslation renders one PreToolUse group per harness and checks
// the matcher the document carries: Claude Code tool names are rewritten through
// the harness's documented vocabulary, or the group is skipped with a warning.
func TestHookMatcherTranslation(t *testing.T) {
	tests := []struct {
		name      string
		harness   string
		doc       string
		matcher   string
		overrides map[string]string
		want      string // substring of the rendered document; empty means the group is skipped
		warning   string
	}{
		{name: "cursor single", harness: "cursor", doc: ".cursor/hooks.json", matcher: "Bash", want: `"matcher": "^Shell$"`},
		{name: "cursor alternation", harness: "cursor", doc: ".cursor/hooks.json", matcher: "Edit|Read", want: `"matcher": "^Write$|^Read$"`},
		{name: "gemini single", harness: "gemini", doc: ".gemini/settings.json", matcher: "Bash", want: `"matcher": "^run_shell_command$"`},
		{name: "gemini anchored alternation", harness: "gemini", doc: ".gemini/settings.json", matcher: "^Edit$|^Write$", want: `"matcher": "^replace$|^write_file$"`},
		{name: "gemini mcp", harness: "gemini", doc: ".gemini/settings.json", matcher: "mcp__github__.*", want: `"matcher": "^mcp_github_.*$"`},
		{name: "factory single", harness: "factory", doc: ".factory/hooks.json", matcher: "Bash", want: `"matcher": "^Execute$"`},
		{name: "factory mcp", harness: "factory", doc: ".factory/hooks.json", matcher: "mcp__github__.*", want: `"matcher": "^mcp__github__.*$"`},
		{name: "devin anchored", harness: "devin", doc: ".devin/hooks.v1.json", matcher: "^Bash$", want: `"matcher": "^exec$"`},
		{name: "kiro shell", harness: "kiro", doc: ".kiro/hooks/ai-rulez.json", matcher: "Bash", want: `"matcher": "shell"`},
		{name: "goose single", harness: "goose", doc: ".agents/plugins/ai-rulez/hooks/hooks.json", matcher: "Bash", want: `"matcher": "^shell$"`},
		{name: "crush alternation", harness: "crush", doc: "crush.json", matcher: "Read|Grep", want: `"matcher": "^view$|^grep$"`},
		{name: "cortex single", harness: "cortex", doc: ".cortex/settings.json", matcher: "Bash", want: `"matcher": "^bash$"`},
		{name: "poolside pipe list", harness: "poolside", doc: ".poolside/settings.yaml", matcher: "Edit|Write", want: "matcher: edit|write"},
		{name: "vibe single", harness: "vibe", doc: ".vibe/hooks.toml", matcher: "Bash", want: `match = "bash"`},
		{name: "copilot single", harness: "copilot", matcher: "Bash", want: `"matcher": "bash"`},
		{name: "copilot-cli alternation", harness: "copilot-cli", matcher: "Edit|Write", want: `"matcher": "edit|create"`},
		{
			name: "unmapped token skips the group", harness: "devin", doc: ".devin/hooks.v1.json", matcher: "Bash|Task",
			warning: `documents no equivalent of "Task"`,
		},
		{
			name: "mcp without documented naming skips the group", harness: "cursor", doc: ".cursor/hooks.json",
			matcher: "mcp__github__.*", warning: `documents no equivalent of "mcp__github__.*"`,
		},
		{
			name: "no alternation in a glob matcher", harness: "vibe", doc: ".vibe/hooks.toml", matcher: "Bash|Grep",
			warning: "names Claude Code tools",
		},
		{
			name: "override beats the vocabulary", harness: "factory", doc: ".factory/hooks.json", matcher: "Bash",
			overrides: map[string]string{"factory": "Execute|Custom"}, want: `"matcher": "Execute|Custom"`,
		},
		{
			name: "override rescues an unmapped token", harness: "devin", doc: ".devin/hooks.v1.json", matcher: "Task",
			overrides: map[string]string{"devin": "^task$"}, want: `"matcher": "^task$"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			warnings := captureWarnings(t)
			dir := t.TempDir()
			group := config.HookGroup{
				Event: "PreToolUse", Matcher: tt.matcher, Matchers: tt.overrides,
				Hooks: []config.HookAction{{Command: "echo guard"}},
			}
			cfg := &config.Config{BaseDir: dir, Hooks: []config.HookGroup{group}, Run: config.NewRunState()}

			// Act
			var body string
			if tt.harness == "copilot" || tt.harness == "copilot-cli" {
				doc, _, err := settings.OwnedHooksDocument(cfg, tt.harness)
				require.NoError(t, err)
				body = doc
			} else {
				doc := filepath.Join(dir, tt.doc)
				keys, err := settings.HookKeys(cfg, tt.harness, doc)
				require.NoError(t, err)
				if len(keys) > 0 {
					c := dialectCase{harness: tt.harness, doc: doc}
					body = renderDialect(t, c, cfg.Hooks, doc).Body
					if tt.harness == "cursor" || tt.harness == "gemini" {
						result, err := jsonmerge.Apply("", keys)
						require.NoError(t, err)
						body = result.Body
					}
				}
			}

			// Assert
			if tt.want == "" {
				assert.Empty(t, body)
				require.NotEmpty(t, *warnings)
				assert.Contains(t, (*warnings)[0], tt.warning)
				return
			}
			assert.Contains(t, body, tt.want)
			for _, w := range *warnings {
				assert.NotContains(t, w, "not generated")
			}
		})
	}
}
