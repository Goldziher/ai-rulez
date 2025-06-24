// Package templates provides template rendering for ai_rules output generation.
package templates

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/naamanhirschfeld/ai_rules/internal/config"
)

// TemplateData contains all variables available for template substitution.
type TemplateData struct {
	ProjectName  string
	Version      string
	Description  string
	Rules        []config.Rule
	Timestamp    time.Time
	RuleCount    int
}

// NewTemplateData creates template data from a config.
func NewTemplateData(cfg *config.Config) *TemplateData {
	return &TemplateData{
		ProjectName:  cfg.Metadata.Name,
		Version:      cfg.Metadata.Version,
		Description:  cfg.Metadata.Description,
		Rules:        cfg.Rules,
		Timestamp:    time.Now(),
		RuleCount:    len(cfg.Rules),
	}
}

// Renderer handles template rendering with different output formats.
type Renderer struct {
	templates map[string]*template.Template
}

// NewRenderer creates a new template renderer with built-in templates.
func NewRenderer() *Renderer {
	r := &Renderer{
		templates: make(map[string]*template.Template),
	}

	// Register built-in templates
	r.registerBuiltinTemplates()
	
	return r
}

// Render processes a template with the given data and returns the result.
func (r *Renderer) Render(format string, data *TemplateData) (string, error) {
	tmpl, exists := r.templates[format]
	if !exists {
		return "", fmt.Errorf("unknown template format: %s", format)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", format, err)
	}

	return buf.String(), nil
}

// RegisterTemplate adds a custom template for a format.
func (r *Renderer) RegisterTemplate(format, templateStr string) error {
	tmpl, err := template.New(format).Parse(templateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template for %s: %w", format, err)
	}

	r.templates[format] = tmpl
	return nil
}

// GetSupportedFormats returns all registered template formats.
func (r *Renderer) GetSupportedFormats() []string {
	formats := make([]string, 0, len(r.templates))
	for format := range r.templates {
		formats = append(formats, format)
	}
	return formats
}

// registerBuiltinTemplates sets up the default template.
func (r *Renderer) registerBuiltinTemplates() {
	// Default markdown template - works for all AI assistant formats
	defaultTemplate := `# {{.ProjectName}}
{{- if .Description}}

{{.Description}}
{{- end}}
{{- if .Version}}

Version: {{.Version}}
{{- end}}

Generated on {{.Timestamp.Format "2006-01-02 15:04:05"}}
{{- if .RuleCount}}

Total rules: {{.RuleCount}}
{{- end}}
{{- range .Rules}}

## {{.Name}}
{{- if .Priority}}

**Priority:** {{.Priority}}
{{- end}}

{{.Content}}
{{- end}}
`

	// Register default template (ignore error since it's hardcoded and valid)
	_ = r.RegisterTemplate("default", defaultTemplate)
}

// ValidateTemplate checks if a template string is valid.
func ValidateTemplate(templateStr string) error {
	_, err := template.New("validation").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}
	return nil
}

// RenderString is a utility function to render a template string directly.
func RenderString(templateStr string, data *TemplateData) (string, error) {
	tmpl, err := template.New("inline").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}