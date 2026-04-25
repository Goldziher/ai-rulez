package presets

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
)

const presetNameClaude = "claude"

func init() {
	config.RegisterPreset(presetNameClaude, &ClaudePresetGenerator{})
}

// ClaudePresetGenerator generates Claude preset files
type ClaudePresetGenerator struct{}

// generatePresetHeader creates a header for Claude preset files.
// An optional styleOverride can be passed to force a specific header style
// (e.g., "minimal" for skills/agents where frontmatter must come first).
func generatePresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int, styleOverride ...string) string {
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

	if len(styleOverride) > 0 && styleOverride[0] != "" {
		data.StyleOverride = styleOverride[0]
	}

	return templates.GenerateHeader(data)
}

func (g *ClaudePresetGenerator) GetName() string {
	return presetNameClaude
}

func (g *ClaudePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "CLAUDE.md"),
		filepath.Join(baseDir, ".claude"),
		filepath.Join(baseDir, ".claude", "skills"),
		filepath.Join(baseDir, ".claude", "agents"),
	}
}

func (g *ClaudePresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .claude directory
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".claude"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".claude", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".claude", "agents"),
			IsDir: true,
		},
	)

	// Generate CLAUDE.md with all rules and context
	claudeMD := g.renderClaudeMarkdown(content, cfg)
	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, "CLAUDE.md"),
		Content: claudeMD,
	})

	// Generate skill files (root + domain skills)
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillOutputs, err := g.generateSkillFiles(skill, content, baseDir, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate skill %s: %w", skill.Name, err)
		}
		outputs = append(outputs, skillOutputs...)
	}

	// Generate agent files (root + domain agents)
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	if len(allAgents) > 0 {
		for _, agent := range allAgents {
			agentOutputs, err := g.generateAgentFiles(agent, content, baseDir, cfg)
			if err != nil {
				return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
			}
			outputs = append(outputs, agentOutputs...)
		}
	}

	// Generate command files as skills (root + domain commands)
	allCommands := combineContentFiles(content.Commands, getAllDomainCommands(content))
	logger.Info("Processing commands for Claude preset", "count", len(allCommands))
	if len(allCommands) > 0 {
		for _, command := range allCommands {
			logger.Info("Checking command", "name", command.Name, "has_metadata", command.Metadata != nil)

			// Check if command is enabled and targets Claude
			if g.shouldIncludeCommand(command) {
				logger.Info("Including command in Claude preset", "name", command.Name)
				commandOutputs, err := g.generateCommandAsSkill(command, content, baseDir, cfg)
				if err != nil {
					return nil, fmt.Errorf("generate command %s: %w", command.Name, err)
				}
				outputs = append(outputs, commandOutputs...)
			} else {
				logger.Info("Skipping command (filtered by targets)", "name", command.Name)
			}
		}
	}

	// Generate .claude/settings.json with MCP servers (if configured)
	if len(cfg.MCPServers) > 0 {
		settingsContent, err := g.renderSettingsJSON(cfg)
		if err != nil {
			return nil, fmt.Errorf("render settings.json: %w", err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".claude", "settings.json"),
			Content: settingsContent,
		})
	}

	// Generate .claude/plugins.json with plugin declarations (if configured)
	if len(cfg.Plugins) > 0 {
		pluginsContent, err := g.renderPluginsJSON(cfg)
		if err != nil {
			return nil, fmt.Errorf("render plugins.json: %w", err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".claude", "plugins.json"),
			Content: pluginsContent,
		})
	}

	return outputs, nil
}

func (g *ClaudePresetGenerator) generateSkillFiles(skill config.ContentFile, content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Extract skill ID from path
	skillID := extractSkillID(skill.Path)

	// Create skill directory
	skillDir := filepath.Join(baseDir, ".claude", "skills", skillID)
	outputs = append(outputs, config.OutputFile{
		Path:  skillDir,
		IsDir: true,
	})

	// Generate SKILL.md with frontmatter
	skillOutputPath := filepath.Join(skillDir, "SKILL.md")
	skillContent, err := g.renderSkillFile(skill, content, cfg, skillOutputPath)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFile{
		Path:    skillOutputPath,
		Content: skillContent,
	})

	return outputs, nil
}

//nolint:gocyclo // Acceptable complexity for comprehensive skill generation with target filtering
func (g *ClaudePresetGenerator) renderSkillFile(skill config.ContentFile, content *config.ContentTree, cfg *config.Config, outputPath string) (string, error) {
	var builder strings.Builder
	// Only embed rules/context that explicitly target this skill output path.
	// Untargeted rules are already in CLAUDE.md, so embedding them here would be duplication.
	includedRules := filterContentByExplicitTargets(content.Rules, outputPath, cfg.BaseDir)
	includedContext := filterContentByExplicitTargets(content.Context, outputPath, cfg.BaseDir)

	// Generate frontmatter — only Claude Code fields, no ai-rulez internals
	frontmatter := make(map[string]interface{})
	frontmatter["name"] = skill.Name
	frontmatter["user_invocable"] = false

	if skill.Metadata != nil {
		// Explicitly set description for Claude Code
		if desc, ok := skill.Metadata.Extra["description"]; ok && desc != "" {
			frontmatter["description"] = desc
		}
		// Include extra metadata, excluding ai-rulez internal fields
		for k, v := range skill.Metadata.Extra {
			if k != "priority" && k != "targets" && k != "name" && k != "description" {
				frontmatter[k] = v
			}
		}
	}

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}

	// Frontmatter MUST be first in the file for Claude Code to parse it.
	// No header comment — skill body is the prompt Claude sees when invoked.
	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	// Add rules section
	if len(includedRules) > 0 {
		builder.WriteString("\n\n## Rules\n\n")
		for _, rule := range includedRules {
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
	if len(includedContext) > 0 {
		builder.WriteString("\n## Context\n\n")
		for _, ctx := range includedContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")
			builder.WriteString(ctx.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String(), nil
}

func (g *ClaudePresetGenerator) generateAgentFiles(agent config.ContentFile, content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Extract agent ID from name
	agentID := sanitizeAgentID(agent.Name)

	// Generate agent file with Claude's agent format
	agentOutputPath := filepath.Join(baseDir, ".claude", "agents", agentID+".md")
	agentContent, err := g.renderAgentContent(agent, content, cfg, agentOutputPath)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFile{
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

//nolint:gocyclo // Acceptable complexity for comprehensive markdown generation
func (g *ClaudePresetGenerator) renderClaudeMarkdown(content *config.ContentTree, cfg *config.Config) string {
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

	// Add rules section - inline all content
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
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

			g.renderContentRef(&builder, rule, cfg)
		}
	}

	// Add context section - inline all content
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	if len(allContext) > 0 {
		builder.WriteString("## Context\n\n")
		for _, ctx := range allContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")

			// Add summary if available
			if ctx.Metadata != nil && ctx.Metadata.Extra["summary"] != "" {
				builder.WriteString(ctx.Metadata.Extra["summary"])
				builder.WriteString("\n\n")
			}

			g.renderContentRef(&builder, ctx, cfg)
		}
	}

	// Add agents section listing available subagents (if agent-delegation builtin is enabled)
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))
	renderAgentsSection(&builder, content, allAgents)

	// Skills are generated to .claude/skills/ directory, not inlined in CLAUDE.md
	// Commands are generated to .claude/skills/ directory as skills, not inlined in CLAUDE.md

	return builder.String()
}

func (g *ClaudePresetGenerator) renderAgentContent(agent config.ContentFile, content *config.ContentTree, cfg *config.Config, outputPath string) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildAgentFrontmatter(agent)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal agent frontmatter: %w", err)
	}

	// Only embed rules/context that explicitly target this agent output path.
	// Untargeted rules are already in CLAUDE.md, so embedding them here would be duplication.
	includedRules := filterContentByExplicitTargets(content.Rules, outputPath, cfg.BaseDir)
	includedContext := filterContentByExplicitTargets(content.Context, outputPath, cfg.BaseDir)

	// Frontmatter MUST be first in the file for Claude Code to parse it.
	// No header comment is emitted — everything after frontmatter becomes the
	// agent's system prompt, and generated-file warnings would waste tokens and
	// confuse the agent.
	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")

	// Add agent content directly (no header — this is a system prompt)
	builder.WriteString(agent.Content)

	// Add rules section if explicitly targeted rules exist
	if len(includedRules) > 0 {
		builder.WriteString("\n\n## Rules\n\n")
		for _, rule := range includedRules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n\n")
			builder.WriteString(rule.Content)
			builder.WriteString("\n\n")
		}
	}

	// Add context section if explicitly targeted context exists
	if len(includedContext) > 0 {
		builder.WriteString("\n## Context\n\n")
		for _, ctx := range includedContext {
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

func shouldIncludeInOutput(targets []string, outputPath, baseDir string) bool {
	if len(targets) == 0 {
		return true
	}

	candidates := buildOutputPathCandidates(outputPath, baseDir)
	for _, target := range targets {
		if targetMatchesOutput(target, candidates) {
			return true
		}
	}

	return false
}

// renderContentRef writes a content file's content inline.
func (g *ClaudePresetGenerator) renderContentRef(builder *strings.Builder, file config.ContentFile, cfg *config.Config) {
	processedContent := markdown.ProcessEmbeddedContent(file.Content)
	builder.WriteString(processedContent)
	builder.WriteString("\n\n")
}

func buildOutputPathCandidates(outputPath, baseDir string) []string {
	candidateSet := make(map[string]struct{})
	addCandidate := func(value string) {
		normalized := normalizeTargetPath(value)
		if normalized == "" {
			return
		}
		candidateSet[normalized] = struct{}{}
	}

	addCandidate(outputPath)
	addCandidate(filepath.Base(outputPath))

	baseDir = normalizeTargetPath(baseDir)
	outputPath = normalizeTargetPath(outputPath)
	if baseDir != "" {
		prefix := baseDir + "/"
		if strings.HasPrefix(outputPath, prefix) {
			addCandidate(strings.TrimPrefix(outputPath, prefix))
		}
	}

	candidates := make([]string, 0, len(candidateSet))
	for candidate := range candidateSet {
		candidates = append(candidates, candidate)
	}
	return candidates
}

func normalizeTargetPath(raw string) string {
	if raw == "" {
		return ""
	}

	hasTrailingSlash := strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "\\")
	normalized := filepath.ToSlash(filepath.Clean(raw))
	normalized = strings.TrimPrefix(normalized, "./")

	if normalized == "." {
		return ""
	}

	if hasTrailingSlash && normalized != "/" {
		normalized += "/"
	}

	return normalized
}

func targetMatchesOutput(target string, outputCandidates []string) bool {
	target = normalizeTargetPath(target)
	if target == "" {
		return false
	}

	if target == presetNameClaude {
		for _, candidate := range outputCandidates {
			if candidate == "CLAUDE.md" || strings.HasPrefix(candidate, ".claude/") {
				return true
			}
		}
		return false
	}

	if strings.HasSuffix(target, "/") {
		prefix := strings.TrimSuffix(target, "/")
		for _, candidate := range outputCandidates {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				return true
			}
		}
		return false
	}

	for _, candidate := range outputCandidates {
		if candidate == target {
			return true
		}

		if strings.ContainsAny(target, "*?[") {
			matched, err := path.Match(target, candidate)
			if err == nil && matched {
				return true
			}
		}
	}

	return false
}

// shouldIncludeCommand checks if a command should be included in the Claude preset
func (g *ClaudePresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if command.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Claude is in targets
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == presetNameClaude {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// generateCommandAsSkill generates a command file as a Claude skill
func (g *ClaudePresetGenerator) generateCommandAsSkill(command config.ContentFile, content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Extract command ID from name (sanitize for use as directory name)
	commandID := sanitizeAgentID(command.Name)

	// Create skill directory for command
	skillDir := filepath.Join(baseDir, ".claude", "skills", commandID)
	outputs = append(outputs, config.OutputFile{
		Path:  skillDir,
		IsDir: true,
	})

	// Generate SKILL.md with frontmatter
	skillOutputPath := filepath.Join(skillDir, "SKILL.md")
	skillContent, err := g.renderCommandAsSkill(command, content, cfg, skillOutputPath)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFile{
		Path:    skillOutputPath,
		Content: skillContent,
	})

	return outputs, nil
}

// renderCommandAsSkill renders a command as a Claude skill
//
//nolint:gocyclo // Acceptable complexity for comprehensive skill generation
func (g *ClaudePresetGenerator) renderCommandAsSkill(command config.ContentFile, content *config.ContentTree, cfg *config.Config, outputPath string) (string, error) {
	var builder strings.Builder

	// Generate frontmatter — commands are user-invocable slash commands
	frontmatter := make(map[string]interface{})
	frontmatter["name"] = command.Name
	frontmatter["user_invocable"] = true

	if command.Metadata != nil {
		// Explicitly set description for Claude Code
		if desc, ok := command.Metadata.Extra["description"]; ok && desc != "" {
			frontmatter["description"] = desc
		}
		if command.Metadata.Shortcut != "" {
			frontmatter["shortcut"] = command.Metadata.Shortcut
		}
		if command.Metadata.Category != "" {
			frontmatter["category"] = command.Metadata.Category
		}
		// Include extra metadata, excluding ai-rulez internal fields
		for k, v := range command.Metadata.Extra {
			if k != "priority" && k != "targets" && k != "name" && k != "shortcut" && k != "category" && k != "description" {
				frontmatter[k] = v
			}
		}
	}

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal command frontmatter: %w", err)
	}

	// Only embed rules/context that explicitly target this command output path.
	// Untargeted rules are already in CLAUDE.md, so embedding them here would be duplication.
	includedRules := filterContentByExplicitTargets(content.Rules, outputPath, cfg.BaseDir)
	includedContext := filterContentByExplicitTargets(content.Context, outputPath, cfg.BaseDir)

	// Frontmatter MUST be first in the file for Claude Code to parse it.
	// No header comment — command body is the prompt Claude sees when invoked.
	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")

	// Add command content
	builder.WriteString(command.Content)

	// Add rules section if explicitly targeted rules exist
	if len(includedRules) > 0 {
		builder.WriteString("\n\n## Rules\n\n")
		for _, rule := range includedRules {
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

	// Add context section if explicitly targeted context exists
	if len(includedContext) > 0 {
		builder.WriteString("\n## Context\n\n")
		for _, ctx := range includedContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")
			builder.WriteString(ctx.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String(), nil
}

// renderSettingsJSON generates .claude/settings.json with MCP server configuration
func (g *ClaudePresetGenerator) renderSettingsJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			"command": server.Command,
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
			entry["disabled"] = true
		}

		mcpServers[name] = entry
	}

	settings := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	jsonBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal settings JSON: %w", err)
	}

	return string(jsonBytes) + "\n", nil
}

// renderPluginsJSON generates .claude/plugins.json with plugin declarations
func (g *ClaudePresetGenerator) renderPluginsJSON(cfg *config.Config) (string, error) {
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
