package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConvertV2ToV3MCPServer(t *testing.T) {
	tests := []struct {
		name     string
		v2Server config.MCPServer
		validate func(t *testing.T, v3 config.MCPServerV3)
	}{
		{
			name: "basic MCP server",
			v2Server: config.MCPServer{
				ID:        "test-server",
				Name:      "Test MCP Server",
				Command:   "npx",
				Args:      []string{"-y", "test@latest", "mcp"},
				Transport: "stdio",
			},
			validate: func(t *testing.T, v3 config.MCPServerV3) {
				assert.Equal(t, "Test MCP Server", v3.Name)
				assert.Equal(t, "npx", v3.Command)
				assert.Equal(t, []string{"-y", "test@latest", "mcp"}, v3.Args)
				assert.Equal(t, "stdio", v3.Transport)
			},
		},
		{
			name: "MCP server with environment variables",
			v2Server: config.MCPServer{
				Name:    "Server with Env",
				Command: "python",
				Args:    []string{"-m", "server"},
				Env: map[string]string{
					"DEBUG": "true",
					"PORT":  "3000",
				},
			},
			validate: func(t *testing.T, v3 config.MCPServerV3) {
				assert.Equal(t, "Server with Env", v3.Name)
				assert.Equal(t, "python", v3.Command)
				assert.NotNil(t, v3.Env)
				assert.Equal(t, "true", v3.Env["DEBUG"])
				assert.Equal(t, "3000", v3.Env["PORT"])
			},
		},
		{
			name: "MCP server with description",
			v2Server: config.MCPServer{
				Name:        "Documented Server",
				Description: "A test server with documentation",
				Command:     "node",
			},
			validate: func(t *testing.T, v3 config.MCPServerV3) {
				assert.Equal(t, "Documented Server", v3.Name)
				assert.Equal(t, "A test server with documentation", v3.Description)
				assert.Equal(t, "node", v3.Command)
			},
		},
		{
			name: "MCP server with URL transport",
			v2Server: config.MCPServer{
				Name:      "URL Server",
				Transport: "http",
				URL:       "http://localhost:3000",
			},
			validate: func(t *testing.T, v3 config.MCPServerV3) {
				assert.Equal(t, "URL Server", v3.Name)
				assert.Equal(t, "http", v3.Transport)
				assert.Equal(t, "http://localhost:3000", v3.URL)
			},
		},
		{
			name: "MCP server with enabled flag",
			v2Server: config.MCPServer{
				Name:    "Disabled Server",
				Command: "test",
				Enabled: boolPtr(false),
			},
			validate: func(t *testing.T, v3 config.MCPServerV3) {
				assert.Equal(t, "Disabled Server", v3.Name)
				assert.NotNil(t, v3.Enabled)
				assert.False(t, *v3.Enabled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v3Server := convertV2ToV3MCPServer(&tt.v2Server)
			tt.validate(t, v3Server)
		})
	}
}

func TestMigrateMCPServersEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Should not error when no servers to migrate
	err := MigrateMCPServers([]config.MCPServer{}, tmpDir)
	assert.NoError(t, err)

	// Should not create mcp.yaml when no servers
	mcpPath := filepath.Join(tmpDir, "mcp.yaml")
	assert.NoFileExists(t, mcpPath)
}

func TestMigrateMCPServers(t *testing.T) {
	tmpDir := t.TempDir()

	v2Servers := []config.MCPServer{
		{
			Name:      "Claude MCP",
			Command:   "npx",
			Args:      []string{"-y", "ai-rulez@latest", "mcp"},
			Transport: "stdio",
		},
		{
			Name:        "Test Server",
			Command:     "python",
			Args:        []string{"-m", "test"},
			Description: "A test MCP server",
			Env: map[string]string{
				"DEBUG": "true",
			},
		},
	}

	// Migrate servers
	err := MigrateMCPServers(v2Servers, tmpDir)
	require.NoError(t, err)

	// Verify mcp.yaml was created
	mcpPath := filepath.Join(tmpDir, "mcp.yaml")
	assert.FileExists(t, mcpPath)

	// Read and validate mcp.yaml
	data, err := os.ReadFile(mcpPath)
	require.NoError(t, err)

	var mcpConfig config.MCPConfigV3
	err = yaml.Unmarshal(data, &mcpConfig)
	require.NoError(t, err)

	// Verify structure
	assert.Equal(t, "1.0", mcpConfig.Version)
	assert.NotEmpty(t, mcpConfig.Schema)
	assert.Len(t, mcpConfig.Servers, 2)

	// Verify first server
	assert.Equal(t, "Claude MCP", mcpConfig.Servers[0].Name)
	assert.Equal(t, "npx", mcpConfig.Servers[0].Command)
	assert.Equal(t, []string{"-y", "ai-rulez@latest", "mcp"}, mcpConfig.Servers[0].Args)
	assert.Equal(t, "stdio", mcpConfig.Servers[0].Transport)

	// Verify second server
	assert.Equal(t, "Test Server", mcpConfig.Servers[1].Name)
	assert.Equal(t, "python", mcpConfig.Servers[1].Command)
	assert.Equal(t, "A test MCP server", mcpConfig.Servers[1].Description)
	assert.Equal(t, "true", mcpConfig.Servers[1].Env["DEBUG"])
}

func TestMigrateMCPServersWithTargets(t *testing.T) {
	tmpDir := t.TempDir()

	// Servers with targets are currently ignored (all go to root)
	// This test verifies that targets don't break the migration
	v2Servers := []config.MCPServer{
		{
			Name:    "Server with Targets",
			Command: "test",
			Targets: []string{"CLAUDE.md", ".cursor/rules/*"},
		},
	}

	err := MigrateMCPServers(v2Servers, tmpDir)
	require.NoError(t, err)

	// Verify server was migrated (targets not included in V3)
	mcpPath := filepath.Join(tmpDir, "mcp.yaml")
	data, err := os.ReadFile(mcpPath)
	require.NoError(t, err)

	var mcpConfig config.MCPConfigV3
	err = yaml.Unmarshal(data, &mcpConfig)
	require.NoError(t, err)

	assert.Len(t, mcpConfig.Servers, 1)
	assert.Equal(t, "Server with Targets", mcpConfig.Servers[0].Name)
}

// Helper function to create boolean pointers
func boolPtr(b bool) *bool {
	return &b
}
