package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunVerifiersHandler(t *testing.T) {
	const head = "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"
	tests := []struct {
		name       string
		config     string
		strict     bool
		args       map[string]any
		wantOK     bool
		wantStatus string
		wantErr    bool
	}{
		{"passes", head + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \".ai-rulez/config.toml\"\n", false, nil, true, "pass", false},
		{"error failure", head + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\n", false, nil, false, "fail", false},
		{"warning passes", head + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\nseverity = \"warning\"\n", false, nil, true, "fail", false},
		{"strict warning fails", head + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\nseverity = \"warning\"\n", true, nil, false, "fail", false},
		{"unknown name is a tool error", head, false, map[string]any{"name": "nope"}, false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(tt.config), 0o600))
			args := map[string]any{"working_directory": dir, "strict": tt.strict}
			for k, v := range tt.args {
				args[k] = v
			}

			res, err := RunVerifiersHandler(context.Background(), newRequestWithArgs(args))

			require.NoError(t, err)
			if tt.wantErr {
				assert.True(t, res.IsError)
				return
			}
			require.False(t, res.IsError, textOf(t, res))
			var out struct {
				OK      bool `json:"ok"`
				Results []struct {
					Status string `json:"status"`
				} `json:"results"`
			}
			require.NoError(t, json.Unmarshal([]byte(textOf(t, res)), &out))
			assert.Equal(t, tt.wantOK, out.OK)
			require.Len(t, out.Results, 1)
			assert.Equal(t, tt.wantStatus, out.Results[0].Status)
		})
	}
}

func TestRunVerifiersHandler_GeneratedInSync(t *testing.T) {
	const head = "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"
	const sync = "[[verifiers]]\nname = \"sync\"\ntype = \"generated_in_sync\"\n"
	tests := []struct {
		name       string
		config     string
		stale      bool
		wantOK     bool
		wantStatus string
		wantMsg    string
	}{
		{"stale output fails", head + sync, true, false, "fail", "differ"},
		{"includes are an explicit error, never a pass", head + "[[includes]]\nname = \"i\"\nsource = \"https://example.com/r.git\"\n" + sync, false, false, "error", "not resolved over MCP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(tt.config), 0o600))
			if tt.stale {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("stale\n"), 0o600))
			}

			res, err := RunVerifiersHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			var out struct {
				OK      bool `json:"ok"`
				Results []struct {
					Status  string `json:"status"`
					Message string `json:"message"`
				} `json:"results"`
			}
			require.NoError(t, json.Unmarshal([]byte(textOf(t, res)), &out))
			assert.Equal(t, tt.wantOK, out.OK)
			require.Len(t, out.Results, 1)
			assert.Equal(t, tt.wantStatus, out.Results[0].Status, out.Results[0].Message)
			assert.Contains(t, out.Results[0].Message, tt.wantMsg)
		})
	}
}
