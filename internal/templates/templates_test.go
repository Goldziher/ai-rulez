package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func TestRenderer_Render(t *testing.T) {
	renderer := templates.NewRenderer()

	t.Run("render default template", func(t *testing.T) {
		cfg := &config.Config{
			Metadata: config.Metadata{
				Name: "Test Project",
			},
			Rules: []config.Rule{
				{Name: "Rule1", Content: "Content1", Priority: config.PriorityMedium},
			},
		}

		data := templates.NewTemplateData(cfg, nil)

		output, err := renderer.Render("default", data)
		require.NoError(t, err)
		assert.Contains(t, output, "Test Project")
		assert.Contains(t, output, "Rule1")
		assert.Contains(t, output, "Content1")
	})

	t.Run("render minimal template", func(t *testing.T) {
		data := &templates.TemplateData{
			Rules: []config.Rule{
				{Name: "Rule1", Content: "Content1"},
			},
		}

		output, err := renderer.Render("minimal", data)
		require.NoError(t, err)
		assert.Contains(t, output, "Content1")
	})

	t.Run("render inline template", func(t *testing.T) {
		template := "Project: {{.ProjectName}}\nRules: {{len .Rules}}"
		data := &templates.TemplateData{
			ProjectName: "Test",
			Rules: []config.Rule{
				{Name: "R1", Content: "C1"},
				{Name: "R2", Content: "C2"},
			},
		}

		output, err := templates.RenderString(template, data)
		require.NoError(t, err)
		assert.Contains(t, output, "Project: Test")
		assert.Contains(t, output, "Rules: 2")
	})

	t.Run("unknown template format", func(t *testing.T) {
		renderer := templates.NewRenderer()
		_, err := renderer.Render("nonexistent", &templates.TemplateData{})
		assert.Error(t, err)
	})
}

func TestNewTemplateData(t *testing.T) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Test Project",
			Description: "Test Description",
			Version:     "1.0.0",
		},
		Rules: []config.Rule{
			{Name: "Rule1", Content: "Content1", Priority: config.PriorityMedium},
		},
		Sections: []config.Section{
			{Name: "Section1", Content: "Section content", Priority: config.PriorityLow},
		},
		Agents: []config.Agent{
			{Name: "Agent1", Description: "Agent desc", Priority: config.PriorityHigh},
		},
	}

	data := templates.NewTemplateData(cfg, nil)

	assert.Equal(t, "Test Project", data.ProjectName)
	assert.Equal(t, "Test Description", data.Description)
	assert.Equal(t, "1.0.0", data.Version)
	assert.Len(t, data.Rules, 1)
	assert.Len(t, data.Sections, 1)
	assert.Len(t, data.Agents, 1)
}

func TestNewTemplateDataForOutput(t *testing.T) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Test",
		},
		Rules: []config.Rule{
			{Name: "All", Targets: []string{}},
			{Name: "Claude", Targets: []string{"claude.md"}},
			{Name: "Other", Targets: []string{"other.md"}},
		},
	}

	outputPath := "claude.md"

	data := templates.NewTemplateDataForOutput(cfg, outputPath, nil)

	assert.Len(t, data.Rules, 2)
	assert.Equal(t, "claude.md", data.OutputFile)
}

func TestBuiltinTemplates(t *testing.T) {
	renderer := templates.NewRenderer()
	formats := renderer.GetSupportedFormats()

	assert.Contains(t, formats, "default")
	assert.Contains(t, formats, "minimal")
	assert.Contains(t, formats, "documentation")

	data := &templates.TemplateData{
		ProjectName: "Test",
		Rules:       []config.Rule{{Name: "R1", Content: "C1"}},
	}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			output, err := renderer.Render(format, data)
			assert.NoError(t, err)
			assert.NotEmpty(t, output)
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	t.Run("valid template", func(t *testing.T) {
		template := "{{.ProjectName}} - {{range .Rules}}{{.Name}}{{end}}"
		err := templates.ValidateTemplate(template)
		assert.NoError(t, err)
	})

	t.Run("invalid template", func(t *testing.T) {
		template := "{{.ProjectName} - missing closing"
		err := templates.ValidateTemplate(template)
		assert.Error(t, err)
	})
}
