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
		filepath.Join(baseDir, ".codex"),
		filepath.Join(baseDir, ".codex", "skills"),
	}
}

func (g *CodexPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .codex directory structure
	outputs = append(outputs,
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".codex"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".codex", "skills"),
			IsDir: true,
		},
	)

	// Generate AGENTS.md file (rules and context only, no skills)
	agentsContent := g.renderAgentsMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, "AGENTS.md"),
		Content: agentsContent,
		IsDir:   false,
	})

	// Combine all skills from root and domains
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))

	// Generate skill files to .codex/skills/
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		// Create skill directory
		skillDir := filepath.Join(baseDir, ".codex", "skills", skillID)
		outputs = append(outputs, config.OutputFileV3{
			Path:  skillDir,
			IsDir: true,
		})

		// Generate SKILL.md file
		skillContent := g.renderSkillFile(skill)
		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})
	}

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

	// Skills are generated to .codex/skills/ directory, not inlined in AGENTS.md

	return builder.String()
}

// renderSkillFile renders a skill file in SKILL.md format for Codex
func (g *CodexPresetGenerator) renderSkillFile(skill config.ContentFile) string {
	var builder strings.Builder

	// Add YAML frontmatter
	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")

	// Description is required by Codex for skill loading.
	builder.WriteString("description: ")
	builder.WriteString(quoteYAMLString(resolveCodexSkillDescription(skill)))
	builder.WriteString("\n")

	if skill.Metadata != nil {
		// Add short-description for user-facing display.
		if shortDesc, ok := skill.Metadata.Extra["short-description"]; ok && strings.TrimSpace(shortDesc) != "" {
			builder.WriteString("metadata:\n")
			builder.WriteString("  short-description: ")
			builder.WriteString(quoteYAMLString(strings.TrimSpace(shortDesc)))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	return builder.String()
}

func resolveCodexSkillDescription(skill config.ContentFile) string {
	if skill.Metadata != nil {
		if desc, ok := skill.Metadata.Extra["description"]; ok && strings.TrimSpace(desc) != "" {
			return strings.TrimSpace(desc)
		}
		if shortDesc, ok := skill.Metadata.Extra["short-description"]; ok && strings.TrimSpace(shortDesc) != "" {
			return strings.TrimSpace(shortDesc)
		}
	}

	if strings.TrimSpace(skill.Name) != "" {
		return "Instructions for " + strings.ReplaceAll(strings.TrimSpace(skill.Name), "-", " ") + "."
	}

	return "Instructions for this skill."
}

func quoteYAMLString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	return "\"" + escaped + "\""
}
