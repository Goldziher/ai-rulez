package presets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPreset("cursor", &CursorPresetGenerator{})
}

const presetNameCursor = "cursor"

// CursorPresetGenerator generates Cursor preset files
type CursorPresetGenerator struct{}

func (g *CursorPresetGenerator) GetName() string {
	return presetNameCursor
}

func (g *CursorPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".cursor"),
		filepath.Join(baseDir, ".cursor", "rules"),
		filepath.Join(baseDir, ".cursor", "commands"),
		filepath.Join(baseDir, ".agents"),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, ".agents", "agents"),
	}
}

func (g *CursorPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cursor"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cursor", "rules"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cursor", "commands"),
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

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		ruleContent := g.renderRuleFile(rule)
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".cursor", "rules", sanitized+".mdc"),
			Content: ruleContent,
		})
	}

	// Combine all commands from root and domains
	allCommands := combineContentFiles(content.Commands, getAllDomainCommands(content))

	// Generate command files to .cursor/commands/
	for _, command := range allCommands {
		// Check if command should be included (enabled and targets Cursor if specified)
		if g.shouldIncludeCommand(command) {
			commandContent := g.renderCommandFile(command)
			sanitized := sanitizeName(command.Name)

			outputs = append(outputs, config.OutputFile{
				Path:    filepath.Join(baseDir, ".cursor", "commands", sanitized+".md"),
				Content: commandContent,
			})
		}
	}

	// Combine all skills from root and domains
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))

	// Generate skill files to .agents/skills/
	for _, skill := range allSkills {
		// Check if skill should be included (enabled and targets Cursor if specified)
		if !g.shouldIncludeSkill(skill) {
			continue
		}

		skillID := extractSkillID(skill.Path)

		// Create skill directory
		skillDir := filepath.Join(baseDir, ".agents", "skills", skillID)
		outputs = append(outputs, config.OutputFile{
			Path:  skillDir,
			IsDir: true,
		})

		// Generate SKILL.md file
		skillContent := g.renderSkillFile(skill, cfg)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})

		// Emit bundled resources alongside SKILL.md.
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate agent files to .agents/agents/
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderCursorAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".agents", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	// Generate context files as rule-like files in .cursor/rules/
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
			Path:    filepath.Join(baseDir, ".cursor", "rules", "context-"+sanitized+".mdc"),
			Content: ctxBuilder.String(),
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

func (g *CursorPresetGenerator) renderRuleFile(rule config.ContentFile) string {
	var builder strings.Builder

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
	builder.WriteString(rule.Content)

	return builder.String()
}

// shouldIncludeCommand checks if a command should be included in the Cursor preset
func (g *CursorPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if command.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Cursor is in targets
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == presetNameCursor {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// renderCommandFile renders a command file in Markdown format for Cursor commands
func (g *CursorPresetGenerator) renderCommandFile(command config.ContentFile) string {
	var builder strings.Builder

	// Add title
	builder.WriteString("# /")
	builder.WriteString(command.Name)
	builder.WriteString("\n\n")

	// Add description if present in metadata
	if command.Metadata != nil && command.Metadata.Extra != nil {
		if desc, ok := command.Metadata.Extra["description"]; ok && desc != "" {
			builder.WriteString("**Description:** ")
			builder.WriteString(desc)
			builder.WriteString("\n\n")
		}
	}

	// Add usage if present in metadata
	if command.Metadata != nil && command.Metadata.Usage != "" {
		builder.WriteString("**Usage:** `")
		builder.WriteString(command.Metadata.Usage)
		builder.WriteString("`\n\n")
	}

	// Add aliases if present in metadata
	if command.Metadata != nil && len(command.Metadata.Aliases) > 0 {
		builder.WriteString("**Aliases:** ")
		for i, alias := range command.Metadata.Aliases {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("`/")
			builder.WriteString(alias)
			builder.WriteString("`")
		}
		builder.WriteString("\n\n")
	}

	// Add command content
	builder.WriteString(command.Content)

	return builder.String()
}

// shouldIncludeSkill checks if a skill should be included in the Cursor preset
func (g *CursorPresetGenerator) shouldIncludeSkill(skill config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if skill.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Cursor is in targets
	if len(skill.Metadata.Targets) > 0 {
		for _, target := range skill.Metadata.Targets {
			if target == presetNameCursor {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// renderSkillFile renders a skill file in SKILL.md format for Cursor
func (g *CursorPresetGenerator) renderSkillFile(skill config.ContentFile, cfg *config.Config) string {
	var builder strings.Builder

	// Add YAML frontmatter
	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")

	if skill.Metadata != nil {
		if skill.Metadata.Priority != "" {
			builder.WriteString("priority: ")
			builder.WriteString(skill.Metadata.Priority)
			builder.WriteString("\n")
		}

		if desc, ok := skill.Metadata.Extra["description"]; ok && desc != "" {
			builder.WriteString("description: ")
			builder.WriteString(quoteYAMLString(desc))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	// Index bundled resources so the agent knows what to read on demand.
	builder.WriteString(RenderSkillResourcesIndex(&skill))

	return builder.String()
}

// renderCursorAgentFile renders an agent file with YAML frontmatter for Cursor
func (g *CursorPresetGenerator) renderCursorAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildCursorAgentFrontmatter(agent, cfg)

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

// buildCursorAgentFrontmatter builds frontmatter for a Cursor agent file
func (g *CursorPresetGenerator) buildCursorAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := ResolveAgentModel(presetNameCursor, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	cursorFields := []string{keyDescription, "readonly", "is_background"}
	for _, field := range cursorFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}

	return frontmatter
}

// renderMCPJSON renders .mcp.json with MCP server configuration
func (g *CursorPresetGenerator) renderMCPJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			keyCommand:  server.Command,
			keyDisabled: !server.IsEnabled(),
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
		keyMCPServers: mcpServers,
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal MCP JSON: %w", err)
	}

	return string(jsonBytes) + "\n", nil
}
