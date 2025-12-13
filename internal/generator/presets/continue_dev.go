package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPresetV3("continue-dev", &ContinueDevPresetGenerator{})
}

// ContinueDevPresetGenerator generates Continue.dev preset files
type ContinueDevPresetGenerator struct{}

func (g *ContinueDevPresetGenerator) GetName() string {
	return "continue-dev"
}

func (g *ContinueDevPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".continue"),
		filepath.Join(baseDir, ".continue", "rules"),
		filepath.Join(baseDir, ".continue", "prompts"),
	}
}

func (g *ContinueDevPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .continue directory
	outputs = append(outputs,
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".continue"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".continue", "rules"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".continue", "prompts"),
			IsDir: true,
		},
	)

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		ruleContent := g.renderRuleFile(rule)
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".continue", "rules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	// Generate prompts YAML file
	promptsContent, err := g.renderPromptsYAML(content)
	if err != nil {
		return nil, fmt.Errorf("render prompts YAML: %w", err)
	}

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".continue", "prompts", "ai_rulez_prompts.yaml"),
		Content: promptsContent,
	})

	return outputs, nil
}

func (g *ContinueDevPresetGenerator) renderRuleFile(rule config.ContentFile) string {
	var builder strings.Builder

	// Add title
	builder.WriteString("# ")
	builder.WriteString(rule.Name)
	builder.WriteString("\n\n")

	// Add priority if present
	if rule.Metadata != nil && rule.Metadata.Priority != "" {
		builder.WriteString("**Priority:** ")
		builder.WriteString(rule.Metadata.Priority)
		builder.WriteString("\n\n")
	}

	// Add content
	builder.WriteString(rule.Content)

	return builder.String()
}

func (g *ContinueDevPresetGenerator) renderPromptsYAML(content *config.ContentTreeV3) (string, error) {
	prompts := make([]map[string]interface{}, 0)

	// Add context as prompts
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	for _, ctx := range allContext {
		prompt := map[string]interface{}{
			"name":        ctx.Name,
			"description": fmt.Sprintf("Context: %s", ctx.Name),
			"prompt":      ctx.Content,
		}
		prompts = append(prompts, prompt)
	}

	// Add skills as prompts
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		prompt := map[string]interface{}{
			"name":        skill.Name,
			"description": fmt.Sprintf("Skill: %s", skill.Name),
			"prompt":      skill.Content,
		}
		if skill.Metadata != nil {
			if desc, ok := skill.Metadata.Extra["description"]; ok {
				prompt["description"] = desc
			}
		}
		prompts = append(prompts, prompt)
	}

	yamlData, err := yaml.Marshal(prompts)
	if err != nil {
		return "", fmt.Errorf("marshal YAML: %w", err)
	}

	return string(yamlData), nil
}
