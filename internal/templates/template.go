package templates

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
)

type ContentItem struct {
	Type     string
	Title    string
	Priority int
	Content  string
	IsRule   bool
}

type TemplateData struct {
	ProjectName  string
	Version      string
	Description  string
	Rules        []config.Rule
	Sections     []config.Section
	Agents       []config.Agent
	AllContent   []ContentItem
	Timestamp    time.Time
	RuleCount    int
	SectionCount int
	AgentCount   int
	ConfigFile   string
	OutputFile   string
}

func NewTemplateData(cfg *config.Config) *TemplateData {
	return NewTemplateDataForOutput(cfg, "")
}

func NewTemplateDataForOutput(cfg *config.Config, outputPath string) *TemplateData {
	allRules := cfg.Rules
	allSections := cfg.Sections
	allAgents := cfg.Agents

	if cfg.UserRulez != nil {
		allRules = config.MergeRules(allRules, cfg.UserRulez.Rules)
		allSections = config.MergeSections(allSections, cfg.UserRulez.Sections)
		allAgents = config.MergeAgents(allAgents, cfg.UserRulez.Agents)
	}

	if outputPath != "" {
		var err error
		allRules, err = config.FilterRules(allRules, outputPath, cfg.Targets)
		if err != nil {
			allRules = cfg.Rules
		}

		allSections, err = config.FilterSections(allSections, outputPath, cfg.Targets)
		if err != nil {
			allSections = cfg.Sections
		}

		allAgents, err = config.FilterAgents(allAgents, outputPath, cfg.Targets)
		if err != nil {
			allAgents = cfg.Agents
		}
	}

	sortedRules := make([]config.Rule, len(allRules))
	copy(sortedRules, allRules)

	sortedSections := make([]config.Section, len(allSections))
	copy(sortedSections, allSections)

	allContent := make([]ContentItem, 0, len(allRules)+len(allSections))

	for _, rule := range allRules {
		allContent = append(allContent, ContentItem{
			Type:     "rule",
			Title:    rule.Name,
			Priority: rule.Priority,
			Content:  rule.Content,
			IsRule:   true,
		})
	}

	for _, section := range allSections {
		allContent = append(allContent, ContentItem{
			Type:     "section",
			Title:    section.Title,
			Priority: section.Priority,
			Content:  section.Content,
			IsRule:   false,
		})
	}

	sortContent(allContent)
	sortRulesByPriority(sortedRules)
	sortSectionsByPriority(sortedSections)

	sortedAgents := make([]config.Agent, len(allAgents))
	copy(sortedAgents, allAgents)
	sortAgentsByPriority(sortedAgents)

	return &TemplateData{
		ProjectName:  cfg.Metadata.Name,
		Version:      cfg.Metadata.Version,
		Description:  cfg.Metadata.Description,
		Rules:        sortedRules,
		Sections:     sortedSections,
		Agents:       sortedAgents,
		AllContent:   allContent,
		Timestamp:    time.Now(),
		RuleCount:    len(allRules),
		SectionCount: len(allSections),
		AgentCount:   len(allAgents),
	}
}

type Renderer struct {
	templates map[string]*template.Template
}

func NewRenderer() *Renderer {
	r := &Renderer{
		templates: make(map[string]*template.Template),
	}

	r.registerBuiltinTemplates()

	return r
}

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

func (r *Renderer) RegisterTemplate(format, templateStr string) error {
	tmpl, err := template.New(format).Parse(templateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template for %s: %w", format, err)
	}

	r.templates[format] = tmpl
	return nil
}

func (r *Renderer) GetSupportedFormats() []string {
	formats := make([]string, 0, len(r.templates))
	for format := range r.templates {
		formats = append(formats, format)
	}
	return formats
}

func (r *Renderer) registerBuiltinTemplates() {
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

	if err := r.RegisterTemplate("default", defaultTemplate); err != nil {
		panic(fmt.Sprintf("Failed to register default template: %v", err))
	}
	if err := r.RegisterTemplate("documentation", documentationTemplate); err != nil {
		panic(fmt.Sprintf("Failed to register documentation template: %v", err))
	}
}

func ValidateTemplate(templateStr string) error {
	_, err := template.New("validation").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}
	return nil
}

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

func GenerateHeader(data *TemplateData) string {
	headerTemplate := `<!-- 
🤖 GENERATED FILE - DO NOT EDIT DIRECTLY
===========================================

This file was automatically generated by ai-rulez from {{.ConfigFile}}.

⚠️  IMPORTANT FOR AI ASSISTANTS AND DEVELOPERS:
- DO NOT modify this file directly
- DO NOT add, remove, or change rules in this file
- Changes made here will be OVERWRITTEN on next generation

✅ TO UPDATE RULES:
1. Edit the source configuration: {{.ConfigFile}}
2. Regenerate this file: ai-rulez generate
3. The updated {{.OutputFile}} will be created automatically

📝 Generated: {{.Timestamp.Format "2006-01-02 15:04:05"}}
📁 Source: {{.ConfigFile}}
🎯 Target: {{.OutputFile}}
📊 Content: {{.RuleCount}} rules, {{.SectionCount}} sections

Learn more: https://github.com/Goldziher/ai-rulez
===========================================
-->

`

	tmpl, err := template.New("header").Parse(headerTemplate)
	if err != nil {
		return fmt.Sprintf(`<!-- Generated by ai-rulez from %s - DO NOT EDIT DIRECTLY -->

`, data.ConfigFile)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf(`<!-- Generated by ai-rulez from %s - DO NOT EDIT DIRECTLY -->

`, data.ConfigFile)
	}

	return buf.String()
}

func sortRulesByPriority(rules []config.Rule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].Name < rules[j].Name
	})
}

func sortSectionsByPriority(sections []config.Section) {
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Priority != sections[j].Priority {
			return sections[i].Priority > sections[j].Priority
		}
		return sections[i].Title < sections[j].Title
	})
}

func sortContent(items []ContentItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		return items[i].Title < items[j].Title
	})
}

func sortAgentsByPriority(agents []config.Agent) {
	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Priority != agents[j].Priority {
			return agents[i].Priority > agents[j].Priority
		}
		return agents[i].Name < agents[j].Name
	})
}
