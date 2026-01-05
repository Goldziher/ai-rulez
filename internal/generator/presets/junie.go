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
	config.RegisterPresetV3("junie", &JuniePresetGenerator{})
}

// JuniePresetGenerator generates Junie preset files (.junie/guidelines.md)
type JuniePresetGenerator struct{}

// generateJuniePresetHeader creates a header for Junie preset files
func generateJuniePresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *JuniePresetGenerator) GetName() string {
	return "junie"
}

func (g *JuniePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".junie"),
		filepath.Join(baseDir, ".junie", "guidelines.md"),
	}
}

func (g *JuniePresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .junie directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".junie"),
		IsDir: true,
	})

	// Generate guidelines.md file
	guidelinesContent := g.renderGuidelinesMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".junie", "guidelines.md"),
		Content: guidelinesContent,
		IsDir:   false,
	})

	return outputs, nil
}

func (g *JuniePresetGenerator) renderGuidelinesMarkdown(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Calculate content counts
	ruleCount := len(content.Rules)
	for _, domain := range content.Domains {
		ruleCount += len(domain.Rules)
	}

	// Generate and prepend header
	outputPath := ".junie/guidelines.md"
	header := generateJuniePresetHeader(cfg, outputPath, ruleCount, 0, 0)
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

	// Skills should not be inlined in guidelines.md

	return builder.String()
}
