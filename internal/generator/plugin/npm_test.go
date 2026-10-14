package plugin

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func npmOutput(t *testing.T, dir string, pkg map[string]any) config.OutputFile {
	t.Helper()
	data, err := json.Marshal(pkg)
	require.NoError(t, err)
	return config.OutputFile{Path: filepath.Join(dir, "package.json"), RawContent: data}
}

func TestComposeNPMPackagesMergeAuthoredOnlyRefreshesOpenCodeOwnedKeys(t *testing.T) {
	pi := map[string]any{
		"name": "@acme/probe", "version": "1.0.0", "description": "pi", "pi": map[string]any{},
		"type": "commonjs", "main": "old.js", "scripts": map[string]any{"test": "pi"},
		"dependencies": map[string]any{"authored": "1"},
	}
	tests := []struct {
		name     string
		openCode map[string]any
		wantErr  string
		check    func(t *testing.T, pkg map[string]any)
	}{
		{
			name: "owned keys are refreshed and dependencies merged",
			openCode: map[string]any{
				"name": "@acme/probe", "version": "1.0.0", "description": "pi", "type": "module",
				"main": ".opencode/plugins/probe.js", "dependencies": map[string]any{"@opencode/plugin": "^2.0.26"},
			},
			check: func(t *testing.T, pkg map[string]any) {
				assert.Equal(t, "module", pkg["type"])
				assert.Equal(t, ".opencode/plugins/probe.js", pkg["main"])
				assert.Equal(t, map[string]any{"authored": "1", "@opencode/plugin": "^2.0.26"}, pkg["dependencies"])
				assert.Equal(t, map[string]any{"test": "pi"}, pkg["scripts"])
			},
		},
		{
			name:     "a different description is a disagreement, not an overwrite",
			openCode: map[string]any{"name": "@acme/probe", "version": "1.0.0", "description": "opencode", "type": "module"},
			wantErr:  "disagree",
		},
		{
			name:     "a different version is a disagreement",
			openCode: map[string]any{"name": "@acme/probe", "version": "2.0.0", "description": "pi"},
			wantErr:  "disagree",
		},
		{
			name:     "a different name is a disagreement",
			openCode: map[string]any{"name": "other", "version": "1.0.0", "description": "pi"},
			wantErr:  "disagree",
		},
		{
			name:     "scripts are not refreshed from OpenCode",
			openCode: map[string]any{"name": "@acme/probe", "version": "1.0.0", "description": "pi", "scripts": map[string]any{"test": "oc"}},
			wantErr:  "disagree",
		},
	}
	for _, tt := range tests {
		for _, piFirst := range []bool{true, false} {
			order := "opencode first"
			if piFirst {
				order = "pi first"
			}
			t.Run(tt.name+", "+order, func(t *testing.T) {
				// Arrange
				dir := t.TempDir()
				outputs := []config.OutputFile{npmOutput(t, dir, tt.openCode), npmOutput(t, dir, pi)}
				if piFirst {
					outputs[0], outputs[1] = outputs[1], outputs[0]
				}

				// Act
				got, err := composeNPMPackages(outputs, dir, true)

				// Assert
				if tt.wantErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErr)
					return
				}
				require.NoError(t, err)
				require.Len(t, got, 1)
				var pkg map[string]any
				require.NoError(t, json.Unmarshal(got[0].RawContent, &pkg))
				tt.check(t, pkg)
			})
		}
	}
}
