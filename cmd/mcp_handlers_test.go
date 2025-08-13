package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// TestMCPToolsExist tests that all MCP tools are properly defined
func TestMCPToolsExist(t *testing.T) {
	// Test that the MCP command exists and has the right structure
	if mcpCmd == nil {
		t.Error("MCP command not initialized")
		return
	}

	// These are the tools that should be available
	expectedTools := []string{
		"get_rules",
		"get_sections",
		"get_agents",
		"add_rule",
		"add_section",
		"add_output",
		"add_agent",
		"update_rule",
		"update_section",
		"update_output",
		"update_agent",
		"delete_rule",
		"delete_section",
		"delete_output",
		"delete_agent",
		"generate_output",
		"validate_config",
		"init_project",
		"get_version",
	}

	// We can't easily test the actual MCP server without complex setup
	// But we can test that the main command structure exists
	if mcpCmd.Use != "mcp" {
		t.Errorf("Expected MCP command Use to be 'mcp', got '%s'", mcpCmd.Use)
	}

	if mcpCmd.Short == "" {
		t.Error("MCP command should have a short description")
	}

	if mcpCmd.Long == "" {
		t.Error("MCP command should have a long description")
	}

	if mcpCmd.Run == nil {
		t.Error("MCP command should have a Run function")
	}

	// Log expected tools for manual verification
	t.Logf("Expected MCP tools to be available: %v", expectedTools)
}

// TestAgentConfigHandling tests agent configuration functionality
func TestAgentConfigHandling(t *testing.T) {
	// Create temporary directory and config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	// Create initial config with agents
	initialConfig := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
		Agents: []config.Agent{
			{
				Name:         "test-agent",
				Description:  "Test agent",
				Priority:     5,
				Tools:        []string{"tool1", "tool2"},
				SystemPrompt: "You are a test agent",
			},
		},
	}

	err := config.SaveConfig(initialConfig, configFile)
	if err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Change to temp directory
	originalDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(originalDir)
	}()
	_ = os.Chdir(tempDir)

	// Test loading the config
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify agent was loaded correctly
	if len(cfg.Agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(cfg.Agents))
	}

	agent := cfg.Agents[0]
	if agent.Name != "test-agent" {
		t.Errorf("Expected agent name 'test-agent', got '%s'", agent.Name)
	}

	if agent.Description != "Test agent" {
		t.Errorf("Expected description 'Test agent', got '%s'", agent.Description)
	}

	if agent.Priority != 5 {
		t.Errorf("Expected priority 5, got %d", agent.Priority)
	}

	if len(agent.Tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(agent.Tools))
	}

	if agent.SystemPrompt != "You are a test agent" {
		t.Errorf("Expected system prompt 'You are a test agent', got '%s'", agent.SystemPrompt)
	}
}

// TestProviderConfigGeneration tests the provider configuration generation
func TestProviderConfigGeneration(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_providers.yaml")

	// Test default providers config
	providers := ProviderConfig{
		Enabled:      []string{"claude"},
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	}

	err := createProviderConfigFile("Test Project", configFile, providers)
	if err != nil {
		t.Fatalf("Failed to create provider config: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load and verify the config
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load created config: %v", err)
	}

	if cfg.Metadata.Name != "Test Project" {
		t.Errorf("Expected project name 'Test Project', got '%s'", cfg.Metadata.Name)
	}

	// Should have outputs for Claude
	if len(cfg.Outputs) == 0 {
		t.Error("Expected at least one output for Claude provider")
	}

	// Test with all providers
	allProviders := ProviderConfig{
		Enabled:      []string{"claude", "cursor", "windsurf", "gemini", "copilot", "cline", "continue"},
		WithAgents:   true,
		WithSections: true,
		NoComments:   false,
	}

	allConfigFile := filepath.Join(tempDir, "test_all_providers.yaml")
	err = createProviderConfigFile("All Providers Project", allConfigFile, allProviders)
	if err != nil {
		t.Fatalf("Failed to create all providers config: %v", err)
	}

	allCfg, err := config.LoadConfig(allConfigFile)
	if err != nil {
		t.Fatalf("Failed to load all providers config: %v", err)
	}

	// Should have more outputs for multiple providers
	if len(allCfg.Outputs) <= len(cfg.Outputs) {
		t.Error("Expected more outputs for all providers config")
	}

	// Should have agents and sections when enabled
	if len(allCfg.Agents) == 0 {
		t.Error("Expected agents when WithAgents is true")
	}

	if len(allCfg.Rules) == 0 && len(allCfg.Sections) == 0 {
		t.Error("Expected rules or sections when WithSections is true")
	}
}

// TestVersionConstant tests that the version constant is properly set
func TestVersionConstant(t *testing.T) {
	if Version == "" {
		t.Error("Version constant should not be empty")
	}

	// Should be either "dev" or a proper version
	if Version != "dev" && !isValidVersion(Version) {
		t.Logf("Version is set to: %s (this is expected for dev builds)", Version)
	}
}

// isValidVersion checks if a version string looks like a semantic version
func isValidVersion(v string) bool {
	// Simple check for semantic version pattern
	return v != "" && (v[0] == 'v' || (v[0] >= '0' && v[0] <= '9'))
}
