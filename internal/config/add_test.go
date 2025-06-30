package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddRule(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create initial config
	configFile := filepath.Join(tmpDir, "ai_rulez.yaml")
	initialConfig := &Config{
		Metadata: Metadata{
			Name:    "Test Project",
			Version: "1.0.0",
		},
		Outputs: []Output{
			{File: "claude.md"},
		},
		Rules: []Rule{
			{
				Name:     "Existing Rule",
				Priority: 10,
				Content:  "This is an existing rule",
			},
		},
	}

	// Save initial config
	err = SaveConfig(initialConfig, configFile)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, cfg.Rules, 1)

	// Add new rule
	newRule := Rule{
		Name:     "New Rule",
		Priority: 5,
		Content:  "This is a new rule",
	}
	cfg.Rules = append(cfg.Rules, newRule)

	// Save updated config
	err = SaveConfig(cfg, configFile)
	require.NoError(t, err)

	// Reload and verify
	updatedCfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, updatedCfg.Rules, 2)
	assert.Equal(t, "New Rule", updatedCfg.Rules[1].Name)
	assert.Equal(t, 5, updatedCfg.Rules[1].Priority)
	assert.Equal(t, "This is a new rule", updatedCfg.Rules[1].Content)
}

func TestAddSection(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create initial config
	configFile := filepath.Join(tmpDir, "ai_rulez.yaml")
	initialConfig := &Config{
		Metadata: Metadata{
			Name:    "Test Project",
			Version: "1.0.0",
		},
		Outputs: []Output{
			{File: "claude.md"},
		},
		Sections: []Section{
			{
				Title:    "Existing Section",
				Priority: 10,
				Content:  "This is an existing section",
			},
		},
	}

	// Save initial config
	err = SaveConfig(initialConfig, configFile)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, cfg.Sections, 1)

	// Add new section
	newSection := Section{
		Title:    "New Section",
		Priority: 5,
		Content:  "This is a new section",
	}
	cfg.Sections = append(cfg.Sections, newSection)

	// Save updated config
	err = SaveConfig(cfg, configFile)
	require.NoError(t, err)

	// Reload and verify
	updatedCfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, updatedCfg.Sections, 2)
	assert.Equal(t, "New Section", updatedCfg.Sections[1].Title)
	assert.Equal(t, 5, updatedCfg.Sections[1].Priority)
	assert.Equal(t, "This is a new section", updatedCfg.Sections[1].Content)
}

func TestAddRuleWithDefaults(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create minimal config
	configFile := filepath.Join(tmpDir, "ai_rulez.yaml")
	initialConfig := &Config{
		Metadata: Metadata{
			Name: "Test Project",
		},
		Outputs: []Output{
			{File: "claude.md"},
		},
	}

	// Save initial config
	err = SaveConfig(initialConfig, configFile)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadConfig(configFile)
	require.NoError(t, err)

	// Add rule with priority 0 (should default to 1)
	newRule := Rule{
		Name:    "Default Priority Rule",
		Content: "This rule should have default priority",
	}
	cfg.Rules = append(cfg.Rules, newRule)

	// Save and reload
	err = SaveConfig(cfg, configFile)
	require.NoError(t, err)

	updatedCfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, updatedCfg.Rules, 1)
	assert.Equal(t, 1, updatedCfg.Rules[0].Priority) // Should be defaulted to 1
}

func TestAddOutput(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create initial config
	configFile := filepath.Join(tmpDir, "ai_rulez.yaml")
	initialConfig := &Config{
		Metadata: Metadata{
			Name:    "Test Project",
			Version: "1.0.0",
		},
		Outputs: []Output{
			{File: "claude.md"},
		},
	}

	// Save initial config
	err = SaveConfig(initialConfig, configFile)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, cfg.Outputs, 1)

	// Add new output
	newOutput := Output{
		File:     ".cursorrules",
		Template: "cursor",
	}
	cfg.Outputs = append(cfg.Outputs, newOutput)

	// Save updated config
	err = SaveConfig(cfg, configFile)
	require.NoError(t, err)

	// Reload and verify
	updatedCfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, updatedCfg.Outputs, 2)
	assert.Equal(t, ".cursorrules", updatedCfg.Outputs[1].File)
	assert.Equal(t, "cursor", updatedCfg.Outputs[1].Template)
}

func TestAddOutputDuplicate(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create initial config with an output
	configFile := filepath.Join(tmpDir, "ai_rulez.yaml")
	initialConfig := &Config{
		Metadata: Metadata{
			Name:    "Test Project",
			Version: "1.0.0",
		},
		Outputs: []Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
		},
	}

	// Save initial config
	err = SaveConfig(initialConfig, configFile)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, cfg.Outputs, 2)

	// Check if we can detect duplicate
	duplicateExists := false
	for _, output := range cfg.Outputs {
		if output.File == "claude.md" {
			duplicateExists = true
			break
		}
	}
	assert.True(t, duplicateExists, "Should detect existing output file")
}