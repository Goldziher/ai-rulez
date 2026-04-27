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
	config.RegisterPreset("junie", &JuniePresetGenerator{})
}

// JuniePresetGenerator generates Junie preset files (.junie/guidelines.md)
type JuniePresetGenerator struct{}

// generateJuniePresetHeader creates a header for Junie preset files
func generateJuniePresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
	// Create TemplateData for header generation
	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    time.Now(),
		ConfigFile:   configFileName(cfg),
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
		filepath.Join(baseDir, ".junie", "skills"),
		filepath.Join(baseDir, ".junie", "agents"),
	}
}

func (g *JuniePresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .junie directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".junie"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".junie", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".junie", "agents"),
			IsDir: true,
		},
	)

	// Generate guidelines.md file
	guidelinesContent := g.renderGuidelinesMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, ".junie", "guidelines.md"),
		Content: guidelinesContent,
		IsDir:   false,
	})

	// Generate skill files to .junie/skills/
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".junie", "skills", skillID)
		outputs = append(outputs,
			config.OutputFile{
				Path:  skillDir,
				IsDir: true,
			},
			config.OutputFile{
				Path:    filepath.Join(skillDir, "SKILL.md"),
				Content: g.renderSkillFile(skill),
			},
		)
	}

	// Generate agent files to .junie/agents/
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderJunieAgentFile(agent)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".junie", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

func (g *JuniePresetGenerator) renderGuidelinesMarkdown(content *config.ContentTree, cfg *config.Config) string {
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

	// Skills are generated to .junie/skills/ directory, not inlined

	return builder.String()
}

// renderSkillFile renders a skill file in SKILL.md format for Junie
func (g *JuniePresetGenerator) renderSkillFile(skill config.ContentFile) string {
	var builder strings.Builder

	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")
	builder.WriteString("description: ")
	builder.WriteString(quoteYAMLString(config.SkillDescriptionForContent(skill)))
	builder.WriteString("\n")
	builder.WriteString("---\n\n")
	builder.WriteString(skill.Content)

	return builder.String()
}

// renderJunieAgentFile renders an agent file with YAML frontmatter for Junie
func (g *JuniePresetGenerator) renderJunieAgentFile(agent config.ContentFile) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildJunieAgentFrontmatter(agent)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal agent frontmatter: %w", err)
	}

	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")
	builder.WriteString(agent.Content)

	return builder.String(), nil
}

// buildJunieAgentFrontmatter builds frontmatter for a Junie agent file
func (g *JuniePresetGenerator) buildJunieAgentFrontmatter(agent config.ContentFile) map[string]interface{} {
	frontmatter := map[string]interface{}{
		"name": agent.Name,
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	junieScalarFields := []string{
		"description", "disallowedTools",
		"model", "allowPromptArgument",
	}
	for _, field := range junieScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}
	if len(agent.Metadata.Skills) > 0 {
		frontmatter["skills"] = agent.Metadata.Skills
	}

	return frontmatter
}
