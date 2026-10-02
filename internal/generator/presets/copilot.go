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
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const presetNameCopilot = "copilot"

// copilotRulesTarget is the path-specific instructions folder Copilot reads.
var copilotRulesTarget = rulefiles.Target{
	Preset:    presetNameCopilot,
	Dir:       ".github/instructions",
	Ext:       ".instructions.md",
	Dialect:   rulefiles.DialectCopilot,
	Recursive: true,
	Banner:    true,
}

func init() {
	config.RegisterPreset(presetNameCopilot, &CopilotPresetGenerator{})
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
		filepath.Join(baseDir, ".github", "commands"),
	}
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
	outputs = append(outputs, config.OutputFile{
		Path:    filepath.Join(baseDir, ".github", "copilot-instructions.md"),
		Content: g.renderInstructionsFile(cfg, inlineRules, inlineContext),
	})
	ruleOutputs, err := renderCopilotRuleFiles(ruleFiles, baseDir, cfg)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, ruleOutputs...)

	// Generate skill files to .github/skills/
	allSkills := allSkills(content)
	for _, skill := range allSkills {
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
	for _, agent := range allAgents {
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

	// Generate .github/commands directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".github", "commands"),
		IsDir: true,
	})

	// Generate command files to .github/commands/
	allCommands := allCommands(content)
	for _, command := range allCommands {
		if !g.shouldIncludeCommand(command) {
			continue
		}
		sanitized := sanitizeName(command.Name)
		commandContent := g.renderCommandFile(command)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".github", "commands", sanitized+".md"),
			Content: commandContent,
		})
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
		})
	}

	return outputs, nil
}

// planCopilotRules routes rules and context between .github/instructions files
// and the inline remainder of copilot-instructions.md. Copilot applies an
// instructions file on GitHub.com only through applyTo, so items that are not
// always-on or glob-scoped (auto, manual) stay inline instead of becoming
// files that would never be applied automatically.
func planCopilotRules(content *config.ContentTree, cfg *config.Config) (files []rulefiles.Item, rules, ctx []config.ContentFile, err error) {
	return planCopilotItems(allInlineRules(content), allInlineContext(content), cfg)
}

// planCopilotItems is planCopilotRules over explicit rule and context lists, so
// machine-local rules are routed exactly like shared ones.
func planCopilotItems(allRules, allContext []config.ContentFile, cfg *config.Config,
) (files []rulefiles.Item, rules, ctx []config.ContentFile, err error) {
	routing := rulefiles.RoutingFor(cfg.RulesModeFor(presetNameCopilot), true)
	target := copilotRulesTarget
	// Items Copilot cannot apply automatically stay inline and never reach
	// Plan, so they cannot collide with the files that do get written.
	candidates := func(all []config.ContentFile, kind string) (out []config.ContentFile) {
		for _, cf := range all {
			switch mode := rulefiles.EffectiveModeOf(cf); {
			case mode == config.ActivationAuto || mode == config.ActivationManual:
			case rulefiles.OnlyNegatedGlobs(cf):
				logger.Warn(kind+" \""+cf.Name+"\" has only negated globs, which Copilot's applyTo cannot express; "+
					"kept in copilot-instructions.md", "path", cf.Path)
			default:
				out = append(out, cf)
			}
		}
		return out
	}
	planned, _, _, err := rulefiles.Plan(candidates(allRules, "rule"), candidates(allContext, "context"),
		&target, routing, rulefiles.ScopeInfo{}, nil)
	if err != nil {
		return nil, nil, nil, oops.With("preset", presetNameCopilot).Wrapf(err, "plan copilot rule files")
	}
	filed := make(map[string]struct{}, len(planned))
	for i := range planned {
		it := planned[i]
		files = append(files, it)
		filed[copilotItemKey(it.Kind, it.File)] = struct{}{}
	}
	keepInline := func(kind rulefiles.Kind, all []config.ContentFile) (out []config.ContentFile) {
		for _, cf := range all {
			if _, ok := filed[copilotItemKey(kind, cf)]; !ok {
				out = append(out, cf)
			}
		}
		return out
	}
	return files, keepInline(rulefiles.KindRule, allRules), keepInline(rulefiles.KindContext, allContext), nil
}

func copilotItemKey(kind rulefiles.Kind, cf config.ContentFile) string {
	return strconv.Itoa(int(kind)) + "\x00" + cf.Path + "\x00" + cf.Name
}

func renderCopilotRuleFiles(items []rulefiles.Item, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	if len(items) == 0 {
		return nil, nil
	}
	t := copilotRulesTarget
	outputs := []config.OutputFile{{Path: filepath.Join(baseDir, filepath.FromSlash(t.Dir)), IsDir: true}}
	for i := range items {
		it := items[i]
		text, notes, err := rulefiles.Render(t, it, cfg)
		if err != nil {
			return nil, oops.With("preset", presetNameCopilot, "rule", it.File.Name).Wrapf(err, "render copilot instructions")
		}
		path := filepath.Join(baseDir, filepath.FromSlash(t.Dir), filepath.FromSlash(rulefiles.FileName(t, it)))
		rulefiles.ReportNotes(path, notes)
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
	outputPath := ".github/copilot-instructions.md"
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
	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Add context section
	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Skills are generated to .github/skills/ directory, not inlined

	return builder.String()
}

// renderSkillFile renders a skill file in SKILL.md format for Copilot
func (g *CopilotPresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

	copilotFields := []string{
		keyDescription, "target",
		"user-invocable", "disable-model-invocation",
		"agents", "handoffs", "mcp-servers",
	}
	for _, field := range copilotFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
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

// renderCommandFile renders a command file in Markdown format for Copilot
func (g *CopilotPresetGenerator) renderCommandFile(command config.ContentFile) string {
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

// renderMCPJSON renders the mcpServers key ai-rulez owns into the .mcp.json at
// mcpPath, preserving every other top-level key that is already there. An empty
// mcpPath renders a fresh document, which is what the unit tests exercise.
func (g *CopilotPresetGenerator) renderMCPJSON(mcpPath string, cfg *config.Config) (jsonmerge.Result, error) {
	mcpServers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			keyDisabled: !server.IsEnabled(),
		}

		// Copilot keys remote transport on `type` (not `transport`). A stdio entry
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

		mcpServers[name] = entry
	}

	return applyMergedDocument(mcpPath, []jsonmerge.OwnedKey{
		{Name: keyMCPServers, Value: mcpServers},
	})
}
