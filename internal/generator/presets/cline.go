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

const presetNameCline = "cline"

func init() {
	config.RegisterPreset(presetNameCline, &ClinePresetGenerator{})
}

// ClinePresetGenerator generates Cline preset files
type ClinePresetGenerator struct{}

// generateClinePresetHeader creates a header for Cline preset files
func generateClinePresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *ClinePresetGenerator) GetName() string {
	return presetNameCline
}

func (g *ClinePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".clinerules"),
		filepath.Join(baseDir, ".cline"),
		filepath.Join(baseDir, ".cline", "skills"),
		filepath.Join(baseDir, ".cline", "agents"),
	}
}

func (g *ClinePresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".clinerules"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cline"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cline", "skills"),
			IsDir: true,
		},
	)

	// Combine all rules from root and domains
	allRules := allInlineRules(content)

	// Generate rule files
	for _, rule := range allRules {
		outputPath := filepath.Join(".clinerules", sanitizeName(rule.Name)+".md")
		ruleContent := g.renderRuleFile(rule, cfg, outputPath, len(allRules))
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".clinerules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	// Generate skill files to .cline/skills/
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".cline", "skills", skillID)
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
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate context files as rule-like files in .clinerules/
	allContext := allInlineContext(content)
	for _, ctx := range allContext {
		sanitized := sanitizeName(ctx.Name)
		var ctxBuilder strings.Builder
		ctxBuilder.WriteString("# ")
		ctxBuilder.WriteString(ctx.Name)
		ctxBuilder.WriteString("\n\n")
		processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
		ctxBuilder.WriteString(processedContent)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".clinerules", "context-"+sanitized+".md"),
			Content: ctxBuilder.String(),
		})
	}

	// Add .cline/agents directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".cline", "agents"),
		IsDir: true,
	})

	// Generate agent files to .cline/agents/
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderClineAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".cline", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Cline
func (g *ClinePresetGenerator) renderSkillFile(skill config.ContentFile) string {
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
	builder.WriteString(RenderSkillResourcesIndex(&skill))

	return builder.String()
}

func (g *ClinePresetGenerator) renderRuleFile(rule config.ContentFile, cfg *config.Config, outputPath string, ruleCount int) string {
	var builder strings.Builder

	// Generate and prepend header
	header := generateClinePresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

	// Add title
	builder.WriteString("# ")
	builder.WriteString(rule.Name)
	builder.WriteString("\n\n")

	// Add priority if present
	if !cfg.IsCompact() && rule.Metadata != nil && rule.Metadata.Priority != "" {
		builder.WriteString("**Priority:** ")
		builder.WriteString(rule.Metadata.Priority)
		builder.WriteString("\n\n")
	}

	// Add content
	processedContent := markdown.ProcessEmbeddedContent(rule.Content)
	builder.WriteString(processedContent)

	return builder.String()
}

// renderClineAgentFile renders an agent file with YAML frontmatter for Cline
func (g *ClinePresetGenerator) renderClineAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildClineAgentFrontmatter(agent, cfg)

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

// buildClineAgentFrontmatter builds frontmatter for a Cline agent file
func (g *ClinePresetGenerator) buildClineAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := ResolveAgentModel(presetNameCline, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	clineScalarFields := []string{keyDescription}
	for _, field := range clineScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
