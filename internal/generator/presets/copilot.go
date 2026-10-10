package presets

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/markdown"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

const presetNameCopilot = "copilot"

// copilotRulesTarget is the path-specific instructions folder Copilot reads.
var copilotRulesTarget = rulefiles.Target{
	Preset:    presetNameCopilot,
	Dir:       ".github/instructions",
	RootFile:  copilotInstructionsFile,
	Ext:       ".instructions.md",
	Dialect:   rulefiles.DialectCopilot,
	Recursive: true,
	Banner:    true,
}

// CopilotPresetGenerator generates GitHub Copilot preset files
type CopilotPresetGenerator struct{}

// generateCopilotPresetHeader creates a header for Copilot preset files
func generateCopilotPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *CopilotPresetGenerator) GetName() string {
	return presetNameCopilot
}

func (g *CopilotPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".github"),
		filepath.Join(baseDir, ".github", "copilot-instructions.md"),
		filepath.Join(baseDir, ".github", "instructions"),
		filepath.Join(baseDir, ".github", "skills"),
		filepath.Join(baseDir, ".github", "agents"),
		filepath.Join(baseDir, ".github", "prompts"),
	}
}

// ProjectLayout is where the preset writes project-level files; user scope maps them
// onto GlobalOutputPaths.
func (g *CopilotPresetGenerator) ProjectLayout() ProjectLayout {
	return ProjectLayout{
		RootFile: copilotInstructionsFile, RulesDir: ".github/instructions", SkillsDir: ".github/skills",
		AgentsDir: ".github/agents", CommandsDir: ".github/prompts",
	}
}

// GlobalOutputPaths is the Copilot user-scope layout under ~/.copilot: the
// personal instructions file, skills and agents.
func (g *CopilotPresetGenerator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	return GlobalLayout{
		RootFile:  ".copilot/copilot-instructions.md",
		RulesDir:  ".copilot/instructions",
		SkillsDir: ".copilot/skills",
		AgentsDir: ".copilot/agents",
		Sidecars:  map[string]string{MergedDocCopilotHooks: ".copilot/hooks/ai-rulez.json"},
		// Copilot also reads the shared agent skill directory.
		SkillReaders: []string{".copilot/skills", agentsSkillsDir},
	}.Resolve(home, getenv)
}

func (g *CopilotPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .github directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".github"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".github", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".github", "agents"),
			IsDir: true,
		},
	)

	// Generate copilot-instructions.md
	ruleFiles, inlineRules, inlineContext, err := planCopilotRules(content, cfg)
	if err != nil {
		return nil, err
	}
	// With agents_md the shared AGENTS.md carries the always-on and the auto and
	// manual items, and copilot-instructions.md would shadow it in other tools.
	if !cfg.ReadsSharedAgentsMD(presetNameCopilot) {
		rulefiles.WarnUnreadScopeFile(cfg, presetNameCopilot, copilotInstructionsFile, inlineRules, inlineContext)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".github", "copilot-instructions.md"),
			Content: g.renderInstructionsFile(cfg, inlineRules, inlineContext),
		})
	}
	ruleOutputs, err := renderCopilotRuleFiles(ruleFiles, baseDir, cfg)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, ruleOutputs...)

	// Generate skill files to .github/skills/
	allSkills := allSkills(content)
	for idx := range allSkills {
		skill := allSkills[idx]
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".github", "skills", skillID)
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

	// Generate agent files to .github/agents/ (uses .agent.md extension)
	allAgents := allAgents(content)
	for idx := range allAgents {
		agent := allAgents[idx]
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderCopilotAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".github", "agents", agentID+".agent.md"),
			Content: agentContent,
		})
	}

	// Generate .github/prompts directory. GitHub Copilot reads reusable
	// prompt files from .github/prompts/*.prompt.md.
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".github", "prompts"),
		IsDir: true,
	})

	// Generate command files to .github/prompts/*.prompt.md
	allCommands := allCommands(content)
	for idx := range allCommands {
		command := allCommands[idx]
		if !g.shouldIncludeCommand(command) {
			continue
		}
		sanitized := sanitizeName(command.Name)
		commandContent := g.renderCommandFile(command)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".github", "prompts", sanitized+".prompt.md"),
			Content: commandContent,
		})
	}

	// VS Code (Copilot Chat) reads workspace MCP servers from .vscode/mcp.json,
	// under `servers`; the file often carries comments and the user's own servers,
	// so merge into it.
	if servers := mcpEntries(cfg, VSCodeMCPEntry); len(servers) > 0 {
		path := filepath.Join(baseDir, filepath.FromSlash(MergedDocVSCodeMCP))
		doc, err := renderMergedMCP(cfg, path, docmerge.FormatJSONC, []string{keyServers}, servers)
		if err != nil {
			return nil, fmt.Errorf("render .vscode/mcp.json: %w", err)
		}
		outputs = append(outputs, mergedOutput(path, doc))
	}

	// Generate .mcp.json if MCP servers are configured. A tracked, hand-authored
	// /.mcp.json is a common pattern, so merge the owned mcpServers key into what
	// is already there instead of replacing the document (#185).
	if len(cfg.MCPServers) > 0 {
		mcpPath := filepath.Join(baseDir, MergedDocMCPJSON)
		mcpFile, err := g.renderMCPJSON(mcpPath, cfg)
		if err != nil {
			return nil, fmt.Errorf("render .mcp.json: %w", err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:           mcpPath,
			Content:        mcpFile.Body,
			PartiallyOwned: mcpFile.PartiallyOwned,
			MergeClaims:    mcpFile.Claims,
		})
	}

	permissionOutputs, err := g.permissionsOutputs(cfg, baseDir)
	if err != nil {
		return nil, fmt.Errorf("render copilot permissions: %w", err)
	}
	outputs = append(outputs, permissionOutputs...)

	hookOutputs, err := g.hooksOutputs(cfg, baseDir)
	if err != nil {
		return nil, err
	}
	return append(outputs, hookOutputs...), nil
}

// planCopilotRules routes rules and context between .github/instructions files
// and the inline remainder of copilot-instructions.md. Copilot applies an
// instructions file on GitHub.com only through applyTo, so items that are not
// always-on or glob-scoped (auto, manual) stay inline instead of becoming
// files that would never be applied automatically.
func planCopilotRules(content *config.ContentTree, cfg *config.Config) (files []rulefiles.Item, rules, ctx []config.ContentFile, err error) {
	return planCopilotItems(allInlineRules(content), allInlineContext(content), cfg, rulefiles.ScopeOf(cfg), rulefiles.RegistryFor(cfg, presetNameCopilot), false)
}

// planCopilotItems is planCopilotRules over explicit rule and context lists, so
// machine-local rules are routed exactly like shared ones. Local rules pass a
// zero scope and a nil registry: they are root-only and their files never
// compete with the shared ones for a path.
//
// local is set for machine-local rules, which AGENTS.md does not carry: their
// always-on rules stay files whatever agents_md says.
func planCopilotItems(allRules, allContext []config.ContentFile, cfg *config.Config, scope rulefiles.ScopeInfo, reg *rulefiles.Registry, local bool,
) (files []rulefiles.Item, rules, ctx []config.ContentFile, err error) {
	routing := rulefiles.RoutingFor(cfg.RulesModeFor(presetNameCopilot), true)
	if !local {
		routing = routingWithSharedAgentsMD(cfg, presetNameCopilot, routing)
	}
	target := copilotRulesTarget
	// Items Copilot cannot apply automatically stay inline and never reach
	// Plan, so they cannot collide with the files that do get written.
	ruleCands, ruleInline := splitCopilotCandidates(cfg.Diag, allRules, "rule", target)
	ctxCands, ctxInline := splitCopilotCandidates(cfg.Diag, allContext, "context", target)
	planned, plannedRules, plannedContext, err := rulefiles.Plan(ruleCands, ctxCands, &target, routing,
		scope, reg)
	if err != nil {
		return nil, nil, nil, oops.With("preset", presetNameCopilot).Wrapf(err, "plan copilot rule files")
	}
	// The remainder is what Plan left inline plus the items it never saw, in
	// source order.
	keep := func(kind rulefiles.Kind, all, planInline, direct []config.ContentFile) (out []config.ContentFile) {
		want := make(map[string]struct{}, len(planInline)+len(direct))
		cfList := append(append([]config.ContentFile{}, planInline...), direct...)
		for idx := range cfList {
			cf := cfList[idx]
			want[copilotItemKey(kind, cf)] = struct{}{}
		}
		for idx := range all {
			cf := all[idx]
			if _, ok := want[copilotItemKey(kind, cf)]; ok {
				out = append(out, cf)
			}
		}
		return out
	}
	return planned, keep(rulefiles.KindRule, allRules, plannedRules, ruleInline),
		keep(rulefiles.KindContext, allContext, plannedContext, ctxInline), nil
}

// splitCopilotCandidates separates the items Plan may route to instructions
// files from the ones that stay in copilot-instructions.md (auto, manual and
// negated-only-glob items). The second result holds the latter that their
// targets still allow in that file.
func splitCopilotCandidates(d *diag.Collector, all []config.ContentFile, kind string, target rulefiles.Target,
) (candidates, inline []config.ContentFile) {
	stay := func(cf config.ContentFile) {
		if rulefiles.InlineAllowed(cf, target) {
			inline = append(inline, cf)
			return
		}
		if rulefiles.FileAllowed(cf, target, kindOf(kind)) {
			d.Raise(kind+" \""+cf.Name+"\" is targeted only at "+target.Dir+" but Copilot cannot apply it "+
				"automatically there; omitted", "path", cf.Path)
		}
	}
	for idx := range all {
		cf := all[idx]
		switch mode := rulefiles.EffectiveModeOf(cf); {
		case mode == config.ActivationAuto || mode == config.ActivationManual:
			stay(cf)
		case rulefiles.OnlyNegatedGlobs(cf):
			rulefiles.WarnOnlyNegated(d, kind, cf, "copilot-instructions.md")
			stay(cf)
		default:
			candidates = append(candidates, cf)
		}
	}
	return candidates, inline
}

func kindOf(kind string) rulefiles.Kind {
	if kind == "context" {
		return rulefiles.KindContext
	}
	return rulefiles.KindRule
}

func copilotItemKey(kind rulefiles.Kind, cf config.ContentFile) string {
	return strconv.Itoa(int(kind)) + "\x00" + cf.Path + "\x00" + cf.Name
}

func renderCopilotRuleFiles(items []rulefiles.Item, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	if len(items) == 0 {
		return nil, nil
	}
	t := copilotRulesTarget
	var outputs []config.OutputFile
	if !rulefiles.InScope(cfg) {
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(baseDir, filepath.FromSlash(t.Dir)), IsDir: true})
	}
	for i := range items {
		it := items[i]
		text, notes, err := rulefiles.Render(t, it, cfg)
		if err != nil {
			return nil, oops.With("preset", presetNameCopilot, "rule", it.File.Name).Wrapf(err, "render copilot instructions")
		}
		path := rulefiles.RulesDirPath(cfg, baseDir, t, rulefiles.FileName(t, it))
		rulefiles.ReportNotes(cfg.Diag, path, notes)
		outputs = append(outputs, config.OutputFile{Path: path, Content: text})
	}
	return outputs, nil
}

func (g *CopilotPresetGenerator) renderInstructionsFile(cfg *config.Config, allRules, allContext []config.ContentFile) string {
	var builder strings.Builder

	// Calculate content counts from the deduplicated rule set so the header
	// count matches what is actually rendered.
	ruleCount := len(allRules)

	// Generate and prepend header
	outputPath := copilotInstructionsFile
	header := generateCopilotPresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

	// Add header
	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	// Add rules section
	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Diag: cfg.Diag, Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Add context section
	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Diag: cfg.Diag, Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Skills are generated to .github/skills/ directory, not inlined

	return builder.String()
}

// renderSkillFile renders a skill file in SKILL.md format for Copilot: the shared
// skill rendering, which the copilot-cli spec writes to the same path.
func (g *CopilotPresetGenerator) renderSkillFile(skill config.ContentFile) string {
	return renderAgentSkillFile(skill)
}

// renderCopilotAgentFile renders an agent file with YAML frontmatter for Copilot
func (g *CopilotPresetGenerator) renderCopilotAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildCopilotAgentFrontmatter(agent, cfg)

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

// buildCopilotAgentFrontmatter builds frontmatter for a Copilot agent file
// copilotAgentFields are the agent frontmatter keys Copilot carries through.
// mcp-servers connects, so it must be in config's executing-key registry (a test
// checks it).
var copilotAgentFields = []string{
	keyDescription, "target",
	metaUserInvocable, metaDisableModelInvocation,
	"agents", "handoffs", "mcp-servers",
}

func (g *CopilotPresetGenerator) buildCopilotAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := ResolveAgentModel(presetNameCopilot, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	for _, field := range copilotAgentFields {
		if val, ok := typedAgentField(agent.Metadata, field); ok {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}

// shouldIncludeCommand checks if a command should be included in the Copilot preset
func (g *CopilotPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	if command.Metadata == nil {
		return true
	}
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == presetNameCopilot {
				return true
			}
		}
		return false
	}
	return true
}

// renderCommandFile renders a command as a Copilot prompt file: `description`
// (and `argument-hint`, when the command has one) as frontmatter and the command
// as the body. VS Code takes input from ${input:name}, not $ARGUMENTS, so the
// placeholder is rewritten to ${input:args}.
func (g *CopilotPresetGenerator) renderCommandFile(command config.ContentFile) string {
	frontmatter := map[string]any{}
	if command.Metadata != nil {
		if desc := command.Metadata.Extra[keyDescription]; desc != "" {
			frontmatter[keyDescription] = desc
		}
		if hint := command.Metadata.Extra["argument-hint"]; hint != "" {
			frontmatter["argument-hint"] = hint
		}
	}
	body := strings.ReplaceAll(markdown.ProcessEmbeddedContent(command.Content), "$ARGUMENTS", "${input:args}")
	if len(frontmatter) == 0 {
		return body
	}
	data, err := yaml.Marshal(frontmatter)
	if err != nil {
		return body
	}
	return "---\n" + string(data) + "---\n\n" + body
}

// renderMCPJSON renders the mcpServers key ai-rulez owns into the .mcp.json at
// mcpPath, preserving every other top-level key that is already there. An empty
// mcpPath renders a fresh document, which is what the unit tests exercise.
func (g *CopilotPresetGenerator) renderMCPJSON(mcpPath string, cfg *config.Config) (jsonmerge.Result, error) {
	return renderSharedMCPJSON(mcpPath, cfg)
}

// renderSharedMCPJSON renders the root .mcp.json in the one shape every writer of
// that path must agree on (the generator keeps a single copy and rejects
// divergent content): `disabled` on every entry, `type` for remote transports,
// the URL, headers and env as configured. It matches the mcp preset's output.
func renderSharedMCPJSON(mcpPath string, cfg *config.Config) (jsonmerge.Result, error) {
	mcpServers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			keyDisabled: !server.IsEnabled(),
		}

		// Remote transports are keyed on `type` (not `transport`). A stdio entry
		// with an empty command is invalid, so omit command/args for remote
		// (http/sse) transports and emit `type` plus the URL instead.
		switch t := server.GetTransport(); t {
		case config.TransportHTTP, config.TransportSSE:
			entry["type"] = t
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
		if server.URL != "" {
			entry["url"] = server.URL
		}

		ApplySharedMCPJSONRefs(entry, server, cfg)
		mcpServers[name] = entry
	}

	// The ai-rulez self server is on by default; add it in the same shape the mcp
	// preset's mcpJSONOwnedKeys builds, so every writer of .mcp.json agrees.
	if cfg.HasSelfServer() {
		if _, declared := mcpServers[config.SelfMCPServerName]; !declared {
			mcpServers[config.SelfMCPServerName] = cfg.SelfMCPServerEntry(schema.Version)
		}
	}

	return applyMergedDocument(cfg, mcpPath, []jsonmerge.OwnedKey{
		{Name: keyMCPServers, Value: mcpServers, Members: true},
	})
}
