package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeHeadersProject writes a project whose single remote MCP server sends
// headers, with the given presets and gitignore setting.
func writeHeadersProject(t *testing.T, presets string, gitignore bool, headers string) string {
	t.Helper()
	tempDir := t.TempDir()
	aiRulezDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "rules"), 0o755))
	body := "version = \"5.0\"\nname = \"mcp-headers\"\npresets = " + presets + "\n"
	if gitignore {
		body += "gitignore = true\n"
	} else {
		body += "gitignore = false\n"
	}
	body += "\n[[mcp_servers]]\nname = \"remote\"\ntransport = \"http\"\nurl = \"https://mcp.example.com/mcp\"\nheaders = " + headers + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.toml"), []byte(body), 0o644))
	return tempDir
}

func readJSONPath(t *testing.T, path string, keys ...string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	var node any
	require.NoError(t, json.Unmarshal(data, &node), "parse %s", path)
	for _, key := range keys {
		obj, ok := node.(map[string]any)
		require.True(t, ok, "%s: %v is not an object at %q", path, node, key)
		node = obj[key]
	}
	return node
}

func TestGenerator_MCPHeaders_RenderedForRemoteServers(t *testing.T) {
	// Arrange
	tempDir := writeHeadersProject(t,
		`["claude", "cursor", "copilot", "gemini", "opencode", "antigravity"]`, true,
		`{ Authorization = "Bearer ${API_TOKEN}", X-Team = "core" }`)
	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	cfg.MCPEnvOverrides = map[string]string{"API_TOKEN": "t0k"}

	// Act
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	// Assert
	want := map[string]any{"Authorization": "Bearer t0k", "X-Team": "core"}
	tests := []struct {
		file string
		path []string
	}{
		{".mcp.json", []string{"mcpServers", "remote", "headers"}},
		{".gemini/settings.json", []string{"mcpServers", "remote", "headers"}},
		{".agents/mcp_config.json", []string{"mcpServers", "remote", "headers"}},
		{"opencode.json", []string{"mcp", "remote", "headers"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Equal(t, want, readJSONPath(t, filepath.Join(tempDir, tt.file), tt.path...))
		})
	}
}

func TestGenerator_MCPHeaders_FailsUnresolvedPlaceholder(t *testing.T) {
	tempDir := writeHeadersProject(t, `["claude"]`, true, `{ Authorization = "Bearer ${MISSING_HEADER_TOKEN}" }`)
	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)

	err = NewGenerator(cfg).Generate("default")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved MCP env placeholders")
	assert.NoFileExists(t, filepath.Join(tempDir, ".mcp.json"))
}

func TestGenerator_MCPHeaders_SecretOutputMustBeGitignored(t *testing.T) {
	tests := []struct {
		name    string
		presets string
		headers string
		wantErr bool
	}{
		{name: "literal Authorization header", presets: `["claude"]`, headers: `{ Authorization = "Bearer abc" }`, wantErr: true},
		{name: "placeholder header", presets: `["claude"]`, headers: `{ X-Api = "${HDR_VALUE}" }`, wantErr: true},
		{name: "opencode.json is guarded too", presets: `["opencode"]`, headers: `{ Authorization = "Bearer abc" }`, wantErr: true},
		{name: "non-secret header", presets: `["claude"]`, headers: `{ X-Team = "core" }`, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := writeHeadersProject(t, tt.presets, false, tt.headers)
			cfg, err := config.LoadConfig(context.Background(), tempDir)
			require.NoError(t, err)
			cfg.MCPEnvOverrides = map[string]string{"HDR_VALUE": "v"}

			err = NewGenerator(cfg).Generate("default")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "contains secrets but is not gitignored")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Secret header values must not leak into, or perturb, the source hash.
func TestComputeSourceHash_RedactsSecretHeaders(t *testing.T) {
	t.Parallel()

	build := func(token string) *config.Config {
		cfg := &config.Config{
			Name: "x", Version: "5.0", BaseDir: t.TempDir(),
			Content: &config.ContentTree{},
			MCPServers: map[string]*config.MCPServer{
				"remote": {
					Name: "remote", Transport: config.TransportHTTP, URL: "https://mcp.example.com/mcp",
					Headers: map[string]string{"Authorization": "Bearer " + token},
				},
			},
		}
		require.NoError(t, (&Generator{ctx: t.Context(), config: cfg}).resolveMCPEnv())
		return cfg
	}

	assert.Equal(t,
		computeSourceHash(build("one"), &config.ContentTree{}),
		computeSourceHash(build("two"), &config.ContentTree{}))
}

// Plugin bundles are distributed and never carry headers, so generating one
// must not require header secrets to be resolvable.
func TestGeneratePlugin_DoesNotResolveMCPHeaders(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, filepath.Join("..", "..", "tests", "fixtures", "plugin", "basemind"), dir)
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(raw, []byte(`
[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${PLUGIN_UNSET_HEADER_TOKEN}" }
`)...), 0o644))

	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	gen := NewGenerator(cfg)

	require.NoError(t, gen.GeneratePlugin(""))
	require.NoError(t, gen.VerifyPlugin(""))
}

// A value is secret because it came from a placeholder; a second resolve pass
// sees the expanded value and must not forget that.
func TestResolveMCPEnv_KeepsSecretKeysAcrossPasses(t *testing.T) {
	cfg := &config.Config{
		Name: "x", Version: "5.0", BaseDir: t.TempDir(),
		MCPEnvOverrides: map[string]string{"TENANT": "acme"},
		MCPServers: map[string]*config.MCPServer{
			"remote": {
				Name: "remote", Transport: config.TransportHTTP, URL: "https://mcp.example.com/mcp",
				Env:     map[string]string{"TENANT_ID": "${TENANT}"},
				Headers: map[string]string{"X-Tenant": "${TENANT}"},
			},
		},
	}
	gen := &Generator{ctx: t.Context(), config: cfg}

	require.NoError(t, gen.resolveMCPEnv())
	require.NoError(t, gen.resolveMCPEnv())

	server := cfg.MCPServers["remote"]
	assert.Equal(t, []string{"TENANT_ID"}, server.SecretEnvKeys)
	assert.Equal(t, []string{"X-Tenant"}, server.SecretHeaderKeys)
}
