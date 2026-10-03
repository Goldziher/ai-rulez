package presets

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
)

const opencodePresetName = "opencode"

// opencodeSchemaURL points an author's editor at OpenCode's config schema.
// ai-rulez owns this key alongside mcp.servers: emitting it means a freshly
// generated opencode.json is entirely ai-rulez's (so it is gitignored and
// manifest-tracked), while a hand-authored file that adds further keys — model,
// mcp.timeout — still counts as the consumer's and is preserved (#185).
const opencodeSchemaURL = "https://opencode.ai/config.json"

func init() {
	config.RegisterPreset(opencodePresetName, &OpencodePresetGenerator{})
}

// OpencodePresetGenerator generates Opencode preset files (AGENTS.md)
type OpencodePresetGenerator struct{}

// generateOpenCodePresetHeader creates a header for Opencode preset files
func generateOpenCodePresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *OpencodePresetGenerator) GetName() string {
	return opencodePresetName
}

// LocalRootFile implements config.LocalRootProvider: AGENTS.md → AGENTS.local.md.
func (g *OpencodePresetGenerator) LocalRootFile() string {
	return config.LocalVariantPath("AGENTS.md")
}

func (g *OpencodePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "AGENTS.md"),
		filepath.Join(baseDir, ".opencode"),
		filepath.Join(baseDir, ".opencode", "skills"),
		filepath.Join(baseDir, ".opencode", "agents"),
	}
}

func (g *OpencodePresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .opencode directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".opencode"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".opencode", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".opencode", "agents"),
			IsDir: true,
		},
	)

	// Generate AGENTS.md file
	agentsContent := g.renderAgentsMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, "AGENTS.md"),
		Content: agentsContent,
		IsDir:   false,
	})

	// Generate skill files to .opencode/skills/
	allSkills := allSkills(content)
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".opencode", "skills", skillID)
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
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate agent files to .opencode/agents/
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderOpencodeAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".opencode", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	// Generate opencode.json in OpenCode's native v2 shape. ai-rulez owns
	// $schema and mcp.servers, so a fresh document is entirely ours; any other
	// key a hand-authored opencode.json carries is preserved (#185, #194).
	if len(cfg.MCPServers) > 0 {
		mcpPath := filepath.Join(baseDir, MergedDocOpencodeConfig)
		mcpFile, err := g.renderMCPConfig(mcpPath, cfg)
		if err != nil {
			return nil, fmt.Errorf("render opencode.json: %w", err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:           mcpPath,
			Content:        mcpFile.Body,
			PartiallyOwned: mcpFile.PartiallyOwned,
		})
	}

	return outputs, nil
}

// renderMCPConfig renders OpenCode's native v2 MCP servers into opencode.json.
// It owns the top-level $schema and the nested mcp.servers key, so every sibling
// key under mcp (such as mcp.timeout) and every other top-level key survive the
// merge.
func (g *OpencodePresetGenerator) renderMCPConfig(mcpPath string, cfg *config.Config) (jsonmerge.Result, error) {
	servers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			keyDisabled: !server.IsEnabled(),
		}

		// V2 has exactly two types: local (stdio) and remote (Streamable HTTP).
		switch server.GetTransport() {
		case config.TransportHTTP, config.TransportSSE:
			entry["type"] = "remote"
			if server.URL != "" {
				entry["url"] = server.URL
			}
			if len(server.Headers) > 0 {
				entry[keyHeaders] = server.Headers
			}
		default:
			entry["type"] = "local"
			if server.Command != "" {
				// V2 takes the executable and its arguments as one array.
				entry[keyCommand] = append([]string{server.Command}, server.Args...)
			}
			if len(server.Env) > 0 {
				entry["environment"] = server.Env
			}
		}

		servers[name] = entry
	}

	return applyMergedDocument(mcpPath, []jsonmerge.OwnedKey{
		{Path: []string{"$schema"}, Value: opencodeSchemaURL},
		{Path: []string{"mcp", "servers"}, Value: servers},
	})
}

func (g *OpencodePresetGenerator) renderAgentsMarkdown(content *config.ContentTree, cfg *config.Config) string {
	var builder strings.Builder

	// Calculate content counts
	allRules := rootRules(content, opencodePresetName, "AGENTS.md")
	allAgents := allAgents(content)

	// Add header before title
	header := generateOpenCodePresetHeader(cfg, "AGENTS.md", len(allRules), 0, len(allAgents))
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
	allContext := rootContext(content, opencodePresetName, "AGENTS.md")
	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Add agents section listing available subagents (if agent-delegation builtin is enabled)
	renderAgentsSection(&builder, content, allAgents)

	// Skills are generated to .opencode/skills/ directory, not inlined

	return builder.String()
}

// renderSkillFile renders a skill file in SKILL.md format for OpenCode
func (g *OpencodePresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

// renderOpencodeAgentFile renders an agent file with YAML frontmatter for OpenCode
func (g *OpencodePresetGenerator) renderOpencodeAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildOpencodeAgentFrontmatter(agent, cfg)

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

// resolveOpencodeModel returns the agent's model when OpenCode can resolve it,
// otherwise "". OpenCode only understands "provider/model"; a bare alias like
// "sonnet" (valid for Claude) makes it drop the whole agent file without an
// error, or fail at session time with "Model not found: sonnet/.". The agent
// is emitted without a model instead, so it inherits the session's model.
func resolveOpencodeModel(agent config.ContentFile, cfg *config.Config) string {
	model := ResolveAgentModel(opencodePresetName, agent, cfg)
	if model == "" || IsProviderQualifiedModel(model) {
		return model
	}
	logger.Warn("OpenCode needs a provider-qualified model (provider/model); omitting it so the agent inherits the session model",
		"agent", agent.Name, "model", model,
		"hint", "set opencode_model in the agent frontmatter or defaults.model_by_preset.opencode")
	return ""
}

// buildOpencodeAgentFrontmatter builds native v2 frontmatter for an OpenCode
// agent file. The agent's identity comes from its filename, so no `name` key is
// emitted; effort is expressed as a separate model variant key.
func (g *OpencodePresetGenerator) buildOpencodeAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{}

	if agent.Metadata != nil {
		if description := agent.Metadata.Extra[keyDescription]; EmitAgentField(cfg, "description") && description != "" {
			frontmatter[keyDescription] = description
		}
	}

	// Resolve effort before the metadata-nil short-circuit so a defaults-only
	// effort still applies to agents with no frontmatter. Markdown agents take
	// the model as plain "provider/model" (the "#variant" form exists only in
	// opencode.json), so the variant is always a separate key. A variant named
	// in the source model wins over the generic effort.
	variant := MapEffort(opencodePresetName, ResolveAgentEffort(opencodePresetName, agent, cfg))
	if model := resolveOpencodeModel(agent, cfg); model != "" {
		model, sourceVariant, _ := strings.Cut(model, "#")
		frontmatter[keyModel] = model
		if sourceVariant != "" {
			variant = sourceVariant
		}
	}
	if variant != "" {
		frontmatter["variant"] = variant
	}

	// Default to `all` so a generated agent is spawnable as a subagent as well
	// as selectable as a primary; OpenCode defaults an agent with no `mode` to
	// primary-only. A source `mode` still wins. Set before the metadata
	// short-circuit so a bare agent is still spawnable.
	mode := "all"
	if agent.Metadata != nil {
		if sourceMode := agent.Metadata.Extra["mode"]; sourceMode != "" {
			mode = sourceMode
		}
	}
	frontmatter["mode"] = mode

	if agent.Metadata == nil {
		return frontmatter
	}

	if hidden, ok := opencodeHidden(agent); ok {
		frontmatter["hidden"] = hidden
	}

	// temperature and top_p are top-level numeric keys; both OpenCode loaders
	// accept them and v2 migrates them into the request body itself.
	for field, num := range opencodeSampling(agent) {
		frontmatter[field] = num
	}

	return frontmatter
}

// opencodeHidden parses the agent's hidden flag. OpenCode types it as a
// boolean: a quoted string makes it drop the agent.
func opencodeHidden(agent config.ContentFile) (value, ok bool) {
	raw := agent.Metadata.Extra["hidden"]
	if raw == "" {
		return false, false
	}
	hidden, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("OpenCode agent field hidden must be true or false; omitting it", "agent", agent.Name, "value", raw)
		return false, false
	}
	return hidden, true
}

// opencodeSampling collects the sampling parameters as numbers; a quoted
// string would reach the provider as one.
func opencodeSampling(agent config.ContentFile) map[string]float64 {
	values := map[string]float64{}
	for _, field := range []string{keyTemperature, "top_p"} {
		raw := agent.Metadata.Extra[field]
		if raw == "" {
			continue
		}
		num, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			logger.Warn("OpenCode agent field "+field+" must be a number; omitting it", "agent", agent.Name, "value", raw)
			continue
		}
		values[field] = num
	}
	return values
}
