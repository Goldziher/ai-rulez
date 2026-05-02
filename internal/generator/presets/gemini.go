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
	config.RegisterPreset("gemini", &GeminiPresetGenerator{})
}

// GeminiPresetGenerator generates Gemini preset files
type GeminiPresetGenerator struct{}

// generatePresetHeader creates a header for Gemini preset files
func generateGeminiPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *GeminiPresetGenerator) GetName() string {
	return "gemini"
}

func (g *GeminiPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".gemini"),
		filepath.Join(baseDir, "GEMINI.md"),
		filepath.Join(baseDir, ".agents"),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, ".agents", "agents"),
	}
}

func (g *GeminiPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".gemini"),
			IsDir: true,
		},
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

	// Generate settings.json with MCP configuration
	settingsContent, err := g.renderSettingsJSON(cfg)
	if err != nil {
		return nil, fmt.Errorf("render settings.json: %w", err)
	}

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, ".gemini", "settings.json"),
		Content: settingsContent,
	})

	// Generate GEMINI.md with all rules and context
	geminiMD := g.renderGeminiMarkdown(content, cfg)
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

		skillContent := g.renderGeminiSkillFile(skill)
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
		agentContent, err := g.renderGeminiAgentFile(agent)
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

func (g *GeminiPresetGenerator) renderSettingsJSON(cfg *config.Config) (string, error) {
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

func (g *GeminiPresetGenerator) renderGeminiMarkdown(content *config.ContentTree, cfg *config.Config) string {
	var builder strings.Builder

	// Calculate content counts
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))

	// Add header before title
	header := generateGeminiPresetHeader(cfg, "GEMINI.md", len(allRules), 0, len(allAgents))
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

	// Add agents section listing available subagents (if agent-delegation builtin is enabled)
	renderAgentsSection(&builder, content, allAgents)

	// Skills are generated to .agents/skills/ directory, not inlined in GEMINI.md

	return builder.String()
}

// renderGeminiSkillFile renders a skill file in SKILL.md format for Gemini
func (g *GeminiPresetGenerator) renderGeminiSkillFile(skill config.ContentFile) string {
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

// renderGeminiAgentFile renders an agent file with YAML frontmatter for Gemini
func (g *GeminiPresetGenerator) renderGeminiAgentFile(agent config.ContentFile) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildGeminiAgentFrontmatter(agent)

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

// buildGeminiAgentFrontmatter builds frontmatter for a Gemini agent file
func (g *GeminiPresetGenerator) buildGeminiAgentFrontmatter(agent config.ContentFile) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	geminiScalarFields := []string{keyDescription, keyKind, keyModel, keyTemperature, "max_turns", "timeout_mins"}
	for _, field := range geminiScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
