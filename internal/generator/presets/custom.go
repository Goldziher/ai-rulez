package presets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	// Register the custom preset generator factory
	config.CustomPresetGeneratorFactory = func(preset config.Preset) config.PresetGenerator {
		return NewCustomPresetGenerator(&preset)
	}
}

// CustomPresetGenerator handles custom preset generation
type CustomPresetGenerator struct {
	Preset config.Preset
}

// NewCustomPresetGenerator creates a new custom preset generator
func NewCustomPresetGenerator(preset *config.Preset) *CustomPresetGenerator {
	return &CustomPresetGenerator{
		Preset: *preset,
	}
}

func (g *CustomPresetGenerator) GetName() string {
	return g.Preset.Name
}

func (g *CustomPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, g.Preset.Path),
	}
}

func (g *CustomPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	switch g.Preset.Type {
	case config.PresetTypeMarkdown:
		return g.generateMarkdown(content, baseDir, cfg)
	case config.PresetTypeDirectory:
		return g.generateDirectory(content, baseDir, cfg)
	case config.PresetTypeJSON:
		return g.generateJSON(content, baseDir, cfg)
	default:
		return nil, fmt.Errorf("unsupported preset type: %s", g.Preset.Type)
	}
}

func (g *CustomPresetGenerator) generateMarkdown(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Prepare template data
	data := g.prepareTemplateData(content, cfg)

	// Render content
	renderedContent, err := g.renderTemplate(data)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, g.Preset.Path),
		Content: renderedContent,
	})

	return outputs, nil
}

func (g *CustomPresetGenerator) generateDirectory(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	dirPath := filepath.Join(baseDir, g.Preset.Path)

	// Create directory
	outputs = append(outputs, config.OutputFile{
		Path:  dirPath,
		IsDir: true,
	})

	// Generate files for each rule, context, and skill
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	for _, rule := range allRules {
		sanitized := sanitizeName(rule.Name)
		ruleContent := g.renderContentFile(rule)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(dirPath, sanitized+".md"),
			Content: ruleContent,
		})
	}

	return outputs, nil
}

func (g *CustomPresetGenerator) generateJSON(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Prepare template data
	data := g.prepareTemplateData(content, cfg)

	// If template is provided, render it; otherwise use default JSON structure
	var jsonContent string
	var err error

	if g.Preset.Template != "" {
		jsonContent, err = g.renderTemplate(data)
		if err != nil {
			return nil, err
		}
	} else {
		// Default JSON structure
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal JSON: %w", err)
		}
		jsonContent = string(jsonData)
	}

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, g.Preset.Path),
		Content: jsonContent,
	})

	return outputs, nil
}

func (g *CustomPresetGenerator) prepareTemplateData(content *config.ContentTree, cfg *config.Config) map[string]interface{} {
	// Combine all content
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))

	// Convert to template-friendly structures
	rules := make([]map[string]interface{}, 0, len(allRules))
	for _, rule := range allRules {
		ruleData := map[string]interface{}{
			keyNameField:    rule.Name,
			keyContentField: rule.Content,
		}
		if rule.Metadata != nil {
			if rule.Metadata.Priority != "" {
				ruleData["Priority"] = rule.Metadata.Priority
			}
			if len(rule.Metadata.Targets) > 0 {
				ruleData["Targets"] = rule.Metadata.Targets
			}
		}
		rules = append(rules, ruleData)
	}

	contextData := make([]map[string]interface{}, 0, len(allContext))
	for _, ctx := range allContext {
		contextData = append(contextData, map[string]interface{}{
			keyNameField:    ctx.Name,
			keyContentField: ctx.Content,
		})
	}

	skills := make([]map[string]interface{}, 0, len(allSkills))
	for _, skill := range allSkills {
		// Backward-compat default for legacy templates that interpolate
		// `.Content` directly: include inlined references so users who never
		// updated their template still see reference material.
		legacyContent := skill.Content + InlineSkillResources(&skill)

		skillData := map[string]interface{}{
			keyNameField:    skill.Name,
			keyContentField: legacyContent,
			"Body":          skill.Content,
			"Resources":     skillResourcesToMaps(skill.Resources),
		}
		if skill.Metadata != nil {
			if skill.Metadata.Priority != "" {
				skillData["Priority"] = skill.Metadata.Priority
			}
			for k, v := range skill.Metadata.Extra {
				skillData[k] = v
			}
		}
		skills = append(skills, skillData)
	}

	return map[string]interface{}{
		keyNameField:  cfg.Name,
		"Description": cfg.Description,
		"Version":     cfg.Version,
		"Rules":       rules,
		"Context":     contextData,
		"Skills":      skills,
		"Domains":     g.prepareDomainData(content),
	}
}

func (g *CustomPresetGenerator) prepareDomainData(content *config.ContentTree) map[string]interface{} {
	domains := make(map[string]interface{})

	for name, domain := range content.Domains {
		domainData := map[string]interface{}{
			keyNameField: domain.Name,
			"Rules":      g.contentFilesToMaps(domain.Rules),
			"Context":    g.contentFilesToMaps(domain.Context),
			"Skills":     g.contentFilesToMaps(domain.Skills),
		}
		domains[name] = domainData
	}

	return domains
}

// skillResourcesToMaps converts skill resources into template-friendly maps.
// Bytes are exposed as strings — custom templates targeting binary assets
// must handle that themselves.
func skillResourcesToMaps(resources []config.SkillResource) []map[string]interface{} {
	if len(resources) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(resources))
	for _, r := range resources {
		out = append(out, map[string]interface{}{
			"Kind":          r.Kind,
			"RelPath":       r.RelPath,
			keyContentField: string(r.Content),
			"Description":   r.Description,
		})
	}
	return out
}

func (g *CustomPresetGenerator) contentFilesToMaps(files []config.ContentFile) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		fileData := map[string]interface{}{
			keyNameField:    file.Name,
			keyContentField: file.Content,
		}
		if file.Metadata != nil {
			if file.Metadata.Priority != "" {
				fileData["Priority"] = file.Metadata.Priority
			}
			if len(file.Metadata.Targets) > 0 {
				fileData["Targets"] = file.Metadata.Targets
			}
		}
		result = append(result, fileData)
	}
	return result
}

func (g *CustomPresetGenerator) renderTemplate(data map[string]interface{}) (string, error) {
	if g.Preset.Template == "" {
		return g.renderDefault(data), nil
	}

	tmpl, err := template.New("custom").Parse(g.Preset.Template)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var builder strings.Builder
	if err := tmpl.Execute(&builder, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return builder.String(), nil
}

//nolint:gocyclo // Complex logic, acceptable for this use case
func (g *CustomPresetGenerator) renderDefault(data map[string]interface{}) string {
	var builder strings.Builder

	// Default markdown format
	if name, ok := data[keyNameField].(string); ok {
		builder.WriteString("# ")
		builder.WriteString(name)
		builder.WriteString("\n\n")
	}

	if desc, ok := data["Description"].(string); ok && desc != "" {
		builder.WriteString(desc)
		builder.WriteString("\n\n")
	}

	// Add rules
	if rules, ok := data["Rules"].([]map[string]interface{}); ok && len(rules) > 0 {
		builder.WriteString("## Rules\n\n")
		for _, rule := range rules {
			if name, ok := rule[keyNameField].(string); ok {
				builder.WriteString("### ")
				builder.WriteString(name)
				builder.WriteString("\n")
			}
			if priority, ok := rule["Priority"].(string); ok {
				builder.WriteString("**Priority:** ")
				builder.WriteString(priority)
				builder.WriteString("\n\n")
			}
			if content, ok := rule[keyContentField].(string); ok {
				builder.WriteString(content)
				builder.WriteString("\n\n")
			}
		}
	}

	// Add context
	if context, ok := data["Context"].([]map[string]interface{}); ok && len(context) > 0 {
		builder.WriteString("## Context\n\n")
		for _, ctx := range context {
			if name, ok := ctx[keyNameField].(string); ok {
				builder.WriteString("### ")
				builder.WriteString(name)
				builder.WriteString("\n\n")
			}
			if content, ok := ctx[keyContentField].(string); ok {
				builder.WriteString(content)
				builder.WriteString("\n\n")
			}
		}
	}

	// Add skills
	if skills, ok := data["Skills"].([]map[string]interface{}); ok && len(skills) > 0 {
		builder.WriteString("## Skills\n\n")
		for _, skill := range skills {
			if name, ok := skill[keyNameField].(string); ok {
				builder.WriteString("### ")
				builder.WriteString(name)
				builder.WriteString("\n\n")
			}
			if content, ok := skill[keyContentField].(string); ok {
				builder.WriteString(content)
				builder.WriteString("\n\n")
			}
		}
	}

	return builder.String()
}

func (g *CustomPresetGenerator) renderContentFile(file config.ContentFile) string {
	var builder strings.Builder

	builder.WriteString("# ")
	builder.WriteString(file.Name)
	builder.WriteString("\n\n")

	if file.Metadata != nil && file.Metadata.Priority != "" {
		builder.WriteString("**Priority:** ")
		builder.WriteString(file.Metadata.Priority)
		builder.WriteString("\n\n")
	}

	builder.WriteString(file.Content)

	return builder.String()
}
