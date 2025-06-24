package templates_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naamanhirschfeld/ai_rules/internal/config"
	"github.com/naamanhirschfeld/ai_rules/internal/templates"
)

func TestNewTemplateData(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Test Project",
			Version:     "1.0.0",
			Description: "Test description",
		},
		Rules: []config.Rule{
			{Name: "rule1", Content: "content1"},
			{Name: "rule2", Content: "content2"},
		},
	}

	data := templates.NewTemplateData(cfg)

	assert.Equal(t, "Test Project", data.ProjectName)
	assert.Equal(t, "1.0.0", data.Version)
	assert.Equal(t, "Test description", data.Description)
	assert.Len(t, data.Rules, 2)
	assert.Equal(t, 2, data.RuleCount)
	assert.WithinDuration(t, time.Now(), data.Timestamp, time.Second)
}

func TestRenderer_Render(t *testing.T) {
	t.Parallel()

	renderer := templates.NewRenderer()
	data := &templates.TemplateData{
		ProjectName: "Test Project",
		Version:     "1.0.0",
		Description: "Test description",
		Rules: []config.Rule{
			{Name: "Style Rules", Priority: "high", Content: "Use TypeScript strict mode"},
			{Name: "Testing Rules", Content: "Write unit tests"},
		},
		Timestamp: time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC),
		RuleCount: 2,
	}

	tests := []struct {
		name   string
		format string
		checks []string // Strings that should be present in output
	}{
		{
			name:   "default format",
			format: "default",
			checks: []string{
				"# Test Project",
				"Test description",
				"Version: 1.0.0",
				"Generated on 2023-12-25 10:30:00",
				"Total rules: 2",
				"## Style Rules",
				"**Priority:** high",
				"Use TypeScript strict mode",
				"## Testing Rules",
				"Write unit tests",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := renderer.Render(tt.format, data)
			require.NoError(t, err)

			for _, check := range tt.checks {
				assert.Contains(t, result, check, "Output should contain: %s", check)
			}
		})
	}
}

func TestRenderer_UnknownFormat(t *testing.T) {
	t.Parallel()

	renderer := templates.NewRenderer()
	data := &templates.TemplateData{}

	_, err := renderer.Render("unknown", data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown template format")
}

func TestRenderer_RegisterTemplate(t *testing.T) {
	t.Parallel()

	renderer := templates.NewRenderer()

	// Register a custom template
	customTemplate := "Custom: {{.ProjectName}} ({{.RuleCount}} rules)"
	err := renderer.RegisterTemplate("custom", customTemplate)
	require.NoError(t, err)

	// Test rendering with custom template
	data := &templates.TemplateData{
		ProjectName: "Test",
		RuleCount:   3,
	}

	result, err := renderer.Render("custom", data)
	require.NoError(t, err)
	assert.Equal(t, "Custom: Test (3 rules)", result)
}

func TestRenderer_RegisterTemplate_InvalidSyntax(t *testing.T) {
	t.Parallel()

	renderer := templates.NewRenderer()

	err := renderer.RegisterTemplate("invalid", "{{.Invalid}")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template")
}

func TestRenderer_GetSupportedFormats(t *testing.T) {
	t.Parallel()

	renderer := templates.NewRenderer()
	formats := renderer.GetSupportedFormats()

	expectedFormats := []string{"default"}
	for _, expected := range expectedFormats {
		assert.Contains(t, formats, expected)
	}
}

func TestValidateTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "valid template",
			template:    "Hello {{.ProjectName}}!",
			expectError: false,
		},
		{
			name:        "invalid template - unclosed action",
			template:    "Hello {{.ProjectName}",
			expectError: true,
		},
		{
			name:        "invalid template - bad syntax",
			template:    "Hello {{range .}}",
			expectError: true,
		},
		{
			name:        "empty template",
			template:    "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := templates.ValidateTemplate(tt.template)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRenderString(t *testing.T) {
	t.Parallel()

	data := &templates.TemplateData{
		ProjectName: "Test Project",
		RuleCount:   5,
	}

	tests := []struct {
		name     string
		template string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple substitution",
			template: "Project: {{.ProjectName}}",
			expected: "Project: Test Project",
			wantErr:  false,
		},
		{
			name:     "multiple variables",
			template: "{{.ProjectName}} has {{.RuleCount}} rules",
			expected: "Test Project has 5 rules",
			wantErr:  false,
		},
		{
			name:     "invalid template",
			template: "{{.Invalid}",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := templates.RenderString(tt.template, data)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBuiltinTemplates_NoErrors(t *testing.T) {
	t.Parallel()

	// Test that all builtin templates can be rendered without errors
	renderer := templates.NewRenderer()
	data := &templates.TemplateData{
		ProjectName: "Test",
		Version:     "1.0.0",
		Description: "Description",
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: "high", Content: "Content 1"},
		},
		Timestamp: time.Now(),
		RuleCount: 1,
	}

	formats := []string{"default"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			result, err := renderer.Render(format, data)
			assert.NoError(t, err)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, "Content 1")
		})
	}
}

func TestBuiltinTemplates_EmptyRules(t *testing.T) {
	t.Parallel()

	// Test templates with no rules
	renderer := templates.NewRenderer()
	data := &templates.TemplateData{
		ProjectName: "Empty Project",
		Rules:       []config.Rule{},
		Timestamp:   time.Now(),
		RuleCount:   0,
	}

	formats := []string{"default"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			result, err := renderer.Render(format, data)
			assert.NoError(t, err)
			// Should not panic and should produce some output
			assert.NotEmpty(t, strings.TrimSpace(result))
		})
	}
}