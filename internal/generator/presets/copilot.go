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

const presetNameCopilot = "copilot"

func init() {
	config.RegisterPreset(presetNameCopilot, &CopilotPresetGenerator{})
}

// CopilotPresetGenerator generates GitHub Copilot preset files
type CopilotPresetGenerator struct{}

// generateCopilotPresetHeader creates a header for Copilot preset files
func generateCopilotPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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
	return presetNameCopilot
}

func (g *CopilotPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".github"),
		filepath.Join(baseDir, ".github", "copilot-instructions.md"),
		filepath.Join(baseDir, ".github", "skills"),
		filepath.Join(baseDir, ".github", "agents"),
		filepath.Join(baseDir, ".github", "commands"),
	}
}

func (g *CopilotPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .github directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".github"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".github", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".github", "agents"),
			IsDir: true,
		},
	)

	// Generate copilot-instructions.md
	instructionsContent := g.renderInstructionsFile(content, cfg)
	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, ".github", "copilot-instructions.md"),
		Content: instructionsContent,
	})

	// Generate skill files to .github/skills/
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".github", "skills", skillID)
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

	// Generate agent files to .github/agents/ (uses .agent.md extension)
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderCopilotAgentFile(agent)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".github", "agents", agentID+".agent.md"),
			Content: agentContent,
		})
	}

	// Generate .github/commands directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".github", "commands"),
		IsDir: true,
	})

	// Generate command files to .github/commands/
	allCommands := combineContentFiles(content.Commands, getAllDomainCommands(content))
	for _, command := range allCommands {
		if !g.shouldIncludeCommand(command) {
			continue
		}
		sanitized := sanitizeName(command.Name)
		commandContent := g.renderCommandFile(command)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".github", "commands", sanitized+".md"),
			Content: commandContent,
		})
	}

	// Generate .mcp.json if MCP servers are configured
	if len(cfg.MCPServers) > 0 {
		mcpContent, err := g.renderMCPJSON(cfg)
		if err != nil {
			return nil, fmt.Errorf("render .mcp.json: %w", err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".mcp.json"),
			Content: mcpContent,
		})
	}

	return outputs, nil
}

func (g *CopilotPresetGenerator) renderInstructionsFile(content *config.ContentTree, cfg *config.Config) string {
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

	// Skills are generated to .github/skills/ directory, not inlined

	return builder.String()
}

// renderSkillFile renders a skill file in SKILL.md format for Copilot
func (g *CopilotPresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

// renderCopilotAgentFile renders an agent file with YAML frontmatter for Copilot
func (g *CopilotPresetGenerator) renderCopilotAgentFile(agent config.ContentFile) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildCopilotAgentFrontmatter(agent)

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

// buildCopilotAgentFrontmatter builds frontmatter for a Copilot agent file
func (g *CopilotPresetGenerator) buildCopilotAgentFrontmatter(agent config.ContentFile) map[string]interface{} {
	frontmatter := map[string]interface{}{
		"name": agent.Name,
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	copilotFields := []string{
		"description", "tools", "model", "target",
		"user-invocable", "disable-model-invocation",
		"agents", "handoffs", "mcp-servers",
	}
	for _, field := range copilotFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}

	return frontmatter
}

// shouldIncludeCommand checks if a command should be included in the Copilot preset
func (g *CopilotPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	if command.Metadata == nil {
		return true
	}
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == presetNameCopilot {
				return true
			}
		}
		return false
	}
	return true
}

// renderCommandFile renders a command file in Markdown format for Copilot
func (g *CopilotPresetGenerator) renderCommandFile(command config.ContentFile) string {
	var builder strings.Builder

	builder.WriteString("# /")
	builder.WriteString(command.Name)
	builder.WriteString("\n\n")

	if command.Metadata != nil && command.Metadata.Extra != nil {
		if desc, ok := command.Metadata.Extra["description"]; ok && desc != "" {
			builder.WriteString("**Description:** ")
			builder.WriteString(desc)
			builder.WriteString("\n\n")
		}
	}

	if command.Metadata != nil && command.Metadata.Usage != "" {
		builder.WriteString("**Usage:** `")
		builder.WriteString(command.Metadata.Usage)
		builder.WriteString("`\n\n")
	}

	processedContent := markdown.ProcessEmbeddedContent(command.Content)
	builder.WriteString(processedContent)

	return builder.String()
}

// renderMCPJSON renders .mcp.json with MCP server configuration
func (g *CopilotPresetGenerator) renderMCPJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			"command":  server.Command,
			"disabled": !server.IsEnabled(),
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

		mcpServers[name] = entry
	}

	payload := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal MCP JSON: %w", err)
	}

	return string(jsonBytes) + "\n", nil
}
