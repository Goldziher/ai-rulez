package presets

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPresetV3("continue-dev", &ContinueDevPresetGenerator{})
}

// ContinueDevPresetGenerator generates Continue.dev preset files
type ContinueDevPresetGenerator struct{}

// generateContinueDevPresetHeader creates a header for Continue.dev preset files
func generateContinueDevPresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
	// Create TemplateData for header generation
	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    time.Now(),
		ConfigFile:   "config.yaml", // V3 uses config.yaml
		OutputFile:   outputPath,
		Config:       cfg,
		RuleCount:    ruleCount,
		SectionCount: sectionCount,
		AgentCount:   agentCount,
	}

	return templates.GenerateHeader(data)
}

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
		outputPath := filepath.Join(".continue", "rules", sanitizeName(rule.Name)+".md")
		ruleContent := g.renderRuleFile(rule, cfg, outputPath, len(allRules))
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".continue", "rules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	// Generate prompts YAML file
	promptsContent, err := g.renderPromptsYAML(content, cfg)
	if err != nil {
		return nil, fmt.Errorf("render prompts YAML: %w", err)
	}

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".continue", "prompts", "ai_rulez_prompts.yaml"),
		Content: promptsContent,
	})

	return outputs, nil
}

func (g *ContinueDevPresetGenerator) renderRuleFile(rule config.ContentFile, cfg *config.ConfigV3, outputPath string, ruleCount int) string {
	var builder strings.Builder

	// Generate and prepend header
	header := generateContinueDevPresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

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
	processedContent := markdown.ProcessEmbeddedContent(rule.Content)
	builder.WriteString(processedContent)

	return builder.String()
}

func (g *ContinueDevPresetGenerator) renderPromptsYAML(content *config.ContentTreeV3, cfg *config.ConfigV3) (string, error) {
	var builder strings.Builder

	// Calculate content counts
	ruleCount := len(content.Rules)
	for _, domain := range content.Domains {
		ruleCount += len(domain.Rules)
	}

	// Generate and prepend header
	outputPath := ".continue/prompts/ai_rulez_prompts.yaml"
	header := generateContinueDevPresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

	prompts := make([]map[string]interface{}, 0)

	// Add context as prompts
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	for _, ctx := range allContext {
		processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
		prompt := map[string]interface{}{
			"name":        ctx.Name,
			"description": fmt.Sprintf("Context: %s", ctx.Name),
			"prompt":      processedContent,
		}
		prompts = append(prompts, prompt)
	}

	// Add skills as prompts
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		processedContent := markdown.ProcessEmbeddedContent(skill.Content)
		prompt := map[string]interface{}{
			"name":        skill.Name,
			"description": fmt.Sprintf("Skill: %s", skill.Name),
			"prompt":      processedContent,
		}
		if skill.Metadata != nil {
			if desc, ok := skill.Metadata.Extra["description"]; ok {
				prompt["description"] = desc
			}
		}
		prompts = append(prompts, prompt)
	}

	// Add commands as prompts
	allCommands := combineContentFiles(content.Commands, getAllDomainCommands(content))
	for _, command := range allCommands {
		// Check if command is enabled and targets continue-dev
		if !g.shouldIncludeCommand(command) {
			continue
		}

		processedContent := markdown.ProcessEmbeddedContent(command.Content)
		prompt := map[string]interface{}{
			"name":        command.Name,
			"description": fmt.Sprintf("Command: %s", command.Name),
			"prompt":      processedContent,
		}

		// Add metadata if available
		if command.Metadata != nil {
			if desc, ok := command.Metadata.Extra["description"]; ok {
				prompt["description"] = desc
			}
			if usage := command.Metadata.Usage; usage != "" {
				prompt["usage"] = usage
			}
		}

		prompts = append(prompts, prompt)
	}

	yamlData, err := yaml.Marshal(prompts)
	if err != nil {
		return "", fmt.Errorf("marshal YAML: %w", err)
	}

	// Prepend the header to YAML content
	builder.WriteString(string(yamlData))
	return builder.String(), nil
}

// shouldIncludeCommand checks if a command should be included in the continue-dev preset
func (g *ContinueDevPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if command.Metadata == nil {
		return true
	}

	// If targets are specified, only include if continue-dev is in targets
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == "continue-dev" {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}
