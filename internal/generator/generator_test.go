package generator_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai_rules/internal/config"
	"github.com/Goldziher/ai_rules/internal/generator"
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
			{File: "CLAUDE.md"},
			{File: filepath.Join("rules", ".cursorrules")},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{Name: "Style Rule", Priority: 10, Content: "Use TypeScript strict mode"},
			{Name: "Testing Rule", Content: "Write unit tests for all functions"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Check that all files were created
	expectedFiles := []string{
		filepath.Join(tmpDir, "CLAUDE.md"),
		filepath.Join(tmpDir, "rules", ".cursorrules"),
		filepath.Join(tmpDir, ".windsurfrules"),
	}

	for _, file := range expectedFiles {
		t.Run(filepath.Base(file), func(t *testing.T) {
			// Check file exists
			_, err := os.Stat(file)
			assert.NoError(t, err, "File %s should exist", file)

			// Check file has content
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
			{File: outputFile},
			{File: "other.md"},
		},
		Rules: []config.Rule{
			{Name: "Test Rule", Content: "Test content"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateOutput(cfg, outputFile)
	require.NoError(t, err)

	// Check that only the specified file was created
	_, err = os.Stat(filepath.Join(tmpDir, outputFile))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, "other.md"))
	assert.True(t, os.IsNotExist(err), "Other file should not exist")

	// Check content
	content, err := os.ReadFile(filepath.Join(tmpDir, outputFile))
	require.NoError(t, err)
	assert.Contains(t, string(content), "Single Output Test")
	assert.Contains(t, string(content), "Test Rule")
}

func TestGenerator_GenerateOutput_FileNotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Outputs: []config.Output{
			{File: "existing.md"},
		},
	}

	gen := generator.New()
	err := gen.GenerateOutput(cfg, "nonexistent.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output file nonexistent.md not found")
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
			{File: outputFile, Template: "custom"},
		},
		Rules: []config.Rule{
			{Name: "Test Rule", Content: "Test content"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)

	// Register custom template
	customTemplate := "Custom: {{.ProjectName}} has {{.RuleCount}} rules"
	err := gen.RegisterTemplate("custom", customTemplate)
	require.NoError(t, err)

	// Generate with custom template
	err = gen.GenerateOutput(cfg, outputFile)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, outputFile))
	require.NoError(t, err)
	assert.Equal(t, "Custom: Custom Template Test has 1 rules", string(content))
}

func TestGenerator_PreviewOutput(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Preview Test",
		},
		Outputs: []config.Output{
			{File: "preview.md"},
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
			{File: "existing.md"},
		},
	}

	gen := generator.New()
	_, err := gen.PreviewOutput(cfg, "nonexistent.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output file nonexistent.md not found")
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
	assert.Contains(t, err.Error(), "no outputs defined")
}

func TestGenerator_DirectoryCreation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	deepPath := filepath.Join("deep", "nested", "path", "file.md")

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "Directory Test"},
		Outputs:  []config.Output{{File: deepPath}},
		Rules:    []config.Rule{{Name: "Test", Content: "Content"}},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Check that file was created and directories exist
	_, err = os.Stat(filepath.Join(tmpDir, deepPath))
	assert.NoError(t, err)

	// Check that directories were created
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
			{File: outputFile, Template: "test-vars"},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Content: "Content 2"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)

	// Register template that uses all variables
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
