package presets

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func init() {
	config.RegisterPreset("windsurf", &WindsurfPresetGenerator{})
}

// WindsurfPresetGenerator generates Windsurf preset files
type WindsurfPresetGenerator struct{}

// generateWindsurfPresetHeader creates a header for Windsurf preset files
func generateWindsurfPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *WindsurfPresetGenerator) GetName() string {
	return "windsurf"
}

func (g *WindsurfPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".windsurf"),
		filepath.Join(baseDir, ".windsurf", "rules"),
		filepath.Join(baseDir, ".windsurf", "skills"),
		filepath.Join(baseDir, ".windsurf", "agents"),
	}
}

func (g *WindsurfPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .windsurf directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".windsurf"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".windsurf", "rules"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".windsurf", "skills"),
			IsDir: true,
		},
	)

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		outputPath := filepath.Join(".windsurf", "rules", sanitizeName(rule.Name)+".md")
		ruleContent := g.renderRuleFile(rule, cfg, outputPath, len(allRules))
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".windsurf", "rules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	// Generate skill files to .windsurf/skills/
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".windsurf", "skills", skillID)
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

	// Generate context files as rule-like files in .windsurf/rules/
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	for _, ctx := range allContext {
		sanitized := sanitizeName(ctx.Name)
		var ctxBuilder strings.Builder
		ctxBuilder.WriteString("# ")
		ctxBuilder.WriteString(ctx.Name)
		ctxBuilder.WriteString("\n\n")
		processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
		ctxBuilder.WriteString(processedContent)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".windsurf", "rules", "context-"+sanitized+".md"),
			Content: ctxBuilder.String(),
		})
	}

	// Add .windsurf/agents directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".windsurf", "agents"),
		IsDir: true,
	})

	// Generate agent files to .windsurf/agents/
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderWindsurfAgentFile(agent)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".windsurf", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Windsurf
func (g *WindsurfPresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

// renderTriggerFrontmatter renders Windsurf trigger frontmatter if needed
func (g *WindsurfPresetGenerator) renderTriggerFrontmatter(builder *strings.Builder, rule config.ContentFile) {
	if rule.Metadata == nil {
		return
	}

	// Check if we should render trigger frontmatter
	if !rule.Metadata.ShouldRenderTriggerFrontmatter() {
		return
	}

	rawMode := strings.TrimSpace(rule.Metadata.Extra["trigger"])
	mode := rule.Metadata.GetTriggerMode()

	// Warn when user provided an unknown trigger mode and we fallback to manual.
	if rawMode != "" && !config.IsValidTriggerMode(rawMode) {
		logger.Warn(
			"Unknown trigger mode in Windsurf rule, using default",
			"mode", rawMode,
			"rule", rule.Name,
			"default", config.TriggerManual,
		)
		mode = config.TriggerManual
	}

	// Only render if non-default
	if mode == config.TriggerManual {
		desc := rule.Metadata.GetTriggerDescription()
		glob := rule.Metadata.GetTriggerGlob()

		// Still render if has extra config
		if desc == "" && glob == "" {
			return
		}
	}

	builder.WriteString("---\n")
	builder.WriteString("trigger: ")
	builder.WriteString(mode)
	builder.WriteString("\n")

	if desc := rule.Metadata.GetTriggerDescription(); desc != "" {
		builder.WriteString("description: ")
		builder.WriteString(quoteWindsurfYAMLString(desc))
		builder.WriteString("\n")
	}

	if glob := rule.Metadata.GetTriggerGlob(); glob != "" {
		builder.WriteString("glob: ")
		builder.WriteString(quoteWindsurfYAMLString(glob))
		builder.WriteString("\n")
	}

	builder.WriteString("---\n\n")
}

func quoteWindsurfYAMLString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	return "\"" + escaped + "\""
}

func (g *WindsurfPresetGenerator) renderRuleFile(rule config.ContentFile, cfg *config.Config, outputPath string, ruleCount int) string {
	var builder strings.Builder

	// Render trigger frontmatter FIRST (must be at line 1 for Windsurf)
	g.renderTriggerFrontmatter(&builder, rule)

	// Generate and add header after frontmatter
	header := generateWindsurfPresetHeader(cfg, outputPath, ruleCount, 0, 0)
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

// renderWindsurfAgentFile renders an agent file with YAML frontmatter for Windsurf
func (g *WindsurfPresetGenerator) renderWindsurfAgentFile(agent config.ContentFile) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildWindsurfAgentFrontmatter(agent)

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

// buildWindsurfAgentFrontmatter builds frontmatter for a Windsurf agent file
func (g *WindsurfPresetGenerator) buildWindsurfAgentFrontmatter(agent config.ContentFile) map[string]interface{} {
	frontmatter := map[string]interface{}{
		"name": agent.Name,
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	windsurfFields := []string{"description", "model", "tools"}
	for _, field := range windsurfFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}

	return frontmatter
}
