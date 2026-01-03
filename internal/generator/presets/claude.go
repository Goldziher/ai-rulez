package presets

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPresetV3("claude", &ClaudePresetGenerator{})
}

// ClaudePresetGenerator generates Claude preset files
type ClaudePresetGenerator struct{}

// generatePresetHeader creates a header for Claude preset files
func generatePresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *ClaudePresetGenerator) GetName() string {
	return "claude"
}

func (g *ClaudePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "CLAUDE.md"),
		filepath.Join(baseDir, ".claude"),
		filepath.Join(baseDir, ".claude", "skills"),
		filepath.Join(baseDir, ".claude", "agents"),
	}
}

func (g *ClaudePresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .claude directory
	outputs = append(outputs,
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".claude"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".claude", "skills"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".claude", "agents"),
			IsDir: true,
		},
	)

	// Generate CLAUDE.md with all rules and context
	claudeMD := g.renderClaudeMarkdown(content, cfg)
	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, "CLAUDE.md"),
		Content: claudeMD,
	})

	// Generate skill files
	for _, skill := range content.Skills {
		skillOutputs, err := g.generateSkillFiles(skill, content, baseDir, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate skill %s: %w", skill.Name, err)
		}
		outputs = append(outputs, skillOutputs...)
	}

	// Generate agent files
	if len(content.Agents) > 0 {
		for _, agent := range content.Agents {
			agentOutputs, err := g.generateAgentFiles(agent, content, baseDir, cfg)
			if err != nil {
				return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
			}
			outputs = append(outputs, agentOutputs...)
		}
	}

	return outputs, nil
}

func (g *ClaudePresetGenerator) generateSkillFiles(skill config.ContentFile, content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Extract skill ID from path
	skillID := extractSkillID(skill.Path)

	// Create skill directory
	skillDir := filepath.Join(baseDir, ".claude", "skills", skillID)
	outputs = append(outputs, config.OutputFileV3{
		Path:  skillDir,
		IsDir: true,
	})

	// Generate SKILL.md with frontmatter
	skillOutputPath := filepath.Join(skillDir, "SKILL.md")
	skillContent, err := g.renderSkillFile(skill, content, cfg, skillOutputPath)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFileV3{
		Path:    skillOutputPath,
		Content: skillContent,
	})

	return outputs, nil
}

func (g *ClaudePresetGenerator) renderSkillFile(skill config.ContentFile, content *config.ContentTreeV3, cfg *config.ConfigV3, outputPath string) (string, error) {
	var builder strings.Builder

	// Generate frontmatter
	frontmatter := make(map[string]interface{})
	frontmatter["name"] = skill.Name

	if skill.Metadata != nil {
		if skill.Metadata.Priority != "" {
			frontmatter["priority"] = skill.Metadata.Priority
		}
		if len(skill.Metadata.Targets) > 0 {
			frontmatter["targets"] = skill.Metadata.Targets
		}
		// Include extra metadata
		for k, v := range skill.Metadata.Extra {
			if k != "priority" && k != "targets" && k != "name" {
				frontmatter[k] = v
			}
		}
	}

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}

	// Add header before frontmatter
	header := generatePresetHeader(cfg, outputPath, len(content.Rules), 0, len(content.Agents))
	builder.WriteString(header)

	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	// Add rules section
	if len(content.Rules) > 0 {
		builder.WriteString("\n\n## Rules\n\n")
		for _, rule := range content.Rules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n")
			if rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}
			builder.WriteString(rule.Content)
			builder.WriteString("\n\n")
		}
	}

	// Add context section
	if len(content.Context) > 0 {
		builder.WriteString("\n## Context\n\n")
		for _, ctx := range content.Context {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")
			builder.WriteString(ctx.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String(), nil
}

func (g *ClaudePresetGenerator) generateAgentFiles(agent config.ContentFile, content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Extract agent ID from name
	agentID := sanitizeAgentID(agent.Name)

	// Generate agent file with Claude's agent format
	agentOutputPath := filepath.Join(baseDir, ".claude", "agents", agentID+".md")
	agentContent, err := g.renderAgentContent(agent, content, cfg, agentOutputPath)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFileV3{
		Path:    agentOutputPath,
		Content: agentContent,
	})

	return outputs, nil
}

func (g *ClaudePresetGenerator) buildAgentFrontmatter(agent config.ContentFile) map[string]interface{} {
	frontmatter := make(map[string]interface{})
	frontmatter["name"] = agent.Name

	if agent.Metadata == nil {
		return frontmatter
	}

	// Add standard Claude agent fields
	const (
		description    = "description"
		model          = "model"
		tools          = "tools"
		permissionMode = "permission_mode"
		skills         = "skills"
	)

	fields := []struct {
		key   string
		field string
	}{
		{description, description},
		{model, model},
		{tools, tools},
		{permissionMode, permissionMode},
		{skills, skills},
	}

	for _, f := range fields {
		if val, ok := agent.Metadata.Extra[f.field]; ok && val != "" {
			frontmatter[f.key] = val
		}
	}

	return frontmatter
}

func (g *ClaudePresetGenerator) renderClaudeMarkdown(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Calculate content counts
	ruleCount := len(content.Rules)
	agentCount := len(content.Agents)
	for _, domain := range content.Domains {
		ruleCount += len(domain.Rules)
		agentCount += len(domain.Agents)
	}

	// Generate and prepend header
	outputPath := "CLAUDE.md"
	header := generatePresetHeader(cfg, outputPath, ruleCount, 0, agentCount)
	builder.WriteString(header)

	// Add project title
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
			builder.WriteString("\n")

			if rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}

			builder.WriteString(rule.Content)
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
			builder.WriteString(ctx.Content)
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
			builder.WriteString(skill.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}

func (g *ClaudePresetGenerator) renderAgentContent(agent config.ContentFile, content *config.ContentTreeV3, cfg *config.ConfigV3, outputPath string) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildAgentFrontmatter(agent)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal agent frontmatter: %w", err)
	}

	// Add header before frontmatter
	header := generatePresetHeader(cfg, outputPath, len(content.Rules), 0, len(content.Agents))
	builder.WriteString(header)

	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")

	// Add agent content
	builder.WriteString(agent.Content)

	// Add rules section if available
	if len(content.Rules) > 0 {
		builder.WriteString("\n\n## Rules\n\n")
		for _, rule := range content.Rules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n\n")
			builder.WriteString(rule.Content)
			builder.WriteString("\n\n")
		}
	}

	// Add context section if available
	if len(content.Context) > 0 {
		builder.WriteString("\n## Context\n\n")
		for _, ctx := range content.Context {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")
			builder.WriteString(ctx.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String(), nil
}

// sanitizeAgentID converts an agent name to a valid file ID
func sanitizeAgentID(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// extractSkillID extracts the skill ID from a skill's path
func extractSkillID(skillPath string) string {
	// Path format: .../skills/{skill-id}/SKILL.md
	dir := filepath.Dir(skillPath)
	return filepath.Base(dir)
}
