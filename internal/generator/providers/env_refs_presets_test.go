package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	refSecretEnv    = "s3cret-env-value"
	refSecretHeader = "s3cret-header-value"
)

// refServers is one stdio server with a secret env value and one remote server
// with an embedded bearer header, both resolved from the process environment.
func refServers() map[string]*config.MCPServer {
	return map[string]*config.MCPServer{
		"local": {
			Name: "local", Command: "npx", Args: []string{"-y", "pkg"},
			Env:           map[string]string{"API_TOKEN": refSecretEnv},
			SecretEnvKeys: []string{"API_TOKEN"},
			EnvRefs:       map[string]string{"API_TOKEN": "${API_TOKEN}"},
		},
		"remote": {
			Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp",
			Headers:          map[string]string{"Authorization": "Bearer " + refSecretHeader},
			SecretHeaderKeys: []string{"Authorization"},
			HeaderRefs:       map[string]string{"Authorization": "Bearer ${HDR_TOKEN}"},
		},
	}
}

// TestEnvReferences_PerPreset pins, per preset, which environment reference syntax
// its MCP document carries (issue #212): the tool's own syntax where its
// documentation says it expands one, the resolved value where it does not.
func TestEnvReferences_PerPreset(t *testing.T) {
	tests := []struct {
		preset     string
		path       string
		wantEnv    string // what must appear for the whole-value env placeholder
		wantHeader string // what must appear for the embedded header placeholder
	}{
		{"pi", ".pi/mcp.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		{"amp", ".amp/settings.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		{"factory", ".factory/mcp.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		{"kilo", "kilo.jsonc", "{env:API_TOKEN}", "Bearer {env:HDR_TOKEN}"},
		{"crush", "crush.json", "$API_TOKEN", "Bearer " + refSecretHeader}, // $NAME is whole-value only
		{"mcp", ".mcp.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		{"codebuddy", ".mcp.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		{"commandcode", ".mcp.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		{"reasonix", ".mcp.json", "${API_TOKEN}", "Bearer ${HDR_TOKEN}"},
		// No documented expansion: the resolved value stays.
		{"junie", ".junie/mcp/mcp.json", refSecretEnv, "Bearer " + refSecretHeader},
		{"zed", ".zed/settings.json", refSecretEnv, "Bearer " + refSecretHeader},
		{"trae", ".trae/mcp.json", refSecretEnv, "Bearer " + refSecretHeader},
		{"kiro", ".kiro/settings/mcp.json", refSecretEnv, "Bearer " + refSecretHeader},
		{"qoder", ".mcp.json", refSecretEnv, "Bearer " + refSecretHeader},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)
			cfg := &config.Config{
				Name: "demo", BaseDir: "/test", MCPServers: refServers(), Presets: []config.Preset{{BuiltIn: tt.preset}},
			}

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, "/test", cfg)
			require.NoError(t, err)

			// Assert
			body := requireFile(t, outputs, tt.path).Content
			assert.Contains(t, body, tt.wantEnv)
			assert.Contains(t, body, tt.wantHeader)
			if tt.wantEnv != refSecretEnv {
				assert.NotContains(t, body, refSecretEnv, "the secret must stay out of the file")
			}
			if tt.wantHeader != "Bearer "+refSecretHeader {
				assert.NotContains(t, body, refSecretHeader, "the secret must stay out of the file")
			}
		})
	}
}

// TestEnvReferences_SharedMCPJSONAgrees: every writer of the root .mcp.json must
// render the same bytes, so one writer whose tool does not expand references
// (Qoder) keeps the whole file resolved, and a name the strictest reader would not
// expand (CodeBuddy takes upper-case names only) is never written as a reference.
func TestEnvReferences_SharedMCPJSONAgrees(t *testing.T) {
	tests := []struct {
		name       string
		presets    []string
		envName    string
		wantBraced bool
	}{
		{"mcp alone writes references", []string{"mcp"}, "API_TOKEN", true},
		{"qoder among the writers keeps it resolved", []string{"mcp", "qoder"}, "API_TOKEN", false},
		{"lower-case name is not expanded by every reader", []string{"mcp"}, "api_token", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			servers := refServers()
			servers["local"].EnvRefs = map[string]string{"API_TOKEN": "${" + tt.envName + "}"}
			var presetList []config.Preset
			for _, p := range tt.presets {
				presetList = append(presetList, config.Preset{BuiltIn: p})
			}
			cfg := &config.Config{Name: "demo", BaseDir: "/test", MCPServers: servers, Presets: presetList}

			// Act: every active writer renders the document.
			var bodies []string
			for _, p := range tt.presets {
				gen, err := providers.LoadBuiltin(p)
				require.NoError(t, err)
				outputs, err := gen.Generate(&config.ContentTree{}, "/test", cfg)
				require.NoError(t, err)
				bodies = append(bodies, requireFile(t, outputs, ".mcp.json").Content)
			}

			// Assert
			for _, body := range bodies {
				assert.Equal(t, bodies[0], body, "writers of one path render identical content")
			}
			if tt.wantBraced {
				assert.Contains(t, bodies[0], "${"+tt.envName+"}")
				assert.NotContains(t, bodies[0], refSecretEnv)
			} else {
				assert.Contains(t, bodies[0], refSecretEnv)
			}
		})
	}
}
