package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Config represents the ai_rulez.yaml structure for testing
type Config struct {
	Schema   string   `yaml:"$schema"`
	Metadata Metadata `yaml:"metadata"`
	Outputs  []Output `yaml:"outputs"`
	Agents   []Agent  `yaml:"agents,omitempty"`
	Rules    []Rule   `yaml:"rules"`
	Sections []Section `yaml:"sections,omitempty"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type Output struct {
	File         string `yaml:"file,omitempty"`
	Path         string `yaml:"path,omitempty"`
	Type         string `yaml:"type,omitempty"`
	NamingScheme string `yaml:"naming_scheme,omitempty"`
}

type Agent struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	SystemPrompt string `yaml:"system_prompt"`
}

type Rule struct {
	Name     string `yaml:"name"`
	Priority int    `yaml:"priority"`
	Content  string `yaml:"content"`
}

type Section struct {
	Title    string `yaml:"title"`
	Priority int    `yaml:"priority"`
	Content  string `yaml:"content"`
}

func TestInitProviders(t *testing.T) {
	// Build the binary
	binPath := buildBinary(t)

	tests := []struct {
		name            string
		args            []string
		expectedOutputs []string
		hasAgents       bool
		hasSections     bool
		noComments      bool
		wantErr         bool
	}{
		{
			name:            "default_claude_with_agents",
			args:            []string{"TestDefault"},
			expectedOutputs: []string{"CLAUDE.md", ".claude/agents/"},
			hasAgents:       true,
		},
		{
			name:            "minimal_claude_only",
			args:            []string{"TestMinimal", "--minimal"},
			expectedOutputs: []string{"CLAUDE.md"},
			hasAgents:       false,
			hasSections:     false,
		},
		{
			name:            "popular_providers",
			args:            []string{"TestPopular", "--popular"},
			expectedOutputs: []string{"CLAUDE.md", ".cursor/rules/", ".windsurfrules", ".github/copilot-instructions.md"},
		},
		{
			name: "all_providers",
			args: []string{"TestAll", "--all"},
			expectedOutputs: []string{
				"CLAUDE.md", ".cursor/rules/", ".windsurfrules", 
				"GEMINI.md", ".github/copilot-instructions.md",
				".clinerules", ".continuerules",
			},
		},
		{
			name:            "specific_providers",
			args:            []string{"TestSpecific", "--claude", "--cursor", "--gemini"},
			expectedOutputs: []string{"CLAUDE.md", ".cursor/rules/", "GEMINI.md", ".claude/agents/"},
			hasAgents:       true,
		},
		{
			name:            "with_agents",
			args:            []string{"TestAgents", "--claude", "--with-agents"},
			expectedOutputs: []string{"CLAUDE.md", ".claude/agents/"},
			hasAgents:       true,
		},
		{
			name:            "with_sections",
			args:            []string{"TestSections", "--with-sections"},
			expectedOutputs: []string{"CLAUDE.md", ".claude/agents/"},
			hasAgents:       true,
			hasSections:     true,
		},
		{
			name:            "no_comments",
			args:            []string{"TestNoComments", "--minimal", "--no-comments"},
			expectedOutputs: []string{"CLAUDE.md"},
			noComments:      true,
		},
		{
			name:            "cursor_windsurf_only", 
			args:            []string{"TestEditors", "--cursor", "--windsurf"},
			expectedOutputs: []string{".cursor/rules/", ".windsurfrules"},
			hasAgents:       false,
		},
		{
			name: "all_with_everything",
			args: []string{"TestFull", "--all", "--with-agents", "--with-sections"},
			expectedOutputs: []string{
				"CLAUDE.md", ".cursor/rules/", ".windsurfrules",
				"GEMINI.md", ".github/copilot-instructions.md",
				".clinerules", ".continuerules", ".claude/agents/",
			},
			hasAgents:   true,
			hasSections: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()
			
			// Run init command
			args := append([]string{"init"}, tt.args...)
			cmd := exec.Command(binPath, args...)
			cmd.Dir = tempDir
			
			output, err := cmd.CombinedOutput()
			
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
				return
			}
			
			require.NoError(t, err, "Command failed: %s", string(output))
			
			// Verify ai_rulez.yaml exists
			configPath := filepath.Join(tempDir, "ai_rulez.yaml")
			require.FileExists(t, configPath)
			
			// Load and parse config
			data, err := os.ReadFile(configPath)
			require.NoError(t, err)
			
			var cfg Config
			err = yaml.Unmarshal(data, &cfg)
			require.NoError(t, err)
			
			// Check schema
			assert.Equal(t, "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json", cfg.Schema)
			
			// Check outputs
			for _, expected := range tt.expectedOutputs {
				found := false
				for _, output := range cfg.Outputs {
					path := output.Path
					if path == "" {
						path = output.File
					}
					if strings.Contains(path, expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected output %s not found in config", expected)
			}
			
			// Check agents
			if tt.hasAgents {
				assert.NotEmpty(t, cfg.Agents, "Expected agents but found none")
			} else {
				assert.Empty(t, cfg.Agents, "Expected no agents but found some")
			}
			
			// Check sections
			if tt.hasSections {
				assert.NotEmpty(t, cfg.Sections, "Expected sections but found none")
			} else {
				assert.Empty(t, cfg.Sections, "Expected no sections but found some")
			}
			
			// Check comments
			if tt.noComments {
				assert.NotContains(t, string(data), "# Uncomment", "Should not contain comment instructions")
			}
			
			// Verify rules exist
			assert.NotEmpty(t, cfg.Rules, "Should have rules")
			assert.GreaterOrEqual(t, len(cfg.Rules), 5, "Should have at least 5 rules")
		})
	}
}

func TestInitExistingFile(t *testing.T) {
	binPath := buildBinary(t)
	tempDir := t.TempDir()
	
	// Create existing config
	configPath := filepath.Join(tempDir, "ai_rulez.yaml")
	err := os.WriteFile(configPath, []byte("# existing"), 0644)
	require.NoError(t, err)
	
	// Try to init (should fail)
	cmd := exec.Command(binPath, "init", "TestProject")
	cmd.Dir = tempDir
	
	output, err := cmd.CombinedOutput()
	assert.Error(t, err, "Should fail when file exists")
	assert.Contains(t, string(output), "already exists")
}

func TestInitAndGenerate(t *testing.T) {
	binPath := buildBinary(t)
	tempDir := t.TempDir()
	
	// Init with popular providers
	cmd := exec.Command(binPath, "init", "TestGen", "--popular", "--with-agents")
	cmd.Dir = tempDir
	
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Init failed: %s", string(output))
	
	// Run generate
	cmd = exec.Command(binPath, "generate")
	cmd.Dir = tempDir
	
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Generate failed: %s", string(output))
	
	// Check generated files
	expectedFiles := []string{
		"CLAUDE.md",
		".windsurfrules", 
		".github/copilot-instructions.md",
		".cursor/rules/rules.mdc",
	}
	
	for _, file := range expectedFiles {
		path := filepath.Join(tempDir, file)
		assert.FileExists(t, path, "Expected %s to be generated", file)
		
		// Verify files have content
		if info, err := os.Stat(path); err == nil {
			assert.Greater(t, info.Size(), int64(100), "File %s should have content", file)
		}
	}
	
	// Check agent files
	agentDir := filepath.Join(tempDir, ".claude/agents")
	entries, err := os.ReadDir(agentDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 2, "Should have generated at least 2 agent files")
}

// Helper function to build the binary
func buildBinary(t *testing.T) string {
	t.Helper()
	
	// Check if we're in CI or local
	binName := "ai-rulez-e2e-test"
	binPath := filepath.Join(t.TempDir(), binName)
	
	// Build from parent directory
	testDir, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")
	projectRoot := filepath.Dir(testDir)
	
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to build binary: %s", string(output))
	
	return binPath
}