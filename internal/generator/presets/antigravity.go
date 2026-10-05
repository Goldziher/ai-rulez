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
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPreset(presetNameAntigravity, &AntigravityPresetGenerator{})
}

// AntigravityPresetGenerator generates Antigravity preset files
type AntigravityPresetGenerator struct{}

// antigravityRulesTarget is Antigravity's workspace rules folder. Antigravity
// reads only the top level of the folder and documents a 24,576 byte limit per
// file; it is applied here as a soft limit on the rendered runes, which
// under-counts for non-ASCII text.
var antigravityRulesTarget = rulefiles.Target{
	Preset:    presetNameAntigravity,
	Dir:       ".agents/rules",
	RootFile:  geminiRootFile,
	Ext:       ".md",
	Dialect:   rulefiles.DialectTrigger,
	Recursive: false,
	MaxChars:  antigravityRuleMaxChars,
	Banner:    true,
}

const antigravityRuleMaxChars = 24576

// antigravityRouting decides which rules become files. GEMINI.md is written by
// both the antigravity and gemini presets and the last writer wins, so when
// both are enabled the root file must stay self-contained: everything inline,
// unless the user set rules.mode_by_preset.antigravity explicitly.
//
// demoted is the routing that would have applied without the gemini preset
// (RoutingNone when nothing was demoted).
func antigravityRouting(cfg *config.Config, warn func(msg string, args ...any)) (routing, demoted rulefiles.Routing) {
	if cfg == nil {
		return rulefiles.RoutingNone, rulefiles.RoutingNone
	}
	want := rulefiles.RoutingFor(cfg.RulesModeFor(presetNameAntigravity), true)
	if cfg.AgentsMD {
		// Neither antigravity nor gemini writes GEMINI.md: the always-on content
		// is in the shared AGENTS.md, so the rules folder keeps the rest and
		// nothing is loaded twice or demoted.
		return rulefiles.WithoutAlwaysOn(want), rulefiles.RoutingNone
	}
	if !geminiPresetEnabled(cfg) {
		return want, rulefiles.RoutingNone
	}
	if cfg.RulesModeExplicitFor(presetNameAntigravity) {
		warn("antigravity and gemini presets both write GEMINI.md; rules moved to .agents/rules "+
			"will be loaded twice (the gemini preset still inlines them in GEMINI.md)",
			"mode", cfg.RulesModeFor(presetNameAntigravity))
		return want, rulefiles.RoutingNone
	}
	return rulefiles.RoutingNone, want
}

func geminiPresetEnabled(cfg *config.Config) bool {
	for i := range cfg.Presets {
		if cfg.Presets[i].GetName() == presetNameGemini {
			return true
		}
	}
	return false
}

func generateAntigravityPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *AntigravityPresetGenerator) GetName() string {
	return presetNameAntigravity
}

// antigravityWorkflows is the folder of Antigravity workflows (custom slash
// commands), shared by the IDE and the CLI.
var antigravityWorkflows = commandFilesSpec{preset: presetNameAntigravity, dir: ".agents/workflows", ext: ".md"}

// GlobalOutputPaths is the Antigravity user-scope layout: the shared
// ~/.gemini/config tree (rules, skills, agents, MCP), ~/.gemini/GEMINI.md and the
// IDE's global workflows.
func (g *AntigravityPresetGenerator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	return GlobalLayout{
		RootFile:    ".gemini/GEMINI.md",
		RulesDir:    ".gemini/config/rules",
		SkillsDir:   ".gemini/config/skills",
		AgentsDir:   ".gemini/config/agents",
		CommandsDir: ".gemini/antigravity/global_workflows",
		Sidecars:    map[string]string{MergedDocAgentsMCP: ".gemini/config/mcp_config.json"},
	}.Resolve(home, getenv)
}

func (g *AntigravityPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".agents", "workflows"),
		filepath.Join(baseDir, "GEMINI.md"),
		filepath.Join(baseDir, ".agents"),
		filepath.Join(baseDir, ".agents", "rules"),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, ".agents", "agents"),
	}
}

func (g *AntigravityPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .agents directory structure
	outputs = append(outputs,
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

	// Generate .agents/settings.json with MCP configuration.
	//
	// ai-rulez owns the mcpServers key of this document and the consumer owns the
	// rest, so merge into whatever is on disk rather than replacing it (#185).
	//
	// Emitted only when there are MCP servers to contribute, for the same reason as
	// the gemini preset: an owned key is replaced wholesale, so an unconditional
	// write would still reduce a consumer's own mcpServers to the lone ai-rulez
	// self-registration entry. Losing that self-registration in projects with no
	// [[mcp_servers]] is the better trade.
	if len(cfg.MCPServers) > 0 {
		settingsPath := filepath.Join(baseDir, filepath.FromSlash(MergedDocAgentsSettings))
		settings, err := g.renderSettingsJSON(settingsPath, cfg)
		if err != nil {
			return nil, fmt.Errorf("render settings.json: %w", err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:           settingsPath,
			Content:        settings.Body,
			PartiallyOwned: settings.PartiallyOwned,
			MergeClaims:    settings.Claims,
		})
	}

	// Workspace MCP servers live in .agents/mcp_config.json (the file Antigravity
	// reads); settings.json above is kept for output written by earlier versions.
	if len(cfg.MCPServers) > 0 {
		mcpPath := filepath.Join(baseDir, filepath.FromSlash(MergedDocAgentsMCP))
		mcpConfig, err := g.renderMCPConfigJSON(mcpPath, cfg)
		if err != nil {
			return nil, fmt.Errorf("render mcp_config.json: %w", err)
		}
		outputs = append(outputs, mergedOutput(mcpPath, mcpConfig))
	}

	// Commands are workflows: .agents/workflows/{id}.md, invoked as /{id}.
	workflows, err := commandFileOutputs(content, baseDir, antigravityWorkflows)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, workflows...)

	routing, demoted := antigravityRouting(cfg, logger.Warn)
	rules, contexts := allInlineRules(content), allInlineContext(content)
	files, inlineRules, inlineContext, err := rulefiles.Plan(rules, contexts,
		&antigravityRulesTarget, routing, rulefiles.ScopeOf(cfg), rulefiles.RegistryFor(cfg, presetNameAntigravity))
	if err != nil {
		return nil, oops.With("preset", presetNameAntigravity).Wrapf(err, "plan antigravity rule files")
	}
	if demoted != rulefiles.RoutingNone {
		// Informational only when the demotion actually costs rule files.
		wouldBe, _, _, planErr := rulefiles.Plan(rules, contexts, &antigravityRulesTarget, demoted, rulefiles.ScopeInfo{}, nil)
		msg := "antigravity rule files disabled: the gemini preset also writes GEMINI.md, " +
			"so all rules stay inline; set rules.mode_by_preset.antigravity to override"
		if planErr == nil && len(wouldBe) > 0 {
			logger.Info(msg)
		} else {
			logger.Debug(msg)
		}
	}
	if len(files) > 0 && !rulefiles.InScope(cfg) {
		outputs = append(outputs, config.OutputFile{
			Path:  filepath.Join(baseDir, filepath.FromSlash(antigravityRulesTarget.Dir)),
			IsDir: true,
		})
	}
	for i := range files {
		it := &files[i]
		text, notes, err := rulefiles.Render(antigravityRulesTarget, *it, cfg)
		if err != nil {
			return nil, oops.With("preset", presetNameAntigravity, "rule", it.File.Name).
				Wrapf(err, "render antigravity rule file")
		}
		path := rulefiles.RulesDirPath(cfg, baseDir, antigravityRulesTarget, rulefiles.FileName(antigravityRulesTarget, *it))
		rulefiles.ReportNotes(path, notes)
		outputs = append(outputs, config.OutputFile{Path: path, Content: text})
	}

	// Generate GEMINI.md with the inline remainder and context. With agents_md the
	// shared AGENTS.md replaces it, so it is not rendered.
	if !cfg.AgentsMD {
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, "GEMINI.md"),
			Content: g.renderMarkdown(inlineRules, inlineContext, content, cfg),
		})
	}

	// Generate skill files to .agents/skills/
	allSkills := allSkills(content)
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".agents", "skills", skillID)
		outputs = append(outputs, config.OutputFile{
			Path:  skillDir,
			IsDir: true,
		})

		skillContent := g.renderSkillFile(skill)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate agent files to .agents/agents/
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".agents", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSettingsJSON renders the mcpServers key ai-rulez owns into the settings
// document at settingsPath, preserving every other top-level key that is already
// there. An empty settingsPath renders a fresh document, which is what the unit
// tests exercise.
func (g *AntigravityPresetGenerator) renderSettingsJSON(
	settingsPath string,
	cfg *config.Config,
) (jsonmerge.Result, error) {
	return applyMergedDocument(settingsPath, []jsonmerge.OwnedKey{
		{Name: keyMCPServers, Value: antigravityMCPServers(cfg), Members: true},
	})
}

// renderMCPConfigJSON renders the same servers into .agents/mcp_config.json, the
// file Antigravity actually reads them from (workspace scope); settings.json is
// kept for compatibility with earlier output.
func (g *AntigravityPresetGenerator) renderMCPConfigJSON(path string, cfg *config.Config) (jsonmerge.Result, error) {
	return applyMergedDocument(path, []jsonmerge.OwnedKey{
		{Name: keyMCPServers, Value: antigravityMCPServers(cfg), Members: true},
	})
}

// antigravityMCPServers is the mcpServers member Antigravity reads.
func antigravityMCPServers(cfg *config.Config) map[string]interface{} {
	mcpServers := make(map[string]interface{})

	// Always include the hardcoded ai-rulez MCP server
	mcpServers["ai-rulez"] = map[string]interface{}{
		keyCommand: cmdNPX,
		keyArgs: []string{
			"-y",
			aiRulezLatest,
			keyMCP,
		},
	}

	// Merge user-configured MCP servers
	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{}

		// Antigravity (Devin/Codeium lineage) has no transport/type key and keys
		// remote servers on `serverUrl`. A stdio entry with an empty command is
		// invalid, so omit command/args for remote (http/sse) transports.
		switch server.GetTransport() {
		case config.TransportHTTP, config.TransportSSE:
			if server.URL != "" {
				entry["serverUrl"] = server.URL
			}
			if len(server.Headers) > 0 {
				entry[keyHeaders] = server.Headers
			}
		default:
			entry[keyCommand] = server.Command
			if len(server.Args) > 0 {
				entry["args"] = server.Args
			}
		}

		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
		if !server.IsEnabled() {
			entry[keyDisabled] = true
		}

		mcpServers[name] = entry
	}

	return mcpServers
}

func (g *AntigravityPresetGenerator) renderMarkdown(
	allRules, allContext []config.ContentFile,
	content *config.ContentTree,
	cfg *config.Config,
) string {
	var builder strings.Builder

	allAgents := allAgents(content)

	header := generateAntigravityPresetHeader(cfg, "GEMINI.md", len(allRules), 0, len(allAgents))
	builder.WriteString(header)

	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	renderAgentsSection(&builder, content, allAgents)

	return builder.String()
}

func (g *AntigravityPresetGenerator) renderSkillFile(skill config.ContentFile) string {
	return renderAgentSkillFile(skill)
}

func (g *AntigravityPresetGenerator) renderAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildAgentFrontmatter(agent, cfg)

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

func (g *AntigravityPresetGenerator) buildAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	// .agents/agents is shared with the amp spec, which writes the same keys:
	// values keep their YAML type and the invocation switches are booleans.
	agentFields := []string{
		keyDescription, keyKind, keyModel, keyTemperature, "max_turns", "timeout_mins",
		"user-invocable", "disable-model-invocation",
	}
	for _, field := range agentFields {
		if val, ok := typedAgentField(agent.Metadata, field); ok {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
