package presets

import (
	"fmt"
	"github.com/Goldziher/ai-rulez/v5/schema"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
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

// ProjectLayout is where the preset writes project-level files; user scope maps them
// onto GlobalOutputPaths.
func (g *AntigravityPresetGenerator) ProjectLayout() ProjectLayout {
	return ProjectLayout{
		RootFile: "GEMINI.md", RulesDir: ".agents/rules", SkillsDir: ".agents/skills", AgentsDir: ".agents/agents",
		CommandsDir: ".agents/skills",
	}
}

// GlobalOutputPaths is the Antigravity user-scope layout: the shared
// ~/.gemini/config tree (rules, skills, agents, MCP), ~/.gemini/GEMINI.md and the
// IDE's global workflows.
func (g *AntigravityPresetGenerator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	return GlobalLayout{
		RootFile:    ".gemini/GEMINI.md",
		RulesDir:    ".gemini/config/rules",
		SkillsDir:   ".gemini/config/skills",
		AgentsDir:   ".gemini/config/agents",
		CommandsDir: ".gemini/config/skills",
		Sidecars: map[string]string{
			MergedDocAgentsMCP:        ".gemini/config/mcp_config.json",
			MergedDocAntigravityHooks: ".gemini/config/hooks.json",
		},
	}.Resolve(home, getenv)
}

func (g *AntigravityPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
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

	// Workspace MCP servers live in .agents/mcp_config.json, the file Antigravity
	// reads. Earlier versions also wrote .agents/settings.json, which nothing
	// reads; it is no longer written, and renderSettingsJSON only remains so the
	// stale-output cleanup can recognise what those versions wrote.
	if len(cfg.MCPServers) > 0 || cfg.HasSelfServer() {
		mcpPath := filepath.Join(baseDir, filepath.FromSlash(MergedDocAgentsMCP))
		mcpConfig, err := g.renderMCPConfigJSON(mcpPath, cfg)
		if err != nil {
			return nil, fmt.Errorf("render mcp_config.json: %w", err)
		}
		outputs = append(outputs, mergedOutput(mcpPath, mcpConfig))
	}

	hookOutputs, err := g.hooksOutputs(cfg, baseDir)
	if err != nil {
		return nil, fmt.Errorf("render antigravity hooks: %w", err)
	}
	outputs = append(outputs, hookOutputs...)

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

	// Generate skill files to .agents/skills/. Workflows (the custom slash
	// commands) retire on 2026-11-01 in favour of skills, so a command is written
	// as a skill, which Antigravity runs on an explicit invocation.
	allSkills := append(allSkills(content), commandAsSkills(content, presetNameAntigravity)...)
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

	// The ai-rulez MCP server is added only when [mcp] self_server asks for it, as
	// for every other preset (Antigravity has no `type` key, so it is dropped).
	if cfg.HasSelfServer() {
		self := cfg.SelfMCPServerEntry(schema.Version)
		delete(self, keyType)
		if _, declared := cfg.MCPServers[config.SelfMCPServerName]; !declared {
			mcpServers[config.SelfMCPServerName] = self
		}
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

	// Antigravity subagents take name, description, model (inherit, flash or pro),
	// tools, subagent and commandExecutionPolicy; the Gemini CLI keys (kind,
	// temperature, max_turns, timeout_mins) are not part of the format.
	for _, field := range []string{keyDescription, "subagent", "commandExecutionPolicy"} {
		if val, ok := typedAgentField(agent.Metadata, field); ok {
			frontmatter[field] = val
		}
	}
	if model := antigravityModel(ResolveAgentModel(presetNameAntigravity, agent, cfg)); model != "" {
		frontmatter[keyModel] = model
	}
	if EmitAgentField(cfg, "tools") {
		if tools := antigravityTools(agent.Metadata.Tools); len(tools) > 0 {
			frontmatter["tools"] = tools
		}
	}

	return frontmatter
}

// antigravityModelTiers is how a model reads as one of Antigravity's three
// subagent models.
var antigravityModelTiers = []struct{ contains, tier string }{
	{"inherit", "inherit"}, {"flash", "flash"}, {"haiku", "flash"}, {"pro", "pro"}, {"sonnet", "pro"}, {"opus", "pro"},
}

// antigravityModel maps a resolved model onto inherit, flash or pro, the only
// values an Antigravity subagent accepts. A Claude alias maps by tier (haiku to
// flash, sonnet and opus to pro); anything else is dropped so the subagent
// inherits.
func antigravityModel(model string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	for _, t := range antigravityModelTiers {
		if strings.Contains(lower, t.contains) {
			return t.tier
		}
	}
	return ""
}

// antigravityToolNames maps the Claude tools with a documented Antigravity
// counterpart; an unmapped name makes the subagent hang, so the rest are dropped.
var antigravityToolNames = map[string]string{
	"read": "view_file", "edit": "replace_file_content", "multiedit": "replace_file_content",
	"grep": "grep_search", "bash": "run_command",
}

// antigravityTools translates Claude tool names, dropping those with no mapping.
func antigravityTools(tools []string) []string {
	var out []string
	for _, tool := range tools {
		name, ok := antigravityToolNames[strings.ToLower(strings.TrimSpace(tool))]
		if ok && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}
