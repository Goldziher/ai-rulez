package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPServers(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".ai-rulez")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create minimal config.yaml
	configContent := `version: "3.0"
name: "Test Project"
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Create mcp.yaml
	mcpContent := `version: "3.0"
mcp_servers:
  - name: github
    description: GitHub integration
    command: npx
    args:
      - "-y"
      - "@modelcontextprotocol/server-github"
    env:
      GITHUB_TOKEN: "test"
    transport: stdio
    enabled: true
  - name: postgres
    description: PostgreSQL
    command: uvx
    args:
      - "mcp-server-postgres"
    transport: stdio
    enabled: true
`
	if err := os.WriteFile(filepath.Join(configDir, "mcp.yaml"), []byte(mcpContent), 0644); err != nil {
		t.Fatalf("Failed to write mcp.yaml: %v", err)
	}

	// Test loading root MCP servers
	servers, err := loadMCPServers(configDir)
	if err != nil {
		t.Fatalf("Failed to load MCP servers: %v", err)
	}

	if len(servers) != 2 {
		t.Errorf("Expected 2 MCP servers, got %d", len(servers))
	}

	// Check github server
	if github, ok := servers["github"]; ok {
		if github.Description != "GitHub integration" {
			t.Errorf("Expected github description 'GitHub integration', got %q", github.Description)
		}
		if github.Command != "npx" {
			t.Errorf("Expected github command 'npx', got %q", github.Command)
		}
		if !github.IsEnabled() {
			t.Errorf("Expected github to be enabled")
		}
	} else {
		t.Errorf("GitHub server not found")
	}

	// Check postgres server
	if postgres, ok := servers["postgres"]; ok {
		if postgres.Description != "PostgreSQL" {
			t.Errorf("Expected postgres description 'PostgreSQL', got %q", postgres.Description)
		}
		if postgres.Command != "uvx" {
			t.Errorf("Expected postgres command 'uvx', got %q", postgres.Command)
		}
	} else {
		t.Errorf("Postgres server not found")
	}
}

func TestLoadDomainMCPServers(t *testing.T) {
	// Create a temporary domain directory
	tmpDir := t.TempDir()
	domainDir := tmpDir

	// Create domain mcp.yaml
	mcpContent := `version: "3.0"
mcp_servers:
  - name: backend-api
    description: Backend API MCP
    command: node
    args:
      - "mcp.js"
    transport: stdio
    enabled: true
`
	if err := os.WriteFile(filepath.Join(domainDir, "mcp.yaml"), []byte(mcpContent), 0644); err != nil {
		t.Fatalf("Failed to write domain mcp.yaml: %v", err)
	}

	// Test loading domain MCP servers
	servers, err := loadDomainMCPServers(domainDir)
	if err != nil {
		t.Fatalf("Failed to load domain MCP servers: %v", err)
	}

	if len(servers) != 1 {
		t.Errorf("Expected 1 MCP server in domain, got %d", len(servers))
	}

	if api, ok := servers["backend-api"]; ok {
		if api.Description != "Backend API MCP" {
			t.Errorf("Expected description 'Backend API MCP', got %q", api.Description)
		}
	} else {
		t.Errorf("backend-api server not found")
	}
}

func TestServersToMap(t *testing.T) {
	servers := []MCPServerV3{
		{Name: "server1", Description: "First"},
		{Name: "server2", Description: "Second"},
	}

	result := serversToMap(servers)

	if len(result) != 2 {
		t.Errorf("Expected 2 servers in map, got %d", len(result))
	}

	if srv, ok := result["server1"]; !ok || srv.Description != "First" {
		t.Errorf("server1 not correctly mapped")
	}

	if srv, ok := result["server2"]; !ok || srv.Description != "Second" {
		t.Errorf("server2 not correctly mapped")
	}
}

func TestMergeMCPServersV3(t *testing.T) {
	root := map[string]*MCPServerV3{
		"github":   {Name: "github", Description: "GitHub root"},
		"postgres": {Name: "postgres", Description: "PostgreSQL root"},
	}

	domain := map[string]*MCPServerV3{
		"postgres": {Name: "postgres", Description: "PostgreSQL domain override"},
		"backend":  {Name: "backend", Description: "Backend specific"},
	}

	result := mergeMCPServersV3(root, domain)

	if len(result) != 3 {
		t.Errorf("Expected 3 servers after merge, got %d", len(result))
	}

	// Check that domain overrides root
	if postgres, ok := result["postgres"]; ok {
		if postgres.Description != "PostgreSQL domain override" {
			t.Errorf("Expected postgres to be overridden, got %q", postgres.Description)
		}
	} else {
		t.Errorf("postgres not found after merge")
	}

	// Check that root server is preserved
	if github, ok := result["github"]; ok {
		if github.Description != "GitHub root" {
			t.Errorf("Expected github to be preserved, got %q", github.Description)
		}
	} else {
		t.Errorf("github not found after merge")
	}

	// Check that domain-only server is added
	if backend, ok := result["backend"]; ok {
		if backend.Description != "Backend specific" {
			t.Errorf("Expected backend specific, got %q", backend.Description)
		}
	} else {
		t.Errorf("backend not found after merge")
	}
}

func TestLoadConfigV3WithMCP(t *testing.T) {
	// Create a complete test fixture
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".ai-rulez")
	domainsDir := filepath.Join(configDir, "domains")
	backendDir := filepath.Join(domainsDir, "backend")

	// Create directory structure
	for _, dir := range []string{configDir, filepath.Join(configDir, "rules"), filepath.Join(configDir, "context"), filepath.Join(configDir, "skills"), backendDir, filepath.Join(backendDir, "rules")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Create config.yaml
	configContent := `version: "3.0"
name: "Test Project"
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Create root mcp.yaml
	rootMCPContent := `version: "3.0"
mcp_servers:
  - name: github
    description: GitHub
    command: npx
    transport: stdio
    enabled: true
`
	if err := os.WriteFile(filepath.Join(configDir, "mcp.yaml"), []byte(rootMCPContent), 0644); err != nil {
		t.Fatalf("Failed to write root mcp.yaml: %v", err)
	}

	// Create domain mcp.yaml
	domainMCPContent := `version: "3.0"
mcp_servers:
  - name: backend-db
    description: Backend Database
    command: docker
    transport: stdio
    enabled: true
`
	if err := os.WriteFile(filepath.Join(backendDir, "mcp.yaml"), []byte(domainMCPContent), 0644); err != nil {
		t.Fatalf("Failed to write domain mcp.yaml: %v", err)
	}

	// Load config
	cfg, err := LoadConfigV3(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify root MCP servers
	if len(cfg.MCPServers) != 1 {
		t.Errorf("Expected 1 root MCP server, got %d", len(cfg.MCPServers))
	}

	if github, ok := cfg.MCPServers["github"]; ok {
		if github.Description != "GitHub" {
			t.Errorf("Expected GitHub description, got %q", github.Description)
		}
	} else {
		t.Errorf("GitHub server not found in root")
	}

	// Verify domain MCP servers
	if domain, ok := cfg.Content.Domains["backend"]; ok {
		if len(domain.MCPServers) != 1 {
			t.Errorf("Expected 1 backend MCP server, got %d", len(domain.MCPServers))
		}

		if db, ok := domain.MCPServers["backend-db"]; ok {
			if db.Description != "Backend Database" {
				t.Errorf("Expected 'Backend Database' description, got %q", db.Description)
			}
		} else {
			t.Errorf("backend-db server not found in domain")
		}
	} else {
		t.Errorf("backend domain not found")
	}
}

func TestLoadConfigV3WithoutMCP(t *testing.T) {
	// Test that config loads successfully even without MCP files
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".ai-rulez")

	// Create minimal structure
	for _, dir := range []string{configDir, filepath.Join(configDir, "rules"), filepath.Join(configDir, "context"), filepath.Join(configDir, "skills")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Create config.yaml without mcp.yaml
	configContent := `version: "3.0"
name: "Test Project"
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Load config - should succeed with empty MCP servers
	cfg, err := LoadConfigV3(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify empty MCP servers
	if len(cfg.MCPServers) != 0 {
		t.Errorf("Expected 0 MCP servers when no mcp.yaml, got %d", len(cfg.MCPServers))
	}
}
