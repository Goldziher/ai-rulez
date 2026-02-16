package presets

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/compression"
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/markdown"
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

	// Generate command files as skills
	logger.Info("Processing commands for Claude preset", "count", len(content.Commands))
	if len(content.Commands) > 0 {
		for _, command := range content.Commands {
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

//nolint:gocyclo // Acceptable complexity for comprehensive skill generation with target filtering
func (g *ClaudePresetGenerator) renderSkillFile(skill config.ContentFile, content *config.ContentTreeV3, cfg *config.ConfigV3, outputPath string) (string, error) {
	var builder strings.Builder
	var includedRules []config.ContentFile
	var includedContext []config.ContentFile

	for _, rule := range content.Rules {
		var targets []string
		if rule.Metadata != nil {
			targets = rule.Metadata.Targets
		}

		if shouldIncludeInOutput(targets, outputPath, cfg.BaseDir) {
			includedRules = append(includedRules, rule)
		}
	}

	for _, ctx := range content.Context {
		var targets []string
		if ctx.Metadata != nil {
			targets = ctx.Metadata.Targets
		}

		if shouldIncludeInOutput(targets, outputPath, cfg.BaseDir) {
			includedContext = append(includedContext, ctx)
		}
	}

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
	header := generatePresetHeader(cfg, outputPath, len(includedRules), 0, len(content.Agents))
	builder.WriteString(header)

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

//nolint:gocyclo // Acceptable complexity for comprehensive markdown generation
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
			builder.WriteString("\n\n") // Add blank line after heading

			if rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}

			// Process the content to remove the H1 and normalize formatting
			processedContent := markdown.ProcessEmbeddedContent(rule.Content)
			builder.WriteString(processedContent)
			builder.WriteString("\n\n")
		}
	}

	// Add context section - render as summaries with @ links
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

			// Add @ link to full file
			contextPath := strings.TrimPrefix(ctx.Path, cfg.BaseDir+"/")
			builder.WriteString("@")
			builder.WriteString(contextPath)
			builder.WriteString("\n\n")
		}
	}

	// Skills are generated to .claude/skills/ directory, not inlined in CLAUDE.md
	// Commands are generated to .claude/skills/ directory as skills, not inlined in CLAUDE.md

	result := builder.String()

	// Apply compression if configured
	if cfg.Compression != nil {
		compressor := compression.NewCompressor(cfg.Compression)
		result = compressor.Compress(result)

		stats := compressor.GetStats()
		logger.Info("CLAUDE.md compression",
			"level", cfg.Compression.GetCompressionLevel(),
			"original", stats.OriginalSize,
			"compressed", stats.CompressedSize,
			"saved_percent", fmt.Sprintf("%.1f%%", stats.Ratio))
	}

	return result
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

	if target == "claude" {
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
			if target == "claude" {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// generateCommandAsSkill generates a command file as a Claude skill
func (g *ClaudePresetGenerator) generateCommandAsSkill(command config.ContentFile, content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Extract command ID from name (sanitize for use as directory name)
	commandID := sanitizeAgentID(command.Name)

	// Create skill directory for command
	skillDir := filepath.Join(baseDir, ".claude", "skills", commandID)
	outputs = append(outputs, config.OutputFileV3{
		Path:  skillDir,
		IsDir: true,
	})

	// Generate SKILL.md with frontmatter
	skillOutputPath := filepath.Join(skillDir, "SKILL.md")
	skillContent, err := g.renderCommandAsSkill(command, content, cfg, skillOutputPath)
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFileV3{
		Path:    skillOutputPath,
		Content: skillContent,
	})

	return outputs, nil
}

// renderCommandAsSkill renders a command as a Claude skill
//
//nolint:gocyclo // Acceptable complexity for comprehensive skill generation
func (g *ClaudePresetGenerator) renderCommandAsSkill(command config.ContentFile, content *config.ContentTreeV3, cfg *config.ConfigV3, outputPath string) (string, error) {
	var builder strings.Builder

	// Generate frontmatter
	frontmatter := make(map[string]interface{})
	frontmatter["name"] = command.Name

	if command.Metadata != nil {
		if command.Metadata.Priority != "" {
			frontmatter["priority"] = command.Metadata.Priority
		}
		if command.Metadata.Shortcut != "" {
			frontmatter["shortcut"] = command.Metadata.Shortcut
		}
		if command.Metadata.Category != "" {
			frontmatter["category"] = command.Metadata.Category
		}
		if len(command.Metadata.Targets) > 0 {
			frontmatter["targets"] = command.Metadata.Targets
		}
		// Include extra metadata
		for k, v := range command.Metadata.Extra {
			if k != "priority" && k != "targets" && k != "name" && k != "shortcut" && k != "category" {
				frontmatter[k] = v
			}
		}
	}

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal command frontmatter: %w", err)
	}

	// Add header before frontmatter
	header := generatePresetHeader(cfg, outputPath, len(content.Rules), 0, len(content.Agents))
	builder.WriteString(header)

	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")

	// Add command content
	builder.WriteString(command.Content)

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
