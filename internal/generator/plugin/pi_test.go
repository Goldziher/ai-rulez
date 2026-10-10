package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePiPackage(t *testing.T) {
	cfg, err := config.LoadConfig(t.Context(), fixtureDir)
	require.NoError(t, err)
	cfg.Plugin.Runtimes = []string{"pi"}
	m, err := BuildManifest(cfg, cfg.Content)
	require.NoError(t, err)
	m.Commands = []config.ContentFile{{Name: "basemind-check", Content: "Check $ARGUMENTS.",
		Metadata: &config.Metadata{Extra: map[string]string{"description": "Check context", "allowed-tools": "Bash"}},
	}}
	outDir := t.TempDir()
	outputs, err := Generate(m, outDir)
	require.NoError(t, err)
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
	assert.Equal(t, "@goldziher/pi-basemind", pkg["name"])
	assert.Contains(t, pkg["keywords"], "pi-package")
	assert.Equal(t, map[string]any{
		"skills": []any{"./.pi/skills"}, "prompts": []any{"./.pi/prompts"},
	}, pkg["pi"])
	assert.Contains(t, pkg["files"], ".pi/")
	skill := outputByPath(t, outputs, ".pi/skills/basemind/SKILL.md")
	assert.Contains(t, string(skill.RawContent), "name: basemind")
	assert.NotEmpty(t, outputByPath(t, outputs, ".pi/skills/basemind/references/usage.md").RawContent)
	prompt := outputByPath(t, outputs, ".pi/prompts/basemind-check.md")
	assert.Contains(t, string(prompt.RawContent), "description:")
	assert.NotContains(t, string(prompt.RawContent), "allowed-tools:")
	assert.Contains(t, string(prompt.RawContent), "Check $ARGUMENTS.")
	for _, out := range outputs {
		assert.NotContains(t, out.Path, "mcp.json")
		assert.NotContains(t, out.Path, "agents/")
		assert.NotContains(t, out.Path, "extensions/")
		require.NoError(t, os.MkdirAll(filepath.Dir(out.Path), 0o755))
		require.NoError(t, os.WriteFile(out.Path, out.RawContent, 0o644))
	}
	require.NoError(t, VerifyProvenance(outDir))
}

func TestGeneratePiAndOpenCodeComposePackage(t *testing.T) {
	var previous []byte
	for _, runtimes := range [][]string{{"pi", "opencode"}, {"opencode", "pi"}} {
		cfg, err := config.LoadConfig(t.Context(), fixtureDir)
		require.NoError(t, err)
		cfg.Plugin.Runtimes = runtimes
		m, err := BuildManifest(cfg, cfg.Content)
		require.NoError(t, err)
		outputs, err := Generate(m, t.TempDir())
		require.NoError(t, err)
		data := outputByPath(t, outputs, "package.json").RawContent
		var pkg map[string]any
		require.NoError(t, json.Unmarshal(data, &pkg))
		assert.Equal(t, "@goldziher/opencode-basemind", pkg["name"])
		assert.Equal(t, ".opencode/plugins/basemind.js", pkg["main"])
		assert.Contains(t, pkg, "exports")
		assert.Contains(t, pkg, "dependencies")
		assert.Contains(t, pkg, "pi")
		assert.Contains(t, pkg["files"], ".pi/")
		assert.Contains(t, pkg["files"], ".opencode/")
		assert.Contains(t, pkg["keywords"], "pi-package")
		if previous != nil {
			assert.Equal(t, previous, data, "runtime order must not decide manifest contents")
		}
		previous = data
	}
}
