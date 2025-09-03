package templates

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
)

type contentItem struct {
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
	AllContent   []contentItem
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

	if outputPath != "" {
		var err error
		allRules, err = config.FilterRules(allRules, outputPath, nil)
		if err != nil {
			allRules = cfg.Rules
		}

		allSections, err = config.FilterSections(allSections, outputPath, nil)
		if err != nil {
			allSections = cfg.Sections
		}

		allAgents, err = config.FilterAgents(allAgents, outputPath, nil)
		if err != nil {
			allAgents = cfg.Agents
		}
	}

	sortedRules := make([]config.Rule, len(allRules))
	copy(sortedRules, allRules)

	sortedSections := make([]config.Section, len(allSections))
	copy(sortedSections, allSections)

	allContent := make([]contentItem, 0, len(allRules)+len(allSections))

	for _, rule := range allRules {
		allContent = append(allContent, contentItem{
			Type:     "rule",
			Title:    rule.Name,
			Priority: rule.Priority.ToInt(),
			Content:  rule.Content,
			IsRule:   true,
		})
	}

	for _, section := range allSections {
		allContent = append(allContent, contentItem{
			Type:     "section",
			Title:    section.Name,
			Priority: section.Priority.ToInt(),
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
		OutputFile:   outputPath,
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
	builtinTemplates := getBuiltinTemplates()

	for name, template := range builtinTemplates {
		if err := r.RegisterTemplate(name, template); err != nil {
			panic(fmt.Sprintf("Failed to register %s template: %v", name, err))
		}
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

// ExecuteTemplate executes a template with any data type
func ExecuteTemplate(templateStr string, data interface{}) (string, error) {
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
	slices.SortFunc(rules, func(a, b config.Rule) int {
		if a.Priority != b.Priority {
			return cmp.Compare(b.Priority, a.Priority)
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

func sortSectionsByPriority(sections []config.Section) {
	slices.SortFunc(sections, func(a, b config.Section) int {
		if a.Priority != b.Priority {
			return cmp.Compare(b.Priority, a.Priority)
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

func sortContent(items []contentItem) {
	slices.SortFunc(items, func(a, b contentItem) int {
		if a.Priority != b.Priority {
			return cmp.Compare(b.Priority, a.Priority)
		}
		return cmp.Compare(a.Title, b.Title)
	})
}

func sortAgentsByPriority(agents []config.Agent) {
	slices.SortFunc(agents, func(a, b config.Agent) int {
		if a.Priority != b.Priority {
			return cmp.Compare(b.Priority, a.Priority)
		}
		return cmp.Compare(a.Name, b.Name)
	})
}
