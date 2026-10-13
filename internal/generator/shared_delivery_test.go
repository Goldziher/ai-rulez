package generator

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

// sharedDeliveryProject writes a project with one skill of the given delivery
// and a bundled script, rendered for presets with agents_md on (the default).
func sharedDeliveryProject(t *testing.T, presets []string, delivery string) string {
	t.Helper()
	root := t.TempDir()
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = `"` + p + `"`
	}
	skill := "---\nname: demo\ndescription: Demo skill. Use when asked.\n"
	if delivery != "" {
		skill += "delivery: " + delivery + "\n"
	}
	skill += "---\nRun `scripts/tool.sh`.\n"
	files := map[string]string{
		"config.toml":                 "version = \"5.0\"\nname = \"shared\"\ngitignore = false\npresets = [" + strings.Join(quoted, ", ") + "]\n",
		"skills/demo/SKILL.md":        skill,
		"skills/demo/scripts/tool.sh": "#!/bin/sh\necho s\n",
		"skills/plain/SKILL.md":       "---\nname: plain\ndescription: Plain skill. Use when asked.\n---\nPLAIN\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
	}
	return root
}

func TestGenerate_SharedSkillsTreeHonoursDelivery(t *testing.T) {
	tests := []struct {
		name       string
		presets    []string
		delivery   string
		wantDemo   bool
		wantScript bool
		wantStub   bool
	}{
		{"codex served", []string{"codex"}, "served", false, false, true},
		{"cursor served", []string{"cursor"}, "served", false, false, true},
		{"gemini served", []string{"gemini"}, "served", false, false, true},
		{"copilot served", []string{"copilot"}, "served", false, false, true},
		{"opencode served", []string{"opencode"}, "served", false, false, true},
		{"zed served", []string{"zed"}, "served", false, false, true},
		{"codex both", []string{"codex"}, "both", true, true, true},
		{"codex static", []string{"codex"}, "static", true, true, false},
		{"codex default", []string{"codex"}, "", true, true, false},
		{"no MCP harness keeps served static", []string{"xum"}, "served", true, true, false},
		{"mixed readers keep the skill and add the stub", []string{"codex", "xum"}, "served", true, true, true},
		{"claude beside codex", []string{"claude", "codex"}, "served", false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := sharedDeliveryProject(t, tt.presets, tt.delivery)
			cfg, err := config.LoadConfig(context.Background(), root)
			require.NoError(t, err)

			// Act
			require.NoError(t, NewGenerator(cfg).Generate(""))

			// Assert
			tree := filepath.Join(root, ".agents", "skills")
			assert.Equal(t, tt.wantDemo, exists(t, filepath.Join(tree, "demo", "SKILL.md")), "demo skill")
			assert.Equal(t, tt.wantScript, exists(t, filepath.Join(tree, "demo", "scripts", "tool.sh")), "bundled script")
			assert.Equal(t, tt.wantStub, exists(t, filepath.Join(tree, "dynamic-skills", "SKILL.md")), "dynamic-skills stub")
			assert.True(t, exists(t, filepath.Join(tree, "plain", "SKILL.md")), "static skill")
		})
	}
}

func TestPresetsMissingStub_SharedTreeStubCounts(t *testing.T) {
	tests := []struct {
		name    string
		presets []string
	}{
		{"codex", []string{"codex"}},
		{"cursor", []string{"cursor"}},
		{"gemini", []string{"gemini"}},
		{"copilot", []string{"copilot"}},
		{"opencode", []string{"opencode"}},
		{"zed", []string{"zed"}},
		{"claude and codex", []string{"claude", "codex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := sharedDeliveryProject(t, tt.presets, "served")
			cfg, err := config.LoadConfig(context.Background(), root)
			require.NoError(t, err)

			// Act
			missing, err := NewGenerator(cfg).PresetsMissingStub("")

			// Assert
			require.NoError(t, err)
			assert.Empty(t, missing)
		})
	}
}
