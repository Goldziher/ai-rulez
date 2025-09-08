package agents

import (
	"os"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/templates"
)

func TestWriteIncrementalUpdate(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "ai-rulez-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory for test
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Setup test context and provider config
	context := &ProjectContext{
		ProjectName: "TestProject",
		RootPath:    tempDir,
	}
	providerConfig := templates.ProviderConfig{
		Claude: true,
		Cursor: true,
	}

	tests := []struct {
		name         string
		phase        int
		output       string
		wantContains []string
	}{
		{
			name:   "Phase 1 creates initial config",
			phase:  1,
			output: "description: A test project for unit testing\nThis is a comprehensive test project.",
			wantContains: []string{
				"$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json",
				"metadata:",
				"name: \"TestProject\"",
				"version: \"1.0.0\"",
				"outputs:",
				"- path: \"CLAUDE.md\"",
				"- path: \".cursor/rules/\"",
			},
		},
		{
			name:  "Phase 2 adds sections",
			phase: 2,
			output: `sections:
  - name: "Project Structure"
    priority: high
    content: |
      This is the project structure section.`,
			wantContains: []string{
				"sections:",
				"- name: \"Project Structure\"",
				"priority: high",
			},
		},
	}

	// Run tests sequentially to build layers
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeIncrementalUpdate(tt.phase, tt.output, context, providerConfig)
			if err != nil {
				t.Fatalf("writeIncrementalUpdate() error = %v", err)
			}

			// Read the config file and check contents
			content, err := os.ReadFile("ai_rulez.yaml")
			if err != nil {
				t.Fatalf("Failed to read ai_rulez.yaml: %v", err)
			}

			configStr := string(content)
			for _, want := range tt.wantContains {
				if !strings.Contains(configStr, want) {
					t.Errorf("Config file should contain %q, but doesn't. Content:\n%s", want, configStr)
				}
			}
		})
	}
}

func TestBuildInitialConfig(t *testing.T) {
	context := &ProjectContext{
		ProjectName: "TestProject",
	}

	tests := []struct {
		name           string
		providerConfig templates.ProviderConfig
		phaseOutput    string
		wantContains   []string
	}{
		{
			name: "Claude and Cursor config",
			providerConfig: templates.ProviderConfig{
				Claude: true,
				Cursor: true,
			},
			phaseOutput: "description: A test project\nThis project does testing.",
			wantContains: []string{
				"$schema:",
				"name: \"TestProject\"",
				"version: \"1.0.0\"",
				"- path: \"CLAUDE.md\"",
				"- path: \".cursor/rules/\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildInitialConfig(context, tt.providerConfig, tt.phaseOutput)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("buildInitialConfig() should contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}

func TestReadExistingConfig(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "ai-rulez-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory for test
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	t.Run("No config file exists", func(t *testing.T) {
		result := readExistingConfig()
		if result != "" {
			t.Errorf("readExistingConfig() should return empty string when no file exists, got %q", result)
		}
	})

	t.Run("Config file exists", func(t *testing.T) {
		testContent := "metadata:\n  name: \"TestProject\""
		err := os.WriteFile("ai_rulez.yaml", []byte(testContent), 0o644)
		if err != nil {
			t.Fatal(err)
		}

		result := readExistingConfig()
		if result != testContent {
			t.Errorf("readExistingConfig() should return file content, got %q, want %q", result, testContent)
		}
	})
}

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		name     string
		outputs  []string
		expected string
	}{
		{
			name:     "extract from description field",
			outputs:  []string{"Some text\ndescription: My awesome project\nMore text"},
			expected: "My awesome project",
		},
		{
			name:     "extract with quotes",
			outputs:  []string{"description: \"Project with quotes\""},
			expected: "Project with quotes",
		},
		{
			name:     "no description",
			outputs:  []string{"Some text without description"},
			expected: "",
		},
		{
			name:     "empty outputs",
			outputs:  []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDescription(tt.outputs)
			if result != tt.expected {
				t.Errorf("extractDescription() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractYAMLSection(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		startMarker string
		endMarkers  []string
		expected    string
	}{
		{
			name: "extract rules section",
			content: `metadata:
  name: test
rules:
  - name: "Rule 1"
    content: "Content 1"
  - name: "Rule 2"
    content: "Content 2"
sections:
  - name: "Section 1"`,
			startMarker: "rules:",
			endMarkers:  []string{"sections:", "agents:"},
			expected: `  - name: "Rule 1"
    content: "Content 1"
  - name: "Rule 2"
    content: "Content 2"`,
		},
		{
			name: "extract sections at end",
			content: `rules:
  - name: "Rule 1"
sections:
  - name: "Section 1"
    content: "Content 1"
  - name: "Section 2"
    content: "Content 2"`,
			startMarker: "sections:",
			endMarkers:  []string{"rules:", "agents:"},
			expected: `  - name: "Section 1"
    content: "Content 1"
  - name: "Section 2"
    content: "Content 2"`,
		},
		{
			name:        "section not found",
			content:     "metadata:\n  name: test",
			startMarker: "rules:",
			endMarkers:  []string{"sections:"},
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractYAMLSection(tt.content, tt.startMarker, tt.endMarkers...)
			if result != tt.expected {
				t.Errorf("extractYAMLSection() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMergePhaseOutputs(t *testing.T) {
	outputs := []string{
		"description: Test project for unit testing",
		`sections:
  - name: "Project Structure"
    priority: high
    content: |
      This is the structure`,
		`rules:
  - name: "Development"
    priority: high
    content: |
      Follow TDD`,
		`agents:
  - name: "test-agent"
    description: "Test agent"
    priority: high`,
		`commands:
  - name: "test"
    command: "go test"
    description: "Run tests"`,
	}

	context := &ProjectContext{
		ProjectName: "TestProject",
		RepoType:    "single",
	}

	providerConfig := templates.ProviderConfig{
		Claude: true,
	}

	result := mergePhaseOutputs(outputs, context, providerConfig)

	// Check metadata
	if !strings.Contains(result, `name: "TestProject"`) {
		t.Error("Result should contain project name")
	}

	if !strings.Contains(result, `description: "Test project for unit testing"`) {
		t.Error("Result should contain description")
	}

	// Check outputs for Claude
	if !strings.Contains(result, `path: "CLAUDE.md"`) {
		t.Error("Result should contain Claude output path")
	}

	// Check sections
	if !strings.Contains(result, "Project Structure") {
		t.Error("Result should contain sections")
	}

	// Check rules
	if !strings.Contains(result, "Development") {
		t.Error("Result should contain rules")
	}

	// Check agents
	if !strings.Contains(result, "test-agent") {
		t.Error("Result should contain agents")
	}

	// Check commands
	if !strings.Contains(result, "go test") {
		t.Error("Result should contain commands")
	}
}

func TestAddProviderContext(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		config        templates.ProviderConfig
		shouldContain string
	}{
		{
			name:   "single provider",
			prompt: "Base prompt",
			config: templates.ProviderConfig{
				Claude: true,
			},
			shouldContain: "Target AI Assistants: Claude",
		},
		{
			name:   "multiple providers",
			prompt: "Base prompt",
			config: templates.ProviderConfig{
				Claude: true,
				Cursor: true,
				Gemini: true,
			},
			shouldContain: "Target AI Assistants: Claude, Cursor, Gemini",
		},
		{
			name:          "no providers",
			prompt:        "Base prompt",
			config:        templates.ProviderConfig{},
			shouldContain: "Base prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addProviderContext(tt.prompt, tt.config)
			if !strings.Contains(result, tt.shouldContain) {
				t.Errorf("addProviderContext() should contain %q, got %q", tt.shouldContain, result)
			}
		})
	}
}

func TestExtractSections(t *testing.T) {
	output := `
sections:
  - name: "Section 1"
    priority: high
    content: |
      Content 1
  - name: "Section 2"
    priority: medium
    content: |
      Content 2
rules:
  - name: "Rule 1"`

	result := extractSections(output)

	if !strings.Contains(result, "Section 1") {
		t.Error("Should extract Section 1")
	}

	if !strings.Contains(result, "Section 2") {
		t.Error("Should extract Section 2")
	}

	if strings.Contains(result, "Rule 1") {
		t.Error("Should not include rules")
	}
}

func TestExtractRules(t *testing.T) {
	output := `
rules:
  - name: "Rule 1"
    priority: critical
    content: |
      Rule content
  - name: "Rule 2"
    priority: high
sections:
  - name: "Section 1"`

	result := extractRules(output)

	if !strings.Contains(result, "Rule 1") {
		t.Error("Should extract Rule 1")
	}

	if !strings.Contains(result, "Rule 2") {
		t.Error("Should extract Rule 2")
	}

	if strings.Contains(result, "Section 1") {
		t.Error("Should not include sections")
	}
}

func TestExtractAgents(t *testing.T) {
	output := `
agents:
  - name: "agent-1"
    description: "First agent"
    priority: high
    system_prompt: |
      You are an agent
  - name: "agent-2"
    description: "Second agent"
commands:
  - name: "test"`

	result := extractAgents(output)

	if !strings.Contains(result, "agent-1") {
		t.Error("Should extract agent-1")
	}

	if !strings.Contains(result, "agent-2") {
		t.Error("Should extract agent-2")
	}

	if strings.Contains(result, "commands") {
		t.Error("Should not include commands")
	}
}

func TestExtractCommands(t *testing.T) {
	output := `
commands:
  - name: "build"
    command: "go build"
    description: "Build project"
  - name: "test"
    command: "go test"
mcp_servers:
  - name: "server1"`

	result := extractCommands(output)

	if !strings.Contains(result, "build") {
		t.Error("Should extract build command")
	}

	if !strings.Contains(result, "go test") {
		t.Error("Should extract test command")
	}

	if strings.Contains(result, "mcp_servers") {
		t.Error("Should not include mcp_servers")
	}
}

func TestExtractMCPServers(t *testing.T) {
	output := `
mcp_servers:
  - name: "ai-rulez"
    command: "uvx ai-rulez"
    description: "AI-Rulez MCP server"
commands:
  - name: "test"`

	result := extractMCPServers(output)

	if !strings.Contains(result, "ai-rulez") {
		t.Error("Should extract ai-rulez server")
	}

	if !strings.Contains(result, "uvx ai-rulez") {
		t.Error("Should extract server command")
	}

	if strings.Contains(result, "commands") {
		t.Error("Should not include commands")
	}
}
