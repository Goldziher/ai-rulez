package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// rulesFormats maps each supported config format to its filename and a minimal body.
var rulesFormats = []struct {
	name string
	file string
	body string
}{
	{"yaml", "config.yaml", "version: \"4.0\"\nname: test\npresets:\n  - claude\n"},
	{"toml", "config.toml", "version = \"4.0\"\nname = \"test\"\npresets = [\"claude\"]\n"},
	{"json", "config.json", "{\"version\":\"4.0\",\"name\":\"test\",\"presets\":[\"claude\"]}\n"},
}

func writeRulesProject(t *testing.T, file, body string) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	path = filepath.Join(cfgDir, file)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return dir, path
}

func updateRules(t *testing.T, dir string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	args["working_directory"] = dir
	res, err := UpdateConfigHandler(context.Background(), newRequestWithArgs(args))
	require.NoError(t, err)
	return res
}

func resultText(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	return tc.Text
}

func loadRules(t *testing.T, dir string) *config.RulesConfig {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return cfg.Rules
}

func TestUpdateConfigHandler_RulesMode_Set(t *testing.T) {
	for _, f := range rulesFormats {
		t.Run(f.name, func(t *testing.T) {
			dir, _ := writeRulesProject(t, f.file, f.body)
			res := updateRules(t, dir, map[string]any{
				"rules_mode":           "split",
				"rules_mode_by_preset": map[string]any{"claude": "inline"},
			})
			require.False(t, res.IsError, resultText(t, res))
			assert.Equal(t, &config.RulesConfig{
				Mode:         "split",
				ModeByPreset: map[string]string{"claude": "inline"},
			}, loadRules(t, dir))
		})
	}
}

func TestUpdateConfigHandler_RulesMode_Rejections(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{"invalid mode", map[string]any{"rules_mode": "both"}},
		{"non-string mode", map[string]any{"rules_mode": 3}},
		{"invalid preset value", map[string]any{"rules_mode_by_preset": map[string]any{"claude": "x"}}},
		{"unknown preset", map[string]any{"rules_mode_by_preset": map[string]any{"nope": "split"}}},
		{"non-string preset value", map[string]any{"rules_mode_by_preset": map[string]any{"claude": 1}}},
		{"non-object map", map[string]any{"rules_mode_by_preset": "split"}},
	}
	for _, f := range rulesFormats {
		for _, tt := range tests {
			t.Run(f.name+"/"+tt.name, func(t *testing.T) {
				dir, path := writeRulesProject(t, f.file, f.body)
				res := updateRules(t, dir, tt.args)
				require.True(t, res.IsError)

				text := strings.NewReplacer("_", " ", ".", " ").Replace(strings.ToLower(resultText(t, res)))
				assert.Contains(t, text, "rules mode")

				after, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, f.body, string(after), "config file must be unchanged")
			})
		}
	}
}

func TestUpdateConfigHandler_RulesMode_Clearing(t *testing.T) {
	seedArgs := func() map[string]any {
		return map[string]any{
			"rules_mode":           "split",
			"rules_mode_by_preset": map[string]any{"claude": "inline", "cursor": "split"},
		}
	}
	tests := []struct {
		name string
		args map[string]any
		want *config.RulesConfig
	}{
		{
			"clear all",
			map[string]any{"rules_mode": "", "rules_mode_by_preset": map[string]any{}},
			nil,
		},
		{
			"empty mode keeps by-preset",
			map[string]any{"rules_mode": ""},
			&config.RulesConfig{ModeByPreset: map[string]string{"claude": "inline", "cursor": "split"}},
		},
		{
			"empty map keeps mode",
			map[string]any{"rules_mode_by_preset": map[string]any{}},
			&config.RulesConfig{Mode: "split"},
		},
		{
			"null map clears by-preset and keeps mode",
			map[string]any{"rules_mode_by_preset": nil},
			&config.RulesConfig{Mode: "split"},
		},
		{
			"empty entry removes that preset",
			map[string]any{"rules_mode_by_preset": map[string]any{"claude": "", "cursor": "split"}},
			&config.RulesConfig{Mode: "split", ModeByPreset: map[string]string{"cursor": "split"}},
		},
		{
			"all entries empty clears by-preset",
			map[string]any{"rules_mode_by_preset": map[string]any{"claude": "", "cursor": ""}},
			&config.RulesConfig{Mode: "split"},
		},
	}
	for _, f := range rulesFormats {
		for _, tt := range tests {
			t.Run(f.name+"/"+tt.name, func(t *testing.T) {
				dir, _ := writeRulesProject(t, f.file, f.body)
				res := updateRules(t, dir, seedArgs())
				require.False(t, res.IsError, resultText(t, res))

				res = updateRules(t, dir, tt.args)
				require.False(t, res.IsError, resultText(t, res))
				assert.Equal(t, tt.want, loadRules(t, dir))
			})
		}
	}
}

func TestReadConfigHandler_SurfacesRules(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	read := newRequestWithArgs(map[string]any{"working_directory": dir})
	res, err := ReadConfigHandler(context.Background(), read)
	require.NoError(t, err)
	payload := resultPayload(t, res)
	assert.Equal(t, "", payload["rules_mode"])
	m, ok := payload["rules_mode_by_preset"].(map[string]interface{})
	require.True(t, ok, "rules_mode_by_preset must always be present as a map")
	assert.Empty(t, m)

	updateRules(t, dir, map[string]any{
		"rules_mode":           "split",
		"rules_mode_by_preset": map[string]any{"cursor": "inline"},
	})

	res, err = ReadConfigHandler(context.Background(), read)
	require.NoError(t, err)
	payload = resultPayload(t, res)
	assert.Equal(t, "split", payload["rules_mode"])
	m, ok = payload["rules_mode_by_preset"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "inline", m["cursor"])
}
