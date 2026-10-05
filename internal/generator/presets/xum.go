package presets

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
	"gopkg.in/yaml.v3"
)

const xumPresetName = "xum"

func init() {
	config.RegisterPreset(xumPresetName, &XumPresetGenerator{})
}

// XumPresetGenerator renders the Xum coding agent's project files:
// a shared AGENTS.md, project skills under .xum/skills, agent definitions under
// .xum/agents, and the stdio, http and sse MCP servers in .xum/mcp.jsonc.
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

	allRules := rootRules(content, cfg, xumPresetName, "AGENTS.md")
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

	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	rulefiles.WriteInlineContext(&builder, rootContext(content, cfg, xumPresetName, "AGENTS.md"), rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

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
	builder.WriteString("\n")
	writeSkillSpecFields(&builder, skill, nil)
	builder.WriteString("---\n\n")
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

// renderMCPConfig writes Xum's repo-level MCP override (.xum/mcp.jsonc). A stdio
// server without env or disabled state keeps the legacy shell-command string;
// otherwise, and for http/sse servers, the entry is Xum's object form so env,
// headers and the disabled flag are preserved. Returns nil when no usable
// servers are configured.
func (g *XumPresetGenerator) renderMCPConfig(baseDir string, cfg *config.Config) (*config.OutputFile, error) {
	servers := xumServers(cfg)
	if len(servers) == 0 {
		return nil, nil
	}

	path := filepath.Join(baseDir, filepath.FromSlash(MergedDocXumMCP))
	result, err := applyMergedDocument(path, []jsonmerge.OwnedKey{{Name: keyServers, Value: servers, Members: true}})
	if err != nil {
		return nil, fmt.Errorf("render .xum/mcp.jsonc: %w", err)
	}
	return &config.OutputFile{
		Path:           path,
		Content:        result.Body,
		PartiallyOwned: result.PartiallyOwned,
		MergeClaims:    result.Claims,
	}, nil
}

// xumServers maps the configured servers onto Xum's mcp.jsonc entries.
func xumServers(cfg *config.Config) map[string]interface{} {
	servers := map[string]interface{}{}
	for name, server := range cfg.MCPServers {
		if entry := xumMCPEntry(server); entry != nil {
			servers[name] = entry
		}
	}
	return servers
}

// xumMCPEntry maps one server onto Xum's mcp.jsonc schema. Xum needs a full
// entry where the shared stdio/url shape is enough: a stdio server is a shell
// command string (or an object when disabled), and a remote one carries the
// `transport` key the other tools do not.
func xumMCPEntry(server *config.MCPServer) interface{} {
	if server == nil {
		return nil
	}
	switch transport := server.GetTransport(); transport {
	case config.TransportHTTP, config.TransportSSE:
		if server.URL == "" {
			return nil
		}
		entry := map[string]interface{}{"transport": transport, "url": server.URL}
		if len(server.Headers) > 0 {
			entry[keyHeaders] = server.Headers
		}
		if !server.IsEnabled() {
			entry[keyDisabled] = true
		}
		return entry
	default:
		if server.Command == "" {
			return nil
		}
		// Xum reads only the command string of a stdio entry in mcp.jsonc (args
		// and env are honored for plugin servers only) and runs it through a
		// POSIX shell, so args are joined into the command and env becomes a
		// leading KEY=value assignment list.
		command := joinShellCommand(server.Command, server.Args)
		if prefix := xumEnvPrefix(server); prefix != "" {
			command = prefix + " " + command
		}
		if server.IsEnabled() {
			return command
		}
		return map[string]interface{}{"transport": config.TransportStdio, "command": command, keyDisabled: true}
	}
}

// xumEnvPrefix renders a stdio server's env as shell assignments ("A=1 B='x y'")
// in key order. Names that are not shell identifiers cannot be assigned this way
// and are skipped with a warning.
func xumEnvPrefix(server *config.MCPServer) string {
	keys := make([]string, 0, len(server.Env))
	for key := range server.Env {
		if !isShellIdentifier(key) {
			logger.Warn("Skipping an MCP env variable Xum's mcp.jsonc cannot express; "+
				"set it in the environment Xum runs in", "server", server.Name, "variable", key)
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+shellQuote(server.Env[key]))
	}
	return strings.Join(parts, " ")
}

func isShellIdentifier(name string) bool {
	for i, r := range name {
		letter := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return name != ""
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
