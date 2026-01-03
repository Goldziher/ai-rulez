package generator_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
)

func TestGenerator_GenerateAll(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Test Project",
			Version:     "1.0.0",
			Description: "Test description",
		},
		Outputs: []config.Output{
			{Path: "CLAUDE.md"},
			{Path: ".cursor/rules/", Type: "rule", NamingScheme: "rules.mdc"},
			{Path: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{Name: "Style Rule", Priority: config.PriorityCritical, Content: "Use TypeScript strict mode"},
			{Name: "Testing Rule", Content: "Write unit tests for all functions"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	expectedFiles := []string{
		filepath.Join(tmpDir, "CLAUDE.md"),
		filepath.Join(tmpDir, ".cursor", "rules", "rules.mdc"),
		filepath.Join(tmpDir, ".windsurfrules"),
	}

	for _, file := range expectedFiles {
		t.Run(filepath.Base(file), func(t *testing.T) {
			_, err := os.Stat(file)
			assert.NoError(t, err, "File %s should exist", file)

			content, err := os.ReadFile(file)
			require.NoError(t, err)

			contentStr := string(content)
			assert.Contains(t, contentStr, "Test Project")
			assert.Contains(t, contentStr, "Style Rule")
			assert.Contains(t, contentStr, "Use TypeScript strict mode")
			assert.Contains(t, contentStr, "Testing Rule")
			assert.Contains(t, contentStr, "Write unit tests for all functions")
		})
	}
}

func TestGenerator_GenerateOutput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := "output.md"

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Single Output Test",
		},
		Outputs: []config.Output{
			{Path: outputFile},
			{Path: "other.md"},
		},
		Rules: []config.Rule{
			{Name: "Test Rule", Content: "Test content"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateOutput(cfg, outputFile)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, outputFile))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, "other.md"))
	assert.True(t, os.IsNotExist(err), "Other file should not exist")

	content, err := os.ReadFile(filepath.Join(tmpDir, outputFile))
	require.NoError(t, err)
	assert.Contains(t, string(content), "Single Output Test")
	assert.Contains(t, string(content), "Test Rule")
}

func TestGenerator_GenerateOutput_FileNotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Outputs: []config.Output{
			{Path: "existing.md"},
		},
	}

	gen := generator.New()
	err := gen.GenerateOutput(cfg, "nonexistent.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output file not found in configuration")
}

func TestGenerator_CustomTemplate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := "custom.md"

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Custom Template Test",
		},
		Outputs: []config.Output{
			{Path: outputFile, Template: map[string]interface{}{
				"type":  "builtin",
				"value": "custom",
			}},
		},
		Rules: []config.Rule{
			{Name: "Test Rule", Content: "Test content"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)

	customTemplate := "Custom: {{.ProjectName}} has {{.RuleCount}} rules"
	err := gen.RegisterTemplate("custom", customTemplate)
	require.NoError(t, err)

	err = gen.GenerateOutput(cfg, outputFile)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, outputFile))
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY")
	assert.Contains(t, contentStr, "Custom: Custom Template Test has 1 rules")
}

func TestGenerator_PreviewOutput(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Preview Test",
		},
		Outputs: []config.Output{
			{Path: "preview.md"},
		},
		Rules: []config.Rule{
			{Name: "Preview Rule", Content: "Preview content"},
		},
	}

	gen := generator.New()
	content, err := gen.PreviewOutput(cfg, "preview.md")
	require.NoError(t, err)

	assert.Contains(t, content, "Preview Test")
	assert.Contains(t, content, "Preview Rule")
	assert.Contains(t, content, "Preview content")
}

func TestGenerator_PreviewOutput_FileNotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Outputs: []config.Output{
			{Path: "existing.md"},
		},
	}

	gen := generator.New()
	_, err := gen.PreviewOutput(cfg, "nonexistent.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output file not found in configuration")
}

func TestGenerator_RegisterTemplate_Invalid(t *testing.T) {
	t.Parallel()

	gen := generator.New()
	err := gen.RegisterTemplate("invalid", "{{.Invalid}")
	assert.Error(t, err)
}

func TestGenerator_ValidateTemplate(t *testing.T) {
	t.Parallel()

	gen := generator.New()

	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "valid template",
			template:    "{{.ProjectName}}",
			expectError: false,
		},
		{
			name:        "invalid template",
			template:    "{{.Invalid}",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := gen.ValidateTemplate(tt.template)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerator_GetSupportedTemplates(t *testing.T) {
	t.Parallel()

	gen := generator.New()
	templates := gen.GetSupportedTemplates()

	assert.Contains(t, templates, "default")
}

func TestGenerator_NoOutputs(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "No Outputs"},
		Outputs:  []config.Output{},
	}

	gen := generator.New()
	err := gen.GenerateAll(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outputs is required")
}

func TestGenerator_DirectoryCreation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	deepPath := filepath.Join("deep", "nested", "path", "file.md")

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "Directory Test"},
		Outputs:  []config.Output{{Path: deepPath}},
		Rules:    []config.Rule{{Name: "Test", Content: "Content"}},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, deepPath))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, filepath.Dir(deepPath)))
	assert.NoError(t, err)
}

func TestGenerator_TemplateVariables(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := "variables.md"

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Variable Test",
			Version:     "2.1.0",
			Description: "Testing all variables",
		},
		Outputs: []config.Output{
			{Path: outputFile, Template: map[string]interface{}{
				"type":  "builtin",
				"value": "test-vars",
			}},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: config.PriorityCritical, Content: "Content 1"},
			{Name: "Rule 2", Content: "Content 2"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)

	testTemplate := `Name: {{.ProjectName}}
Version: {{.Version}}
Description: {{.Description}}
Rule Count: {{.RuleCount}}
Timestamp: {{.Timestamp.Format "2006-01-02"}}
Rules:
{{- range .Rules}}
- {{.Name}}: {{.Content}}
{{- end}}`

	err := gen.RegisterTemplate("test-vars", testTemplate)
	require.NoError(t, err)

	err = gen.GenerateOutput(cfg, outputFile)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, outputFile))
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "Name: Variable Test")
	assert.Contains(t, contentStr, "Version: 2.1.0")
	assert.Contains(t, contentStr, "Description: Testing all variables")
	assert.Contains(t, contentStr, "Rule Count: 2")
	assert.Contains(t, contentStr, time.Now().Format("2006-01-02"))
	assert.Contains(t, contentStr, "- Rule 1: Content 1")
	assert.Contains(t, contentStr, "- Rule 2: Content 2")
}

func TestGenerator_HeaderGeneration(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := "test.md"

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Header Test",
			Version:     "1.0.0",
			Description: "Testing header generation",
		},
		Outputs: []config.Output{
			{Path: outputFile},
		},
		Rules: []config.Rule{
			{Name: "Test Rule", Content: "Test content"},
		},
	}

	configFile := filepath.Join(tmpDir, "test-config.yaml")
	gen := generator.NewWithConfigFile(configFile)

	err := gen.GenerateOutput(cfg, outputFile)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, outputFile))
	require.NoError(t, err)

	contentStr := string(content)

	expectedHeaderContent := []string{
		"<!--",
		"🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY",
		"Project: Header Test",
		"Source: .ai-rulez/test-config.yaml",
		"Target: test.md",
		"WHAT IS AI-RULEZ",
		"INSTRUCTIONS FOR AI AGENTS",
		"https://github.com/Goldziher/ai-rulez",
		"-->",
	}

	for _, expected := range expectedHeaderContent {
		assert.Contains(t, contentStr, expected, "Header should contain: %s", expected)
	}

	headerEndIndex := strings.Index(contentStr, "-->\n\n")
	contentStartIndex := strings.Index(contentStr, "# Header Test")
	assert.True(t, headerEndIndex < contentStartIndex, "Header should come before main content")

	assert.Contains(t, contentStr, "# Header Test")
	assert.Contains(t, contentStr, "## Test Rule")
}

func TestGenerator_HeaderInPreview(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := "preview.md"

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Preview Test",
		},
		Outputs: []config.Output{
			{Path: outputFile},
		},
		Rules: []config.Rule{
			{Name: "Preview Rule", Content: "Preview content"},
		},
	}

	configFile := filepath.Join(tmpDir, "preview-config.yaml")
	gen := generator.NewWithConfigFile(configFile)

	content, err := gen.PreviewOutput(cfg, outputFile)
	require.NoError(t, err)

	assert.Contains(t, content, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY")
	assert.Contains(t, content, "preview-config.yaml")
	assert.Contains(t, content, "preview.md")
	assert.Contains(t, content, "# Preview Test")
}

// TODO: Fix non-deterministic ordering in directory output generation

func TestGenerator_AgentFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Agent Test",
			Version:     "1.0.0",
			Description: "Testing agent generation",
		},
		Outputs: []config.Output{
			{
				Path: ".claude/agents/",
				Type: "agent",
			},
		},
		Agents: []config.Agent{
			{
				Name:         "code-reviewer",
				Description:  "Reviews code for quality",
				Priority:     config.PriorityCritical,
				Tools:        []string{"Read", "Write"},
				SystemPrompt: "You are a code reviewer",
			},
			{
				Name:        "test-writer",
				Description: "Writes unit tests",
				Priority:    config.PriorityMedium,
				Tools:       []string{"Read", "Write", "Execute"},
			},
			{
				Name:         "doc-generator",
				Description:  "Generates documentation",
				SystemPrompt: "Generate clear documentation",
			},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	dirPath := filepath.Join(tmpDir, ".claude", "agents")
	info, err := os.Stat(dirPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	agentFiles := map[string]struct {
		hasTools        bool
		hasSystemPrompt bool
		tools           []string
	}{
		"code-reviewer.md": {
			hasTools:        true,
			hasSystemPrompt: true,
			tools:           []string{"Read", "Write"},
		},
		"test-writer.md": {
			hasTools:        true,
			hasSystemPrompt: false,
			tools:           []string{"Read", "Write", "Execute"},
		},
		"doc-generator.md": {
			hasTools:        false,
			hasSystemPrompt: true,
		},
	}

	for filename, expected := range agentFiles {
		filePath := filepath.Join(dirPath, filename)
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "Agent file %s should exist", filename)

		contentStr := string(content)

		assert.Contains(t, contentStr, "---\n")
		assert.Contains(t, contentStr, "name:")
		assert.Contains(t, contentStr, "description:")

		if expected.hasTools {
			assert.Contains(t, contentStr, "tools:")
			for _, tool := range expected.tools {
				assert.Contains(t, contentStr, tool)
			}
		} else {
			assert.NotContains(t, contentStr, "tools:")
		}

		if expected.hasSystemPrompt {
			parts := strings.Split(contentStr, "---")
			assert.True(t, len(parts) >= 3, "Should have frontmatter delimiters")
			bodyContent := parts[2]
			assert.NotEmpty(t, strings.TrimSpace(bodyContent))
		}
	}
}

func TestGenerator_AgentWithSpecialCharacters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Special Chars Test",
		},
		Outputs: []config.Output{
			{
				Path: "agents/",
				Type: "agent",
			},
		},
		Agents: []config.Agent{
			{
				Name:         "yaml-special",
				Description:  `Agent with "quotes" and 'apostrophes' and: colons`,
				Tools:        []string{"Tool:Special", "Tool'With'Quote"},
				SystemPrompt: `Use "quotes" carefully: don't break YAML`,
			},
			{
				Name:        "multiline-agent",
				Description: "Agent with multiline content",
				SystemPrompt: `Line 1
Line 2 with "quotes"
Line 3: with colon`,
			},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	content1, err := os.ReadFile(filepath.Join(tmpDir, "agents", "yaml-special.md"))
	require.NoError(t, err)

	assert.Contains(t, string(content1), "description:")
	assert.Contains(t, string(content1), "tools:")

	parts := strings.Split(string(content1), "---")
	assert.GreaterOrEqual(t, len(parts), 3)

	content2, err := os.ReadFile(filepath.Join(tmpDir, "agents", "multiline-agent.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content2), "Line 1")
	assert.Contains(t, string(content2), "Line 2")
	assert.Contains(t, string(content2), "Line 3")
}

func TestGenerator_MixedOutputTypes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Mixed Output Test",
		},
		Outputs: []config.Output{
			{
				Path: "CLAUDE.md",
				Type: "rule",
			},
			{
				Path: ".claude/agents/",
				Type: "agent",
			},
			{
				Path: "docs/rules/",
				Type: "rule",
			},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Content: "Content 1"},
			{Name: "Rule 2", Content: "Content 2"},
		},
		Agents: []config.Agent{
			{Name: "agent1", Description: "First agent"},
			{Name: "agent2", Description: "Second agent"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, "CLAUDE.md"))
	assert.NoError(t, err)

	agentFiles, err := os.ReadDir(filepath.Join(tmpDir, ".claude", "agents"))
	require.NoError(t, err)
	assert.Len(t, agentFiles, 2)

	rulesDir := filepath.Join(tmpDir, "docs", "rules")
	_, err = os.Stat(rulesDir)
	assert.NoError(t, err)
}

func TestGenerator_EmptyAgents(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Empty Agents Test",
		},
		Outputs: []config.Output{
			{
				Path: "agents/",
				Type: "agent",
			},
		},
		Agents: []config.Agent{},
		Rules: []config.Rule{
			{Name: "Rule 1", Content: "Content 1"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	dirPath := filepath.Join(tmpDir, "agents")
	info, err := os.Stat(dirPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	files, err := os.ReadDir(dirPath)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestGenerator_CustomNamingScheme(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Custom Naming Test",
		},
		Outputs: []config.Output{
			{
				Path:         "agents/",
				Type:         "agent",
				NamingScheme: "agent-{index:03d}-{name}.yaml",
			},
		},
		Agents: []config.Agent{
			{Name: "reviewer", Description: "Reviews code"},
			{Name: "tester", Description: "Tests code"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	expectedFiles := []string{
		"agent-001-reviewer.yaml",
		"agent-002-tester.yaml",
	}

	for _, filename := range expectedFiles {
		filePath := filepath.Join(tmpDir, "agents", filename)
		_, err := os.Stat(filePath)
		assert.NoError(t, err, "File %s should exist", filename)
	}
}

func BenchmarkComputeContentHashPooled(b *testing.B) {
	content := "This is a test string that will be hashed repeatedly during the benchmark"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = generator.ComputeContentHashPooled(content)
	}
}

func BenchmarkGenerateAll(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Benchmark Project",
			Version:     "1.0.0",
			Description: "Test project for benchmarking",
		},
		Outputs: []config.Output{
			{Path: "output1.md"},
			{Path: "output2.md", Template: "documentation"},
			{Path: "output3.md"},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: config.PriorityCritical, Content: "Content 1"},
			{Name: "Rule 2", Priority: config.PriorityMedium, Content: "Content 2"},
			{Name: "Rule 3", Priority: config.PriorityMinimal, Content: "Content 3"},
		},
	}

	tempDir := b.TempDir()
	gen := generator.NewWithBaseDir(tempDir)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := gen.GenerateAll(cfg); err != nil {
			b.Fatalf("Failed to generate: %v", err)
		}
	}
}

func BenchmarkGenerateAllLarge(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Large Benchmark Project",
			Version:     "1.0.0",
			Description: "Test project with many outputs",
		},
		Outputs: make([]config.Output, 50),
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: config.PriorityCritical, Content: "Content 1"},
			{Name: "Rule 2", Priority: config.PriorityMedium, Content: "Content 2"},
			{Name: "Rule 3", Priority: config.PriorityMinimal, Content: "Content 3"},
		},
	}

	for i := 0; i < 50; i++ {
		cfg.Outputs[i] = config.Output{Path: fmt.Sprintf("output%d.md", i)}
	}

	tempDir := b.TempDir()
	gen := generator.NewWithBaseDir(tempDir)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := gen.GenerateAll(cfg); err != nil {
			b.Fatalf("Failed to generate: %v", err)
		}
	}
}
