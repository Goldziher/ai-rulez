package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)



// TestSetDefaultPriorities tests the priority defaulting logic
func TestSetDefaultPriorities(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Rules: []Rule{
			{Name: "Rule1", Priority: 0},
			{Name: "Rule2", Priority: 5},
			{Name: "Rule3", Priority: 0},
		},
		Sections: []Section{
			{Title: "Section1", Priority: 0},
			{Title: "Section2", Priority: 3},
		},
		Agents: []Agent{
			{Name: "agent1", Priority: 0},
			{Name: "agent2", Priority: 7},
		},
	}

	setDefaultPriorities(cfg)

	// Check that zero priorities were set to 1
	assert.Equal(t, 1, cfg.Rules[0].Priority)
	assert.Equal(t, 5, cfg.Rules[1].Priority)
	assert.Equal(t, 1, cfg.Rules[2].Priority)
	assert.Equal(t, 1, cfg.Sections[0].Priority)
	assert.Equal(t, 3, cfg.Sections[1].Priority)
	assert.Equal(t, 1, cfg.Agents[0].Priority)
	assert.Equal(t, 7, cfg.Agents[1].Priority)
}

// TestValidate_ComplexValidation tests complex validation scenarios
func TestValidate_ComplexValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "rule with id and duplicate check",
			config: &Config{
				Metadata: Metadata{Name: "Test"},
				Outputs:  []Output{{Path: "test.md"}},
				Rules: []Rule{
					{ID: "rule1", Name: "Rule 1", Content: "Content"},
					{ID: "rule1", Name: "Rule 2", Content: "Content"},
				},
			},
			wantErr: false, // IDs can be duplicate (for overriding)
		},
		{
			name: "section with priority edge case",
			config: &Config{
				Metadata: Metadata{Name: "Test"},
				Outputs:  []Output{{Path: "test.md"}},
				Sections: []Section{
					{Title: "Section", Priority: 999, Content: "Content"},
				},
			},
			wantErr: false,
		},
		{
			name: "agent with special characters in name",
			config: &Config{
				Metadata: Metadata{Name: "Test"},
				Outputs:  []Output{{Path: "test.md"}},
				Agents: []Agent{
					{Name: "agent-with-dashes", Description: "Valid"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLoadConfigWithIncludes_ComplexPaths tests include resolution with complex paths
func TestLoadConfigWithIncludes_ComplexPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "configs")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	// Create main config in subdir
	mainConfig := `metadata:
  name: "Main"
  version: "1.0.0"
includes:
  - "../shared.yaml"
  - "./local.yaml"
outputs:
  - path: "output.md"`

	// Create shared config in parent
	sharedConfig := `metadata:
  name: "Shared"
  version: "1.0.0"
outputs:
  - path: "shared.md"
rules:
  - name: "Shared Rule"
    content: "Shared content"`

	// Create local config in same dir
	localConfig := `metadata:
  name: "Local"
  version: "1.0.0"
outputs:
  - path: "local.md"
agents:
  - name: "local-agent"
    description: "Local agent"`

	mainPath := filepath.Join(subDir, "main.yaml")
	sharedPath := filepath.Join(tmpDir, "shared.yaml")
	localPath := filepath.Join(subDir, "local.yaml")

	require.NoError(t, os.WriteFile(mainPath, []byte(mainConfig), 0o644))
	require.NoError(t, os.WriteFile(sharedPath, []byte(sharedConfig), 0o644))
	require.NoError(t, os.WriteFile(localPath, []byte(localConfig), 0o644))

	cfg, err := LoadConfigWithIncludes(mainPath)
	require.NoError(t, err)

	assert.Equal(t, "Main", cfg.Metadata.Name)
	assert.Len(t, cfg.Rules, 1)
	assert.Len(t, cfg.Agents, 1)
	assert.Empty(t, cfg.Includes)
}



// TestValidateIncludes tests include validation
func TestValidateIncludes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create valid include file
	validConfig := `metadata:
  name: "Valid"
  version: "1.0.0"
outputs:
  - path: "test.md"`

	validPath := filepath.Join(tmpDir, "valid.yaml")
	err := os.WriteFile(validPath, []byte(validConfig), 0o644)
	require.NoError(t, err)

	// Test with valid includes
	cfg := &Config{
		Includes: []string{"valid.yaml"},
	}
	err = ValidateIncludes(cfg, tmpDir)
	assert.NoError(t, err)

	// Test with non-existent include
	cfg.Includes = append(cfg.Includes, "missing.yaml")
	err = ValidateIncludes(cfg, tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestValidateOutputs tests output validation
func TestValidateOutputs(t *testing.T) {
	t.Parallel()

	// Test with valid outputs
	outputs := []Output{
		{Path: "test.md"},
		{File: "legacy.md"},
		{Path: "agents/", Type: "agent"},
	}
	err := ValidateOutputs(outputs)
	assert.NoError(t, err)

	// Test with empty outputs
	err = ValidateOutputs([]Output{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required field 'outputs' is missing")

	// Test with invalid output (no file or path)
	outputs = []Output{{}}
	err = ValidateOutputs(outputs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path or outputs[0].file' is missing")
}

// TestMergeSections tests section merging
func TestMergeSections(t *testing.T) {
	t.Parallel()

	sections1 := []Section{
		{ID: "s1", Title: "Section 1", Content: "Original"},
		{Title: "Section 2", Content: "Content 2"},
	}

	sections2 := []Section{
		{ID: "s1", Title: "Section 1", Content: "Overridden"},
		{Title: "Section 3", Content: "Content 3"},
	}

	merged := MergeSections(sections1, sections2)

	assert.Len(t, merged, 3)
	// Find section with ID "s1"
	var s1 *Section
	for i := range merged {
		if merged[i].ID == "s1" {
			s1 = &merged[i]
			break
		}
	}
	require.NotNil(t, s1)
	assert.Equal(t, "Overridden", s1.Content)
}

// TestSaveConfig tests configuration saving
func TestSaveConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "save-test.yaml")

	cfg := &Config{
		Metadata: Metadata{
			Name:        "Save Test",
			Version:     "1.0.0",
			Description: "Testing save functionality",
		},
		Outputs: []Output{
			{Path: "output.md"},
		},
		Rules: []Rule{
			{Name: "Test Rule", Priority: 5, Content: "Test content"},
		},
		Agents: []Agent{
			{Name: "test-agent", Description: "Test agent", Priority: 3},
		},
	}

	err := SaveConfig(cfg, configPath)
	require.NoError(t, err)

	// Load it back and verify
	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, cfg.Metadata.Name, loaded.Metadata.Name)
	assert.Equal(t, cfg.Metadata.Version, loaded.Metadata.Version)
	assert.Len(t, loaded.Rules, 1)
	assert.Len(t, loaded.Agents, 1)
	assert.Equal(t, cfg.Rules[0].Name, loaded.Rules[0].Name)
	assert.Equal(t, cfg.Agents[0].Name, loaded.Agents[0].Name)
}

// TestSaveConfig_Errors tests error conditions in SaveConfig
func TestSaveConfig_Errors(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Metadata: Metadata{Name: "Test"},
		Outputs:  []Output{{Path: "test.md"}},
	}

	// Test with invalid path
	err := SaveConfig(cfg, "/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

// TestCircularIncludeDetection tests circular include detection
func TestCircularIncludeDetection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create config A that includes B
	configA := `metadata:
  name: "Config A"
  version: "1.0.0"
includes:
  - "b.yaml"
outputs:
  - path: "a.md"`

	// Create config B that includes A (circular)
	configB := `metadata:
  name: "Config B"
  version: "1.0.0"
includes:
  - "a.yaml"
outputs:
  - path: "b.md"`

	pathA := filepath.Join(tmpDir, "a.yaml")
	pathB := filepath.Join(tmpDir, "b.yaml")

	require.NoError(t, os.WriteFile(pathA, []byte(configA), 0o644))
	require.NoError(t, os.WriteFile(pathB, []byte(configB), 0o644))

	_, err := LoadConfigWithIncludes(pathA)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular include")
}

// TestResolvePath tests path resolution
func TestResolvePath(t *testing.T) {
	t.Parallel()

	loader := &configLoader{}

	// Test with absolute path
	absPath := "/absolute/path/file.yaml"
	resolved := loader.resolvePath(absPath, "/base/dir")
	assert.Equal(t, absPath, resolved)

	// Test with relative path
	relPath := "relative/file.yaml"
	baseDir := "/base/dir"
	resolved = loader.resolvePath(relPath, baseDir)
	assert.Equal(t, filepath.Join(baseDir, relPath), resolved)

	// Test with current directory reference
	resolved = loader.resolvePath("./file.yaml", baseDir)
	assert.Equal(t, filepath.Join(baseDir, "file.yaml"), resolved)

	// Test with parent directory reference
	resolved = loader.resolvePath("../file.yaml", "/base/sub/dir")
	assert.Equal(t, "/base/sub/file.yaml", resolved)
}


// TestSchemaEmbedding tests that schema is properly embedded
func TestSchemaEmbedding(t *testing.T) {
	t.Parallel()

	// schemaJSON should not be empty
	assert.NotEmpty(t, schemaJSON)
	assert.Contains(t, schemaJSON, "ai-rules-v1")
	assert.Contains(t, schemaJSON, "metadata")
	assert.Contains(t, schemaJSON, "outputs")
	assert.Contains(t, schemaJSON, "agents")

	// Verify it's valid JSON
	var schemaMap map[string]interface{}
	err := yaml.Unmarshal([]byte(schemaJSON), &schemaMap)
	assert.NoError(t, err)
	assert.Contains(t, schemaMap, "$schema")
}