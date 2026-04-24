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
		filepath.Join(baseDir, ".codex", "agents"),
		filepath.Join(baseDir, ".codex", "commands"),
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
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".codex", "agents"),
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

	// Generate agent files to .codex/agents/ (TOML format)
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent := g.renderAgentTOML(agent)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".codex", "agents", agentID+".toml"),
			Content: agentContent,
		})
	}

	// Generate .codex/commands directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".codex", "commands"),
		IsDir: true,
	})

	// Generate command files to .codex/commands/
	allCommands := combineContentFiles(content.Commands, getAllDomainCommands(content))
	for _, command := range allCommands {
		if !g.shouldIncludeCommand(command) {
			continue
		}
		sanitized := sanitizeName(command.Name)
		commandContent := g.renderCommandFile(command)
		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".codex", "commands", sanitized+".md"),
			Content: commandContent,
		})
	}

	// Generate .codex/plugins.json with plugin declarations (if configured)
	if len(cfg.Plugins) > 0 {
		pluginsContent, err := g.renderPluginsJSON(cfg)
		if err != nil {
			return nil, fmt.Errorf("render plugins.json: %w", err)
		}
		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".codex", "plugins.json"),
			Content: pluginsContent,
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

	// Add agents section listing available subagents (if agent-delegation builtin is enabled)
	renderAgentsSection(&builder, content, allAgents)

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
	builder.WriteString(quoteYAMLString(config.SkillDescriptionForContent(skill)))
	builder.WriteString("\n")

	if skill.Metadata != nil {
		// Add short-description for user-facing display.
		if shortDesc := config.SkillShortDescription(skill.Metadata); shortDesc != "" {
			builder.WriteString("metadata:\n")
			builder.WriteString("  short-description: ")
			builder.WriteString(quoteYAMLString(shortDesc))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	return builder.String()
}

// renderAgentTOML renders an agent file in TOML format for Codex
func (g *CodexPresetGenerator) renderAgentTOML(agent config.ContentFile) string {
	var builder strings.Builder

	builder.WriteString("name = ")
	builder.WriteString(quoteTOMLString(agent.Name))
	builder.WriteString("\n")

	description := ""
	if agent.Metadata != nil {
		if desc, ok := agent.Metadata.Extra["description"]; ok {
			description = desc
		}
	}
	builder.WriteString("description = ")
	builder.WriteString(quoteTOMLString(description))
	builder.WriteString("\n")

	builder.WriteString("developer_instructions = ")
	builder.WriteString(quoteTOMLMultiline(agent.Content))
	builder.WriteString("\n")

	return builder.String()
}

// quoteTOMLString quotes a string for TOML single-line values
func quoteTOMLString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	return "\"" + escaped + "\""
}

// quoteTOMLMultiline quotes a string as a TOML multi-line basic string.
// Per the TOML spec, inside """-delimited strings we must escape any sequence
// of three or more consecutive double-quotes. We do this by inserting a
// backslash-escape before the third quote in every run of 3+ quotes.
func quoteTOMLMultiline(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	// Break runs of 3+ quotes: `"""` → `""\"`  (the `\"` restarts counting)
	for strings.Contains(escaped, "\"\"\"") {
		escaped = strings.Replace(escaped, "\"\"\"", "\"\"\\\"", 1)
	}
	return "\"\"\"\n" + escaped + "\n\"\"\""
}

func quoteYAMLString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	return "\"" + escaped + "\""
}

// shouldIncludeCommand checks if a command should be included in the Codex preset
func (g *CodexPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	if command.Metadata == nil {
		return true
	}
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == "codex" {
				return true
			}
		}
		return false
	}
	return true
}

// renderCommandFile renders a command file in Markdown format for Codex
func (g *CodexPresetGenerator) renderCommandFile(command config.ContentFile) string {
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

// renderPluginsJSON generates .codex/plugins.json with plugin declarations
func (g *CodexPresetGenerator) renderPluginsJSON(cfg *config.ConfigV3) (string, error) {
	type pluginEntry struct {
		Marketplace string `json:"marketplace"`
		Name        string `json:"name"`
		Scope       string `json:"scope"`
		Enabled     bool   `json:"enabled"`
	}

	var plugins []pluginEntry
	for _, p := range cfg.Plugins {
		plugins = append(plugins, pluginEntry{
			Marketplace: p.Marketplace,
			Name:        p.Name,
			Scope:       p.GetScope(),
			Enabled:     p.IsEnabled(),
		})
	}

	jsonBytes, err := json.MarshalIndent(plugins, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal plugins JSON: %w", err)
	}

	return string(jsonBytes) + "\n", nil
}
