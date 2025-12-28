package migration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/importer"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// V2ToV3Migrator handles migration from V2 (ai-rulez.yaml) to V3 (.ai-rulez/ directory)
type V2ToV3Migrator struct {
	v2ConfigPath string
	outputDir    string
	logger       *slog.Logger
}

// NewV2ToV3Migrator creates a new V2 to V3 migrator
func NewV2ToV3Migrator(v2ConfigPath, outputDir string) *V2ToV3Migrator {
	return &V2ToV3Migrator{
		v2ConfigPath: v2ConfigPath,
		outputDir:    outputDir,
		logger:       logger.Get(),
	}
}

// Migrate performs the full V2 to V3 migration
func (m *V2ToV3Migrator) Migrate(ctx context.Context) error {
	m.logger.Info("Starting V2 to V3 migration", "source", m.v2ConfigPath, "output", m.outputDir)

	// 1. Load V2 config (fully resolved with extends/includes)
	v2Config, err := m.loadV2Config(ctx)
	if err != nil {
		return oops.
			With("v2_config_path", m.v2ConfigPath).
			Hint("Ensure the V2 config file exists and is valid YAML\nRun 'ai-rulez validate' on the V2 config first").
			Wrapf(err, "load V2 config")
	}

	m.logger.Debug("Loaded V2 config",
		"rules", len(v2Config.Rules),
		"sections", len(v2Config.Sections),
		"agents", len(v2Config.Agents),
		"presets", len(v2Config.Presets))

	// 2. Create .ai-rulez/ directory structure
	aiRulezDir := filepath.Join(m.outputDir, ".ai-rulez")
	if err := m.createDirectoryStructure(aiRulezDir); err != nil {
		return oops.
			With("output_dir", aiRulezDir).
			Wrapf(err, "create directory structure")
	}

	// 3. Convert rules to .ai-rulez/rules/{name}.md
	if err := m.convertRules(v2Config.Rules, aiRulezDir); err != nil {
		return oops.
			With("rules_count", len(v2Config.Rules)).
			Wrapf(err, "convert rules")
	}

	// 4. Convert sections to .ai-rulez/context/{name}.md
	if err := m.convertSections(v2Config.Sections, aiRulezDir); err != nil {
		return oops.
			With("sections_count", len(v2Config.Sections)).
			Wrapf(err, "convert sections")
	}

	// 5. Convert agents to .ai-rulez/skills/{id}/SKILL.md
	if err := m.convertAgents(v2Config.Agents, aiRulezDir); err != nil {
		return oops.
			With("agents_count", len(v2Config.Agents)).
			Wrapf(err, "convert agents")
	}

	// 6. Migrate MCP servers to .ai-rulez/mcp.yaml
	if len(v2Config.MCPServers) > 0 {
		if err := importer.MigrateMCPServers(v2Config.MCPServers, aiRulezDir); err != nil {
			m.logger.Warn("Failed to migrate MCP servers", "error", err)
		}
	}

	// 7. Generate .ai-rulez/config.yaml with detected presets
	detectedPresets := m.detectPresets(v2Config)
	if err := m.generateV3Config(v2Config, detectedPresets, aiRulezDir); err != nil {
		return oops.
			With("presets", detectedPresets).
			Wrapf(err, "generate V3 config")
	}

	// 8. Validate by test-generating
	if err := m.validateMigration(ctx, aiRulezDir); err != nil {
		return oops.
			With("output_dir", aiRulezDir).
			Hint("Review the generated V3 configuration and content files for errors").
			Wrapf(err, "validate migrated configuration")
	}

	m.logger.Info("V2 to V3 migration completed successfully",
		"output_dir", aiRulezDir,
		"rules", len(v2Config.Rules),
		"sections", len(v2Config.Sections),
		"agents", len(v2Config.Agents),
		"presets", detectedPresets)

	return nil
}

// loadV2Config loads the V2 config with all includes/extends resolved
func (m *V2ToV3Migrator) loadV2Config(ctx context.Context) (*config.Config, error) {
	absPath, err := filepath.Abs(m.v2ConfigPath)
	if err != nil {
		return nil, oops.
			With("path", m.v2ConfigPath).
			Wrapf(err, "resolve absolute path")
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil {
		return nil, oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Ensure the file exists: %s", absPath)).
			Wrapf(err, "stat config file")
	}

	// Load config with includes/extends resolved
	cfg, err := config.LoadConfigWithIncludes(ctx, absPath)
	if err != nil {
		return nil, oops.
			With("path", absPath).
			Wrapf(err, "load config with includes")
	}

	return cfg, nil
}

// createDirectoryStructure creates the .ai-rulez/ directory structure
func (m *V2ToV3Migrator) createDirectoryStructure(aiRulezDir string) error {
	dirs := []string{
		aiRulezDir,
		filepath.Join(aiRulezDir, "rules"),
		filepath.Join(aiRulezDir, "context"),
		filepath.Join(aiRulezDir, "skills"),
		filepath.Join(aiRulezDir, "agents"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return oops.
				With("path", dir).
				Hint("Check write permissions on the output directory").
				Wrapf(err, "create directory")
		}
		m.logger.Debug("Created directory", "path", dir)
	}

	return nil
}

// convertRules converts V2 rules to V3 rules/*.md files
func (m *V2ToV3Migrator) convertRules(rules []config.Rule, aiRulezDir string) error {
	if len(rules) == 0 {
		m.logger.Debug("No rules to convert")
		return nil
	}

	rulesDir := filepath.Join(aiRulezDir, "rules")

	for i := range rules {
		filename := utils.SanitizeName(rules[i].Name) + ".md"
		filePath := filepath.Join(rulesDir, filename)

		content := m.buildRuleContent(&rules[i])

		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			return oops.
				With("path", filePath).
				With("rule_name", rules[i].Name).
				Wrapf(err, "write rule file")
		}

		m.logger.Debug("Converted rule", "name", rules[i].Name, "path", filePath)
	}

	m.logger.Info("Converted rules", "count", len(rules))
	return nil
}

// buildRuleContent builds the markdown content for a rule with frontmatter
func (m *V2ToV3Migrator) buildRuleContent(rule *config.Rule) string {
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	if rule.Priority != "" {
		sb.WriteString(fmt.Sprintf("priority: %s\n", rule.Priority))
	} else {
		sb.WriteString("priority: medium\n")
	}
	if len(rule.Targets) > 0 {
		sb.WriteString("targets:\n")
		for _, target := range rule.Targets {
			sb.WriteString(fmt.Sprintf("  - %s\n", target))
		}
	}
	sb.WriteString("---\n\n")

	// Write title
	sb.WriteString(fmt.Sprintf("# %s\n\n", rule.Name))

	// Write content
	sb.WriteString(strings.TrimSpace(rule.Content))
	sb.WriteString("\n")

	return sb.String()
}

// convertSections converts V2 sections to V3 skills/{id}/SKILL.md files
func (m *V2ToV3Migrator) convertSections(sections []config.Section, aiRulezDir string) error {
	if len(sections) == 0 {
		m.logger.Debug("No sections to convert")
		return nil
	}

	skillsDir := filepath.Join(aiRulezDir, "skills")

	for i := range sections {
		// Use ID if available, otherwise use name
		baseName := sections[i].ID
		if baseName == "" {
			baseName = sections[i].Name
		}

		skillID := utils.SanitizeName(baseName)
		skillDirPath := filepath.Join(skillsDir, skillID)

		// Create skill directory
		if err := os.MkdirAll(skillDirPath, 0o755); err != nil {
			return oops.
				With("path", skillDirPath).
				With("section_name", sections[i].Name).
				Wrapf(err, "create skill directory")
		}

		filePath := filepath.Join(skillDirPath, "SKILL.md")
		content := m.buildSectionContent(&sections[i])

		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			return oops.
				With("path", filePath).
				With("section_name", sections[i].Name).
				Wrapf(err, "write skill file")
		}

		m.logger.Debug("Converted section to skill", "name", sections[i].Name, "path", filePath)
	}

	m.logger.Info("Converted sections to skills", "count", len(sections))
	return nil
}

// buildSectionContent builds the markdown content for a section with frontmatter
func (m *V2ToV3Migrator) buildSectionContent(section *config.Section) string {
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	if section.Priority != "" {
		sb.WriteString(fmt.Sprintf("priority: %s\n", section.Priority))
	} else {
		sb.WriteString("priority: medium\n")
	}
	if len(section.Targets) > 0 {
		sb.WriteString("targets:\n")
		for _, target := range section.Targets {
			sb.WriteString(fmt.Sprintf("  - %s\n", target))
		}
	}
	sb.WriteString("---\n\n")

	// Write title (use name if available)
	if section.Name != "" {
		sb.WriteString(fmt.Sprintf("# %s\n\n", section.Name))
	}

	// Write content
	sb.WriteString(strings.TrimSpace(section.Content))
	sb.WriteString("\n")

	return sb.String()
}

// convertAgents converts V2 agents to V3 agents/{name}.md files
func (m *V2ToV3Migrator) convertAgents(agents []config.Agent, aiRulezDir string) error {
	if len(agents) == 0 {
		m.logger.Debug("No agents to convert")
		return nil
	}

	agentsDir := filepath.Join(aiRulezDir, "agents")

	for i := range agents {
		// Use name for file name
		agentID := utils.SanitizeName(agents[i].Name)
		if agentID == "" {
			agentID = utils.SanitizeName(agents[i].ID)
		}

		filename := agentID + ".md"
		filePath := filepath.Join(agentsDir, filename)
		content := m.buildAgentContent(&agents[i])

		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			return oops.
				With("path", filePath).
				With("agent_name", agents[i].Name).
				Wrapf(err, "write agent file")
		}

		m.logger.Debug("Converted agent", "name", agents[i].Name, "path", filePath)
	}

	m.logger.Info("Converted agents", "count", len(agents))
	return nil
}

// buildAgentContent builds the markdown content for an agent with Claude's agent format
func (m *V2ToV3Migrator) buildAgentContent(agent *config.Agent) string {
	var sb strings.Builder

	// Write frontmatter in Claude's agent format
	sb.WriteString("---\n")
	// Use Name if available, otherwise use ID
	agentName := agent.Name
	if agentName == "" {
		agentName = agent.ID
	}
	sb.WriteString(fmt.Sprintf("name: %s\n", agentName))

	if agent.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", agent.Description))
	}

	if agent.Model != "" {
		sb.WriteString(fmt.Sprintf("model: %s\n", agent.Model))
	}

	sb.WriteString("---\n\n")

	// Write title
	if agent.Name != "" {
		sb.WriteString(fmt.Sprintf("# %s\n\n", agent.Name))
	} else {
		sb.WriteString(fmt.Sprintf("# %s\n\n", agent.ID))
	}

	// Write system prompt if available
	if agent.SystemPrompt != "" {
		sb.WriteString(strings.TrimSpace(agent.SystemPrompt))
		sb.WriteString("\n")
	}

	return sb.String()
}

// detectPresets analyzes V2 config and detects which presets to include in V3
//
//nolint:gocyclo // Complex logic, acceptable for this use case
func (m *V2ToV3Migrator) detectPresets(v2Config *config.Config) []string {
	presetSet := make(map[string]bool)

	// Analyze explicit presets in V2
	for _, preset := range v2Config.Presets {
		preset = strings.ToLower(strings.TrimSpace(preset))

		// Map V2 preset to V3 presets
		switch preset {
		case "popular":
			// Expand popular to individual presets
			presetSet["claude"] = true
			presetSet["cursor"] = true
			presetSet["gemini"] = true
			presetSet["windsurf"] = true
			presetSet["copilot"] = true
		case "claude", "cursor", "gemini", "windsurf", "copilot", "continue-dev", "cline", "amp", "codex", "junie":
			presetSet[preset] = true
		default:
			m.logger.Warn("Unknown preset in V2 config, skipping", "preset", preset)
		}
	}

	// Analyze outputs to detect additional presets
	for _, output := range v2Config.Outputs {
		path := strings.ToLower(output.Path)

		if strings.Contains(path, "claude.md") {
			presetSet["claude"] = true
		}
		if strings.Contains(path, ".cursor/rules") {
			presetSet["cursor"] = true
		}
		if strings.Contains(path, "gemini.md") || strings.Contains(path, ".gemini/") {
			presetSet["gemini"] = true
		}
		if strings.Contains(path, ".windsurf/") {
			presetSet["windsurf"] = true
		}
		if strings.Contains(path, "copilot-instructions.md") {
			presetSet["copilot"] = true
		}
		if strings.Contains(path, ".continue/") {
			presetSet["continue-dev"] = true
		}
		if strings.Contains(path, ".clinerules/") {
			presetSet["cline"] = true
		}
		if strings.Contains(path, "agents.md") {
			// Could be amp, codex, or opencode - default to amp
			if !presetSet["amp"] && !presetSet["codex"] {
				presetSet["amp"] = true
			}
		}
		if strings.Contains(path, ".junie/") {
			presetSet["junie"] = true
		}
	}

	// If no presets detected, default to claude
	if len(presetSet) == 0 {
		m.logger.Info("No presets detected, defaulting to claude")
		presetSet["claude"] = true
	}

	// Convert to sorted slice
	presets := make([]string, 0, len(presetSet))
	for preset := range presetSet {
		presets = append(presets, preset)
	}

	// Sort for consistent output
	sortPresets(presets)

	return presets
}

// sortPresets sorts presets in a standard order
func sortPresets(presets []string) {
	order := map[string]int{
		"claude":       1,
		"cursor":       2,
		"gemini":       3,
		"windsurf":     4,
		"copilot":      5,
		"continue-dev": 6,
		"cline":        7,
		"amp":          8,
		"codex":        9,
		"junie":        10,
	}

	// Sort by predefined order, then alphabetically
	sort.SliceStable(presets, func(i, j int) bool {
		orderI, okI := order[presets[i]]
		orderJ, okJ := order[presets[j]]

		if okI && okJ {
			return orderI < orderJ
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return presets[i] < presets[j]
	})
}

// generateV3Config generates the .ai-rulez/config.yaml file
func (m *V2ToV3Migrator) generateV3Config(v2Config *config.Config, presets []string, aiRulezDir string) error {
	// Set gitignore (default to true if not specified in V2)
	gitignore := true
	if v2Config.Gitignore != nil {
		gitignore = *v2Config.Gitignore
	}

	// Create a simple map structure that will marshal correctly
	// We'll marshal presets as strings directly since PresetV3 doesn't have MarshalYAML
	configMap := map[string]interface{}{
		"$schema":     "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-v3.schema.json",
		"version":     "3.0",
		"name":        v2Config.Metadata.Name,
		"description": v2Config.Metadata.Description,
		"presets":     presets,
		"gitignore":   gitignore,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(configMap)
	if err != nil {
		return oops.
			With("config", configMap).
			Wrapf(err, "marshal V3 config")
	}

	// Write to file
	configPath := filepath.Join(aiRulezDir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return oops.
			With("path", configPath).
			Wrapf(err, "write V3 config")
	}

	m.logger.Info("Generated V3 config", "path", configPath, "presets", presets)
	return nil
}

// validateMigration validates the migration by loading the V3 config and attempting to generate
func (m *V2ToV3Migrator) validateMigration(ctx context.Context, aiRulezDir string) error {
	m.logger.Debug("Validating migration", "dir", aiRulezDir)

	// Load the V3 config
	baseDir := filepath.Dir(aiRulezDir)
	v3Config, err := config.LoadConfigV3(ctx, baseDir)
	if err != nil {
		return oops.
			With("base_dir", baseDir).
			Hint("Check the generated .ai-rulez/config.yaml file for errors").
			Wrapf(err, "load V3 config")
	}

	m.logger.Debug("Loaded V3 config for validation",
		"name", v3Config.Name,
		"presets", len(v3Config.Presets),
		"rules", len(v3Config.Content.Rules),
		"context", len(v3Config.Content.Context),
		"skills", len(v3Config.Content.Skills))

	// Validate the V3 config
	if err := v3Config.ValidateV3(); err != nil {
		return oops.
			With("config_name", v3Config.Name).
			Hint("Review the generated V3 configuration and content files").
			Wrapf(err, "validate V3 config")
	}

	// Try to create a generator to ensure structure is valid
	gen := generator.NewGeneratorV3(v3Config)
	if gen == nil {
		return oops.
			Errorf("failed to create generator for validation")
	}

	m.logger.Info("Migration validation successful")
	return nil
}

// MigrationPlan describes what will be migrated
type MigrationPlan struct {
	ProjectName string
	SourcePath  string
	OutputDir   string
	Rules       []MigrationItem
	Context     []MigrationItem
	Skills      []MigrationItem
	Presets     []string
}

// MigrationItem represents a single item to be migrated
type MigrationItem struct {
	Name     string
	Filename string
	ID       string
}

// GetPlan returns a migration plan showing what will be migrated
func (m *V2ToV3Migrator) GetPlan() (*MigrationPlan, error) {
	ctx := context.Background()

	// Load V2 config
	v2Config, err := m.loadV2Config(ctx)
	if err != nil {
		return nil, oops.
			With("v2_config_path", m.v2ConfigPath).
			Wrapf(err, "load V2 config for plan")
	}

	plan := &MigrationPlan{
		ProjectName: v2Config.Metadata.Name,
		SourcePath:  m.v2ConfigPath,
		OutputDir:   m.outputDir,
		Rules:       []MigrationItem{},
		Context:     []MigrationItem{},
		Skills:      []MigrationItem{},
		Presets:     []string{},
	}

	// Collect rules
	for _, rule := range v2Config.Rules {
		filename := utils.SanitizeName(rule.Name)
		plan.Rules = append(plan.Rules, MigrationItem{
			Name:     rule.Name,
			Filename: filename,
		})
	}

	// Collect sections (as context)
	for _, section := range v2Config.Sections {
		baseName := section.ID
		if baseName == "" {
			baseName = section.Name
		}
		filename := utils.SanitizeName(baseName)
		plan.Context = append(plan.Context, MigrationItem{
			Name:     section.Name,
			Filename: filename,
		})
	}

	// Collect agents (as skills)
	for i := range v2Config.Agents {
		agentID := v2Config.Agents[i].ID
		if agentID == "" {
			agentID = utils.SanitizeName(v2Config.Agents[i].Name)
		}
		plan.Skills = append(plan.Skills, MigrationItem{
			Name: v2Config.Agents[i].Name,
			ID:   agentID,
		})
	}

	// Get detected presets
	plan.Presets = m.detectPresets(v2Config)

	return plan, nil
}
