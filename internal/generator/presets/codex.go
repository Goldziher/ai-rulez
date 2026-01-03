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
	config.RegisterPresetV3("codex", &CodexPresetGenerator{})
}

// CodexPresetGenerator generates Codex preset files (AGENTS.md)
type CodexPresetGenerator struct{}

// generateCodexPresetHeader creates a header for Codex preset files
func generateCodexPresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *CodexPresetGenerator) GetName() string {
	return "codex"
}

func (g *CodexPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "AGENTS.md"),
	}
}

func (g *CodexPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Generate AGENTS.md file
	agentsContent := g.renderAgentsMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, "AGENTS.md"),
		Content: agentsContent,
		IsDir:   false,
	})

	return outputs, nil
}

func (g *CodexPresetGenerator) renderAgentsMarkdown(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Calculate content counts
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))

	// Add header before title
	header := generateCodexPresetHeader(cfg, "AGENTS.md", len(allRules), 0, len(allAgents))
	builder.WriteString(header)

	// Add title
	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	// Add rules section
	if len(allRules) > 0 {
		builder.WriteString("## Rules\n\n")
		for _, rule := range allRules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n\n") // Add blank line after heading

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

	// Add skills section
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	if len(allSkills) > 0 {
		builder.WriteString("## Skills\n\n")
		for _, skill := range allSkills {
			builder.WriteString("### ")
			builder.WriteString(skill.Name)
			builder.WriteString("\n\n")

			processedContent := markdown.ProcessEmbeddedContent(skill.Content)
			builder.WriteString(processedContent)
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}
