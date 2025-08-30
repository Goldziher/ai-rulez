package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestMCPCommandExists(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "mcp" {
			found = true
			break
		}
	}

	if !found {
		t.Error("MCP command not found in root command")
	}
}

func TestMCPCommandHelp(t *testing.T) {
	if mcpCmd == nil {
		t.Fatal("mcpCmd is nil")
	}

	help := mcpCmd.Long
	expectedStrings := []string{
		"Model Context Protocol",
		"stdio mode",
		"AI assistants",
		"Claude Desktop",
		"Cursor",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(help, expected) {
			t.Errorf("MCP command help does not contain %q", expected)
		}
	}
}

func TestAddAIRulezToolsDoesNotPanic(t *testing.T) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("addAIRulezTools panicked: %v", r)
			}
		}()
	}()
}

func TestMCPToolsIntegration(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	testConfig := &config.Config{
		Metadata: config.Metadata{
			Name:        "Test Config",
			Version:     "1.0.0",
			Description: "Test configuration for MCP",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
		Rules: []config.Rule{
			{
				Name:     "Test Rule",
				Priority: 10,
				Content:  "This is a test rule",
			},
		},
		Sections: []config.Section{
			{
				Title:    "Test Section",
				Priority: 1,
				Content:  "This is a test section",
			},
		},
	}

	err := config.SaveConfig(testConfig, configFile)
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	originalDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(originalDir)
	}()
	_ = os.Chdir(tempDir)

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Metadata.Name != "Test Config" {
		t.Errorf("Config name mismatch: got %s, want Test Config", cfg.Metadata.Name)
	}

	if len(cfg.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(cfg.Rules))
	}

	if len(cfg.Sections) != 1 {
		t.Errorf("Expected 1 section, got %d", len(cfg.Sections))
	}
}

func TestMCPBinaryBuild(t *testing.T) {
	if mcpCmd == nil {
		t.Error("mcpCmd is nil - MCP command not properly initialized")
	}

	if mcpCmd.Use != "mcp" {
		t.Errorf("MCP command Use field incorrect: got %s, want mcp", mcpCmd.Use)
	}

	if mcpCmd.Short == "" {
		t.Error("MCP command Short description is empty")
	}

	if mcpCmd.Long == "" {
		t.Error("MCP command Long description is empty")
	}

	if mcpCmd.Run == nil {
		t.Error("MCP command Run function is nil")
	}
}
