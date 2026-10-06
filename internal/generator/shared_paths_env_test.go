package generator

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const sharedEnvSecret = "ghp_sharedpathsSECRETvalue0123456789abcdef"

// renderSharedPathsWithEnvRef renders presets with one server whose env holds a
// ${GITHUB_TOKEN} placeholder resolved from the process environment, the way
// generate does, and returns the merged outputs with secret outputs flagged.
func renderSharedPathsWithEnvRef(t *testing.T, base *config.Config, presets []string) ([]config.OutputFile, error) {
	t.Helper()
	cfg := *base
	cfg.MCPServers = map[string]*config.MCPServer{
		"gh": {Command: "npx", Args: []string{"x"}, Env: map[string]string{"GITHUB_TOKEN": "${GITHUB_TOKEN}"}},
	}
	cfg.Presets = make([]config.Preset, len(presets))
	for i, name := range presets {
		cfg.Presets[i] = config.Preset{BuiltIn: name}
	}
	g := NewGenerator(&cfg)
	render, err := g.renderPresets("")
	require.NoError(t, err)
	flat, err := flattenPresetOutputs(nil, render.byPreset)
	if err != nil {
		return nil, err
	}
	g.markSensitiveOutputs(flat)
	return flat, nil
}

// TestSharedPaths_EnvRefNeverResolvedInSharedOutput renders every pair of presets
// that share a path with an environment-referenced secret. A pair either fails
// with a conflict naming a preset, or every output holding the resolved value is
// flagged sensitive (owner-only); no shared file silently carries the secret.
func TestSharedPaths_EnvRefNeverResolvedInSharedOutput(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", sharedEnvSecret)
	base := loadSharedPathsConfig(t, sharedPathsVariants[0])
	names := allBuiltinPresets()
	alone := make(map[string]map[string][]string, len(names))
	for _, name := range names {
		writers, _, err := renderSharedPaths(t, base, []string{name})
		require.NoError(t, err, name)
		alone[name] = writers
	}

	for i, a := range names {
		for _, b := range names[i+1:] {
			if !sharePath(alone[a], alone[b]) {
				continue
			}
			t.Run(a+"+"+b, func(t *testing.T) {
				t.Parallel()
				flat, err := renderSharedPathsWithEnvRef(t, base, []string{a, b})
				if err != nil {
					assert.Contains(t, err.Error(), ".mcp.json")
					return
				}
				for _, o := range flat {
					if strings.Contains(o.Content, sharedEnvSecret) || strings.Contains(string(o.RawContent), sharedEnvSecret) {
						assert.True(t, o.Sensitive, "%s holds the resolved secret without being sensitive", o.Path)
					}
				}
			})
		}
	}
}

// TestSharedPaths_QoderCannotShareReferencedMCPJSON pins that qoder, which
// documents no ${VAR} expansion, never has its resolved secret written into the
// .mcp.json another preset reads as a reference: generation fails naming qoder.
func TestSharedPaths_QoderCannotShareReferencedMCPJSON(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", sharedEnvSecret)
	base := loadSharedPathsConfig(t, sharedPathsVariants[0])
	tests := []struct {
		name    string
		presets []string
		wantErr bool
	}{
		{"claude+qoder", []string{"claude", "qoder"}, true},
		{"cursor+qoder", []string{"cursor", "qoder"}, true},
		{"qoder alone", []string{"qoder"}, false},
		{"qoder with unrelated preset", []string{"gemini", "qoder"}, false},
		{"claude alone", []string{"claude"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			flat, err := renderSharedPathsWithEnvRef(t, base, tt.presets)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "qoder")
				assert.Contains(t, err.Error(), ".mcp.json")
				assert.NotContains(t, err.Error(), sharedEnvSecret)
				return
			}
			require.NoError(t, err)
			for _, o := range flat {
				if o.Path != "" && strings.HasSuffix(o.Path, ".mcp.json") {
					holds := strings.Contains(o.Content, sharedEnvSecret)
					assert.Equal(t, slices.Contains(tt.presets, "qoder"), holds, "%s", o.Path)
					if holds {
						assert.True(t, o.Sensitive)
					}
				}
			}
		})
	}
}
