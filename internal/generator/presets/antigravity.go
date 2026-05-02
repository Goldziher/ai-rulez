package presets

import (
	"encoding/json"
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
	config.RegisterPreset(presetNameAntigravity, &AntigravityPresetGenerator{})
}

// AntigravityPresetGenerator generates Antigravity preset files
type AntigravityPresetGenerator struct{}

func generateAntigravityPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *AntigravityPresetGenerator) GetName() string {
	return presetNameAntigravity
}

func (g *AntigravityPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "GEMINI.md"),
		filepath.Join(baseDir, ".agents"),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, ".agents", "agents"),
	}
}

func (g *AntigravityPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .agents directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".agents"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".agents", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".agents", "agents"),
			IsDir: true,
		},
	)

	// Generate .agents/settings.json with MCP configuration
	settingsContent, err := g.renderSettingsJSON(cfg)
	if err != nil {
		return nil, fmt.Errorf("render settings.json: %w", err)
	}

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, ".agents", "settings.json"),
		Content: settingsContent,
	})

	// Generate GEMINI.md with all rules and context
	geminiMD := g.renderMarkdown(content, cfg)
	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, "GEMINI.md"),
		Content: geminiMD,
	})

	// Generate skill files to .agents/skills/
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".agents", "skills", skillID)
		outputs = append(outputs, config.OutputFile{
			Path:  skillDir,
			IsDir: true,
		})

		skillContent := g.renderSkillFile(skill)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate agent files to .agents/agents/
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderAgentFile(agent)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".agents", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

func (g *AntigravityPresetGenerator) renderSettingsJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]interface{})

	// Always include the hardcoded ai-rulez MCP server
	mcpServers["ai-rulez"] = map[string]interface{}{
		keyCommand: cmdNPX,
		"args": []string{
			"-y",
			"ai-rulez@latest",
			keyMCP,
		},
	}

	// Merge user-configured MCP servers
	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			keyCommand: server.Command,
		}

		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
		if server.Transport != "" {
			entry["transport"] = server.GetTransport()
		}
		if server.URL != "" {
			entry["url"] = server.URL
		}
		if !server.IsEnabled() {
			entry[keyDisabled] = true
		}

		mcpServers[name] = entry
	}

	settings := map[string]interface{}{
		keyMCPServers: mcpServers,
	}

	jsonData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return string(jsonData) + "\n", nil
}

func (g *AntigravityPresetGenerator) renderMarkdown(content *config.ContentTree, cfg *config.Config) string {
	var builder strings.Builder

	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))

	header := generateAntigravityPresetHeader(cfg, "GEMINI.md", len(allRules), 0, len(allAgents))
	builder.WriteString(header)

	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

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

	renderAgentsSection(&builder, content, allAgents)

	return builder.String()
}

func (g *AntigravityPresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

func (g *AntigravityPresetGenerator) renderAgentFile(agent config.ContentFile) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildAgentFrontmatter(agent)

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

func (g *AntigravityPresetGenerator) buildAgentFrontmatter(agent config.ContentFile) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	agentScalarFields := []string{keyDescription, keyKind, keyModel, keyTemperature, "max_turns", "timeout_mins"}
	for _, field := range agentScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
