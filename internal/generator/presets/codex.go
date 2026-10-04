package presets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

const codexPresetName = "codex"

func init() {
	config.RegisterPreset(codexPresetName, &CodexPresetGenerator{})
}

// CodexPresetGenerator generates Codex preset files (AGENTS.md)
type CodexPresetGenerator struct{}

// generateCodexPresetHeader creates a header for Codex preset files
func generateCodexPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
	// Create TemplateData for header generation
	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    cfg.HeaderTimestamp(),
		ConfigFile:   configFileName(cfg),
		OutputFile:   outputPath,
		Config:       cfg,
		RuleCount:    ruleCount,
		SectionCount: sectionCount,
		AgentCount:   agentCount,
	}

	return templates.GenerateHeader(data)
}

func (g *CodexPresetGenerator) GetName() string {
	return codexPresetName
}

func (g *CodexPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "AGENTS.md"),
		filepath.Join(baseDir, ".codex"),
		filepath.Join(baseDir, filepath.FromSlash(config.DefaultCodexSkillsDir)),
		filepath.Join(baseDir, ".codex", "agents"),
		filepath.Join(baseDir, ".codex", "prompts"),
	}
}

func (g *CodexPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	skillsRoot := filepath.Join(baseDir, filepath.FromSlash(cfg.CodexSkillsDirOrDefault()))
	if err := config.ValidateOutputSubdir("codex_skills_dir", cfg.CodexSkillsDirOrDefault()); err != nil {
		return nil, err
	}

	// Create .codex directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".codex"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  skillsRoot,
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".codex", "agents"),
			IsDir: true,
		},
	)

	// Generate AGENTS.md file (rules and context only, no skills)
	agentsContent := g.renderAgentsMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, "AGENTS.md"),
		Content: agentsContent,
		IsDir:   false,
	})

	// Combine all skills from root and domains
	allSkills := allSkills(content)

	// Generate skill files to the skills root (.agents/skills by default)
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		// Create skill directory
		skillDir := filepath.Join(skillsRoot, skillID)
		outputs = append(outputs, config.OutputFile{
			Path:  skillDir,
			IsDir: true,
		})

		// Generate SKILL.md file
		skillContent := g.renderSkillFile(skill)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})

		// Emit bundled resources alongside SKILL.md so the agent can read
		// references on demand rather than receiving them all inlined.
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)

		// Codex has no SKILL.md key for invocation control; an author-set
		// disable-model-invocation becomes agents/openai.yaml policy.
		if disabled, set := skill.Metadata.ExtraBool("disable-model-invocation"); set && disabled {
			agentsDir := filepath.Join(skillDir, "agents")
			outputs = append(outputs,
				config.OutputFile{Path: agentsDir, IsDir: true},
				config.OutputFile{Path: filepath.Join(agentsDir, "openai.yaml"), Content: codexImplicitInvocationOff},
			)
		}
	}

	// Generate agent files to .codex/agents/ (TOML format)
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent := g.renderAgentTOML(agent, cfg)

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".codex", "agents", agentID+".toml"),
			Content: agentContent,
		})
	}

	// Generate .codex/prompts directory. Codex CLI reads custom prompts from
	// .codex/prompts/*.md.
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".codex", "prompts"),
		IsDir: true,
	})

	// Generate command files to .codex/prompts/
	allCommands := allCommands(content)
	for _, command := range allCommands {
		if !g.shouldIncludeCommand(command) {
			continue
		}
		sanitized := sanitizeName(command.Name)
		commandContent := g.renderCommandFile(command)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".codex", "prompts", sanitized+".md"),
			Content: commandContent,
		})
	}

	// Emit .codex/config.toml with the global model_reasoning_effort as a
	// session-level default. Per-agent overrides live in each agent's TOML
	// file (see renderAgentTOML) and take precedence when an agent runs.
	if effort := MapEffort(codexPresetName, ResolveGlobalEffort(codexPresetName, cfg)); effort != "" {
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".codex", "config.toml"),
			Content: fmt.Sprintf("model_reasoning_effort = %q\n", effort),
		})
	}

	// Generate .codex/plugins.json with plugin declarations (if configured)
	if len(cfg.Plugins) > 0 {
		pluginsContent, err := g.renderPluginsJSON(cfg)
		if err != nil {
			return nil, fmt.Errorf("render plugins.json: %w", err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".codex", "plugins.json"),
			Content: pluginsContent,
		})
	}

	return outputs, nil
}

func (g *CodexPresetGenerator) renderAgentsMarkdown(content *config.ContentTree, cfg *config.Config) string {
	return g.renderAgentsMarkdownFor(content, cfg, nil)
}

// sharedAgentsMDOpts shapes the shared AGENTS.md rendered when agents_md is on.
type sharedAgentsMDOpts struct {
	// owners and aliases extend the default AGENTS.md owners and root file names
	// that frontmatter targets are matched against.
	owners, aliases []string
	// inlining keeps only the always-on items plus the scoped ones it asks for.
	inlining config.AgentsMDInlining
	// negatedOnly keeps the items scoped by negated globs only, which no rules
	// folder can express.
	negatedOnly bool
}

// renderAgentsMarkdownFor renders AGENTS.md; a nil shared renders the file the
// codex preset writes on its own.
func (g *CodexPresetGenerator) renderAgentsMarkdownFor(content *config.ContentTree, cfg *config.Config, shared *sharedAgentsMDOpts,
) string {
	var builder strings.Builder

	root := rulefiles.RootTarget(codexPresetName, "AGENTS.md")
	if shared != nil {
		root.Owners, root.RootAliases = shared.owners, shared.aliases
	}
	allRules := withoutBazNested(inlinedInAgentsMD(rulefiles.FilterInline(allInlineRules(content), root), shared, false), cfg)
	allAgents := allAgents(content)

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
	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Add context section
	allContext := withoutBazNested(inlinedInAgentsMD(rulefiles.FilterInline(allInlineContext(content), root), shared, true), cfg)
	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Add agents section listing available subagents (if agent-delegation builtin is enabled)
	renderAgentsSection(&builder, content, allAgents)

	// Skills are generated to .codex/skills/ directory, not inlined in AGENTS.md

	return builder.String()
}

// codexImplicitInvocationOff is the agents/openai.yaml of a skill Codex must
// only run when the user names it ($skill), the Codex counterpart of
// disable-model-invocation.
const codexImplicitInvocationOff = "policy:\n  allow_implicit_invocation: false\n"

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

	// short-description is nested under metadata, merged into the author's own
	// metadata map when there is one.
	extra := map[string]any{}
	if shortDesc := config.SkillShortDescription(skill.Metadata); shortDesc != "" {
		extra["metadata"] = mergeShortDescription(skill.Metadata.SkillSpecFields()["metadata"], shortDesc)
	}
	writeSkillSpecFields(&builder, skill, extra)

	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	// Index bundled resources so the agent knows what to read on demand.
	builder.WriteString(RenderSkillResourcesIndex(&skill))

	return builder.String()
}

// renderAgentTOML renders an agent file in TOML format for Codex
func (g *CodexPresetGenerator) renderAgentTOML(agent config.ContentFile, cfg *config.Config) string {
	var builder strings.Builder

	builder.WriteString("name = ")
	builder.WriteString(quoteTOMLString(agent.Name))
	builder.WriteString("\n")

	description := ""
	if agent.Metadata != nil {
		if desc, ok := agent.Metadata.Extra[keyDescription]; ok {
			description = desc
		}
	}
	builder.WriteString("description = ")
	builder.WriteString(quoteTOMLString(description))
	builder.WriteString("\n")

	if effort := MapEffort(codexPresetName, ResolveAgentEffort(codexPresetName, agent, cfg)); effort != "" {
		builder.WriteString("model_reasoning_effort = ")
		builder.WriteString(quoteTOMLString(effort))
		builder.WriteString("\n")
	}

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
			if target == codexPresetName {
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
		if desc, ok := command.Metadata.Extra[keyDescription]; ok && desc != "" {
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
func (g *CodexPresetGenerator) renderPluginsJSON(cfg *config.Config) (string, error) {
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
