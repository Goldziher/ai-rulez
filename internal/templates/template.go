// Package templates provides template rendering for ai_rules output generation.
package templates

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Goldziher/airules/internal/config"
)

// ContentItem represents a unified content item (rule or section).
type ContentItem struct {
	Type     string // "rule" or "section"
	Title    string // Name for rules, Title for sections
	Priority int
	Content  string
	IsRule   bool
}

// TemplateData contains all variables available for template substitution.
type TemplateData struct {
	ProjectName  string
	Version      string
	Description  string
	Rules        []config.Rule
	Sections     []config.Section
	AllContent   []ContentItem // Rules and sections combined and sorted
	Timestamp    time.Time
	RuleCount    int
	SectionCount int
}

// NewTemplateData creates template data from a config.
func NewTemplateData(cfg *config.Config) *TemplateData {
	// Create a copy of rules and sections to sort
	sortedRules := make([]config.Rule, len(cfg.Rules))
	copy(sortedRules, cfg.Rules)

	sortedSections := make([]config.Section, len(cfg.Sections))
	copy(sortedSections, cfg.Sections)

	// Create unified content list
	allContent := make([]ContentItem, 0, len(cfg.Rules)+len(cfg.Sections))

	// Add rules
	for _, rule := range cfg.Rules {
		allContent = append(allContent, ContentItem{
			Type:     "rule",
			Title:    rule.Name,
			Priority: rule.Priority,
			Content:  rule.Content,
			IsRule:   true,
		})
	}

	// Add sections
	for _, section := range cfg.Sections {
		allContent = append(allContent, ContentItem{
			Type:     "section",
			Title:    section.Title,
			Priority: section.Priority,
			Content:  section.Content,
			IsRule:   false,
		})
	}

	// Sort all content by priority (descending) then by title (ascending)
	sortContent(allContent)

	// Sort individual lists for backward compatibility
	sortRulesByPriority(sortedRules)
	sortSectionsByPriority(sortedSections)

	return &TemplateData{
		ProjectName:  cfg.Metadata.Name,
		Version:      cfg.Metadata.Version,
		Description:  cfg.Metadata.Description,
		Rules:        sortedRules,
		Sections:     sortedSections,
		AllContent:   allContent,
		Timestamp:    time.Now(),
		RuleCount:    len(cfg.Rules),
		SectionCount: len(cfg.Sections),
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
{{- if or .RuleCount .SectionCount}}

Total content: {{.RuleCount}} rules, {{.SectionCount}} sections
{{- end}}
{{- range .AllContent}}
{{- if .IsRule}}

## {{.Title}}

**Priority:** {{.Priority}}

{{.Content}}
{{- else}}

{{.Content}}
{{- end}}
{{- end}}
`

	// Documentation template - more detailed format
	documentationTemplate := `# {{.ProjectName}} - Detailed Rules

**Project Information:**
- Name: {{.ProjectName}}
{{- if .Version}}
- Version: {{.Version}}
{{- end}}
{{- if .Description}}
- Description: {{.Description}}
{{- end}}
- Generated: {{.Timestamp.Format "January 2, 2006 at 3:04 PM"}}
- Total Rules: {{.RuleCount}}

---

## Content

All content is sorted by priority (highest first), then alphabetically by title.

{{range .AllContent}}
{{- if .IsRule}}
### [Rule] {{.Title}} (Priority: {{.Priority}})
{{.Content}}
{{- else}}
{{.Content}}
{{- end}}

{{end}}
`

	// Register built-in templates (ignore errors since they're hardcoded and valid)
	_ = r.RegisterTemplate("default", defaultTemplate)
	_ = r.RegisterTemplate("documentation", documentationTemplate)
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

// sortRulesByPriority sorts rules by priority (descending) then by name (ascending).
func sortRulesByPriority(rules []config.Rule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].Name < rules[j].Name
	})
}

// sortSectionsByPriority sorts sections by priority (descending) then by title (ascending).
func sortSectionsByPriority(sections []config.Section) {
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Priority != sections[j].Priority {
			return sections[i].Priority > sections[j].Priority
		}
		return sections[i].Title < sections[j].Title
	})
}

// sortContent sorts content items by priority (descending) then by title (ascending).
func sortContent(items []ContentItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		return items[i].Title < items[j].Title
	})
}
