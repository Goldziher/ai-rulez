package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
)

const xumPresetName = "xum"

func init() {
	config.RegisterPreset(xumPresetName, &XumPresetGenerator{})
}

// XumPresetGenerator renders the Xum coding agent's project files:
// a shared AGENTS.md, project skills under .xum/skills, agent definitions under
// .xum/agents, and the stdio MCP servers in .xum/mcp.jsonc.
type XumPresetGenerator struct{}

func (g *XumPresetGenerator) GetName() string {
	return xumPresetName
}

// LocalRootFile implements config.LocalRootProvider: AGENTS.md → AGENTS.local.md.
func (g *XumPresetGenerator) LocalRootFile() string {
	return config.LocalVariantPath("AGENTS.md")
}

func (g *XumPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, "AGENTS.md"),
		filepath.Join(baseDir, ".xum"),
		filepath.Join(baseDir, ".xum", "skills"),
		filepath.Join(baseDir, ".xum", "agents"),
		filepath.Join(baseDir, filepath.FromSlash(MergedDocXumMCP)),
	}
}

func (g *XumPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	outputs = append(outputs,
		config.OutputFile{Path: filepath.Join(baseDir, ".xum"), IsDir: true},
		config.OutputFile{Path: filepath.Join(baseDir, ".xum", "skills"), IsDir: true},
		config.OutputFile{Path: filepath.Join(baseDir, ".xum", "agents"), IsDir: true},
		config.OutputFile{
			Path:    filepath.Join(baseDir, "AGENTS.md"),
			Content: g.renderAgentsMarkdown(content, cfg),
		},
	)

	for _, skill := range allSkills(content) {
		skillID := extractSkillID(skill.Path)
		skillDir := filepath.Join(baseDir, ".xum", "skills", skillID)
		outputs = append(outputs,
			config.OutputFile{Path: skillDir, IsDir: true},
			config.OutputFile{
				Path:    filepath.Join(skillDir, "SKILL.md"),
				Content: g.renderSkillFile(skill),
			},
		)
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	for _, agent := range allAgents(content) {
		agentContent, err := g.renderAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".xum", "agents", sanitizeAgentID(agent.Name)+".md"),
			Content: agentContent,
		})
	}

	mcpOutput, err := g.renderMCPConfig(baseDir, cfg)
	if err != nil {
		return nil, err
	}
	if mcpOutput != nil {
		outputs = append(outputs, *mcpOutput)
	}

	return outputs, nil
}

func (g *XumPresetGenerator) renderAgentsMarkdown(content *config.ContentTree, cfg *config.Config) string {
	var builder strings.Builder

	allRules := allInlineRules(content)
	allAgents := allAgents(content)

	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    cfg.HeaderTimestamp(),
		ConfigFile:   configFileName(cfg),
		OutputFile:   "AGENTS.md",
		Config:       cfg,
		RuleCount:    len(allRules),
		AgentCount:   len(allAgents),
		SectionCount: 0,
	}
	builder.WriteString(templates.GenerateHeader(data))

	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact()}, nil)

	rulefiles.WriteInlineContext(&builder, allInlineContext(content), rulefiles.InlineOpts{Compact: cfg.IsCompact()}, nil)

	renderAgentsSection(&builder, content, allAgents)

	return builder.String()
}

func (g *XumPresetGenerator) renderSkillFile(skill config.ContentFile) string {
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")
	builder.WriteString("description: ")
	builder.WriteString(quoteYAMLString(config.SkillDescriptionForContent(skill)))
	builder.WriteString("\n---\n\n")
	builder.WriteString(skill.Content)
	builder.WriteString(RenderSkillResourcesIndex(&skill))
	return builder.String()
}

func (g *XumPresetGenerator) renderAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	frontmatter := g.buildAgentFrontmatter(agent, cfg)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal agent frontmatter: %w", err)
	}

	var builder strings.Builder
	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")
	builder.WriteString(agent.Content)
	return builder.String(), nil
}

// buildAgentFrontmatter maps ai-rulez agent metadata onto Xum's agent schema.
// Xum nests model/thinking under `ai` and tool policy under `tools`; ai-rulez's
// flat effort tiers are translated to Xum's thinkingLevel vocabulary.
func (g *XumPresetGenerator) buildAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Xum only spawns an agent through the task tool when subagent.runnable is
	// true; without it the definition is selectable as a primary mode only.
	// A source `subagent_runnable` of "false" opts back out.
	if agent.Metadata == nil || agent.Metadata.Extra["subagent_runnable"] != "false" {
		frontmatter["subagent"] = map[string]interface{}{"runnable": true}
	}

	ai := map[string]interface{}{}
	if model := ResolveAgentModel(xumPresetName, agent, cfg); model != "" {
		ai["model"] = model
	}
	if level := xumThinkingLevel(ResolveAgentEffort(xumPresetName, agent, cfg)); level != "" {
		ai["thinkingLevel"] = level
	}
	if len(ai) > 0 {
		frontmatter["ai"] = ai
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	if desc, ok := agent.Metadata.Extra[keyDescription]; ok && desc != "" && EmitAgentField(cfg, "description") {
		frontmatter[keyDescription] = desc
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = map[string]interface{}{"add": agent.Metadata.Tools}
	}

	return frontmatter
}

// xumThinkingLevel maps ai-rulez effort tiers onto Xum's thinkingLevel values
// ("off" | "low" | "medium" | "high").
func xumThinkingLevel(tier string) string {
	switch tier {
	case "", "inherit":
		return ""
	case "xhigh", "max":
		return "high"
	default:
		return tier
	}
}

// renderMCPConfig writes Xum's repo-level MCP override (.xum/mcp.jsonc) with the
// stdio servers as command strings. Xum's format only expresses stdio servers,
// so remote (http/sse) entries are skipped with a warning. Returns nil when no
// stdio servers are configured.
func (g *XumPresetGenerator) renderMCPConfig(baseDir string, cfg *config.Config) (*config.OutputFile, error) {
	servers := map[string]string{}
	for name, server := range cfg.MCPServers {
		if server.GetTransport() != config.TransportStdio {
			logger.Warn("Xum preset supports stdio MCP servers only; skipping remote server",
				"server", name, "transport", server.GetTransport())
			continue
		}
		if server.Command == "" {
			continue
		}
		servers[name] = joinShellCommand(server.Command, server.Args)
	}
	if len(servers) == 0 {
		return nil, nil
	}

	path := filepath.Join(baseDir, filepath.FromSlash(MergedDocXumMCP))
	result, err := applyMergedDocument(path, []jsonmerge.OwnedKey{{Name: "servers", Value: servers}})
	if err != nil {
		return nil, fmt.Errorf("render .xum/mcp.jsonc: %w", err)
	}
	return &config.OutputFile{
		Path:           path,
		Content:        result.Body,
		PartiallyOwned: result.PartiallyOwned,
	}, nil
}

// joinShellCommand renders a stdio MCP command and its args as the single shell
// command string Xum's mcp.jsonc expects, quoting tokens that need it.
func joinShellCommand(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(command))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(token string) string {
	if token == "" {
		return "''"
	}
	safe := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune("-._/:=@+,", r)
	}
	if strings.IndexFunc(token, func(r rune) bool { return !safe(r) }) == -1 {
		return token
	}
	return "'" + strings.ReplaceAll(token, "'", `'\''`) + "'"
}
