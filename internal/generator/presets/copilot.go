package presets

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func init() {
	config.RegisterPresetV3("copilot", &CopilotPresetGenerator{})
}

// CopilotPresetGenerator generates GitHub Copilot preset files
type CopilotPresetGenerator struct{}

// generateCopilotPresetHeader creates a header for Copilot preset files
func generateCopilotPresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *CopilotPresetGenerator) GetName() string {
	return "copilot"
}

func (g *CopilotPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".github"),
		filepath.Join(baseDir, ".github", "copilot-instructions.md"),
	}
}

func (g *CopilotPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .github directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".github"),
		IsDir: true,
	})

	// Generate copilot-instructions.md
	instructionsContent := g.renderInstructionsFile(content, cfg)
	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".github", "copilot-instructions.md"),
		Content: instructionsContent,
	})

	return outputs, nil
}

func (g *CopilotPresetGenerator) renderInstructionsFile(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Calculate content counts
	ruleCount := len(content.Rules)
	for _, domain := range content.Domains {
		ruleCount += len(domain.Rules)
	}

	// Generate and prepend header
	outputPath := ".github/copilot-instructions.md"
	header := generateCopilotPresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

	// Add header
	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	// Add rules section
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	if len(allRules) > 0 {
		builder.WriteString("## Rules\n\n")
		for _, rule := range allRules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n\n")

			if rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}

			processedContent := markdown.ProcessEmbeddedContent(rule.Content)
			builder.WriteString(processedContent)
			builder.WriteString("\n\n")
		}
	}

	// Add context section
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	if len(allContext) > 0 {
		builder.WriteString("## Context\n\n")
		for _, ctx := range allContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")
			processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
			builder.WriteString(processedContent)
			builder.WriteString("\n\n")
		}
	}

	// Skills should not be inlined in copilot-instructions.md

	return builder.String()
}
