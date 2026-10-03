package presets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const presetNameGemini = "gemini"

func init() {
	config.RegisterPreset(presetNameGemini, &GeminiPresetGenerator{})
}

// GeminiPresetGenerator generates Gemini preset files
type GeminiPresetGenerator struct{}

// generatePresetHeader creates a header for Gemini preset files
func generateGeminiPresetHeader(cfg *config.Config, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *GeminiPresetGenerator) GetName() string {
	return presetNameGemini
}

// LocalRootFile implements config.LocalRootProvider: GEMINI.md → GEMINI.local.md.
func (g *GeminiPresetGenerator) LocalRootFile() string {
	return config.LocalVariantPath("GEMINI.md")
}

func (g *GeminiPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".gemini"),
		filepath.Join(baseDir, "GEMINI.md"),
		filepath.Join(baseDir, ".agents"),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, ".gemini", "agents"),
	}
}

func (g *GeminiPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".gemini"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".agents"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".agents", "skills"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".gemini", "agents"),
			IsDir: true,
		},
	)

	// Generate .gemini/settings.json with MCP configuration.
	//
	// The file is the consumer's: ai-rulez owns the mcpServers key and Gemini CLI
	// users hand-author everything else (theme, contextFileName, telemetry, ...),
	// so merge into what is already on disk rather than rendering a fresh document
	// over it (#185).
	//
	// Merging alone is not enough, because an owned key is replaced wholesale. An
	// unconditional write would still hand a Gemini user who keeps their own MCP
	// servers in this file nothing but the ai-rulez self-registration entry, on
	// every run, even when this config declares no servers at all. Gating on
	// has-MCP-servers (as the claude provider's sidecar does) costs that
	// self-registration in projects with no [[mcp_servers]] and keeps their file
	// intact instead, which is the better trade.
	//
	// With agents_md the document also owns context.fileName (see
	// geminiContextFileNamePath), which makes it worth writing without MCP
	// servers; switching the flag off removes the value it wrote.
	settingsPath := filepath.Join(baseDir, filepath.FromSlash(MergedDocGeminiSettings))
	owned, err := g.settingsKeys(settingsPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("render settings.json: %w", err)
	}
	if len(owned) > 0 {
		settings, err := applyMergedDocument(settingsPath, owned)
		if err != nil {
			return nil, fmt.Errorf("render settings.json: %w", err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:           settingsPath,
			Content:        settings.Body,
			PartiallyOwned: settings.PartiallyOwned,
		})
	}

	// Generate GEMINI.md with all rules and context. With agents_md the shared
	// AGENTS.md replaces it (the generator drops this output), so skip rendering.
	if !cfg.AgentsMD {
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, "GEMINI.md"),
			Content: g.renderGeminiMarkdown(content, cfg),
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

		skillContent := g.renderGeminiSkillFile(skill)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate agent files to .gemini/agents/, the only project location Gemini
	// CLI loads subagents from.
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderGeminiAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".gemini", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// readsSharedAgentsMD reports whether gemini takes AGENTS.md from the shared
// output, which a monorepo scope run leaves to the root settings: Gemini reads
// the project settings only, and the root document covers nested AGENTS.md files.
func (g *GeminiPresetGenerator) readsSharedAgentsMD(cfg *config.Config) bool {
	return cfg.AgentsMD && !rulefiles.InScope(cfg)
}

// renderSettings renders the keys ai-rulez owns in .gemini/settings.json into
// the document at settingsPath.
func (g *GeminiPresetGenerator) renderSettings(settingsPath string, cfg *config.Config) (jsonmerge.Result, error) {
	owned, err := g.settingsKeys(settingsPath, cfg)
	if err != nil {
		return jsonmerge.Result{}, err
	}
	return applyMergedDocument(settingsPath, owned)
}

// settingsKeys lists the keys ai-rulez owns in .gemini/settings.json: mcpServers
// when the config has servers, and context.fileName with agents_md. With the
// flag off, a context.fileName that is exactly what agents_md wrote is removed
// when ai-rulez wrote the whole document (it is in the previous run's generated
// manifest), since Gemini would otherwise keep ignoring the GEMINI.md that is
// back; in a document the user authored, and for any other value, it is only
// warned about when it lacks GEMINI.md.
func (g *GeminiPresetGenerator) settingsKeys(settingsPath string, cfg *config.Config) ([]jsonmerge.OwnedKey, error) {
	var owned []jsonmerge.OwnedKey
	if len(cfg.MCPServers) > 0 {
		owned = append(owned, jsonmerge.OwnedKey{Name: keyMCPServers, Value: g.mcpServersValue(cfg)})
	}
	switch {
	case g.readsSharedAgentsMD(cfg):
		names, _, err := readGeminiContextFileNames(settingsPath)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(names, string(config.SharedAgentsMD)) {
			names = append(names, string(config.SharedAgentsMD))
		}
		owned = append(owned, jsonmerge.OwnedKey{Path: geminiContextFileNamePath, Value: names})
	case !cfg.AgentsMD && !rulefiles.InScope(cfg):
		names, isList, err := readGeminiContextFileNames(settingsPath)
		if err != nil {
			return nil, err
		}
		onlyAgentsMD := isList && slices.Equal(names, []string{string(config.SharedAgentsMD)})
		switch {
		case onlyAgentsMD && g.wroteSettings(settingsPath, cfg):
			owned = append(owned, jsonmerge.OwnedKey{Path: geminiContextFileNamePath, Remove: true})
		case onlyAgentsMD:
			rulefiles.Warn("agents_md is off but .gemini/settings.json context.fileName is [\"AGENTS.md\"], so Gemini CLI "+
				"ignores the generated GEMINI.md; the file is not one ai-rulez wrote, so the value is left alone",
				"hint", "add \"GEMINI.md\" to context.fileName or remove the key", "path", settingsPath)
		case slices.Contains(names, string(config.SharedAgentsMD)) && !slices.Contains(names, "GEMINI.md"):
			rulefiles.Warn("agents_md is off but .gemini/settings.json context.fileName still lists AGENTS.md without GEMINI.md, "+
				"so Gemini CLI ignores the generated GEMINI.md",
				"hint", "add \"GEMINI.md\" to context.fileName or remove the key", "path", settingsPath)
		}
	}
	return owned, nil
}

// wroteSettings reports whether the previous run wrote the settings document
// whole (it is in the generated manifest). A document ai-rulez only merged keys
// into is the user's, and so is a context.fileName in it, whatever its value.
func (g *GeminiPresetGenerator) wroteSettings(settingsPath string, cfg *config.Config) bool {
	rel, err := filepath.Rel(cfg.BaseDir, settingsPath)
	return err == nil && cfg.Run.WasGenerated(rel)
}

// readGeminiContextFileNames returns the context.fileName names the document at
// settingsPath configures, in order, and whether the value is a list (a single
// string is reported as one name). Existing names are kept when agents_md adds
// AGENTS.md, because Gemini replaces its default (GEMINI.md) with whatever is
// configured. A missing file configures none.
func readGeminiContextFileNames(settingsPath string) (names []string, isList bool, err error) {
	if settingsPath == "" {
		return nil, false, nil
	}
	data, err := os.ReadFile(settingsPath) //nolint:gosec // path is derived from the config base dir
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, oops.With("path", settingsPath).Wrapf(err, "read gemini settings")
	}
	names, isList = existingContextFileNames(data)
	return names, isList, nil
}

// geminiContextFileNamePath addresses context.fileName in the settings document.
var geminiContextFileNamePath = []string{geminiContextKey, "fileName"}

const geminiContextKey = "context"

// existingContextFileNames reads context.fileName (a string or a list of
// strings) from a settings document; unparseable input yields none, since the
// merge reports the syntax error itself.
func existingContextFileNames(data []byte) (names []string, isList bool) {
	var doc struct {
		Context struct {
			FileName json.RawMessage `json:"fileName"`
		} `json:"context"`
	}
	if json.Unmarshal(data, &doc) != nil || len(doc.Context.FileName) == 0 {
		return nil, false
	}
	var list []string
	if json.Unmarshal(doc.Context.FileName, &list) == nil {
		return list, true
	}
	var single string
	if json.Unmarshal(doc.Context.FileName, &single) == nil && single != "" {
		return []string{single}, false
	}
	return nil, false
}

// mcpServersValue is the mcpServers value ai-rulez owns in the settings document.
func (g *GeminiPresetGenerator) mcpServersValue(cfg *config.Config) map[string]interface{} {
	mcpServers := make(map[string]interface{})

	// Always include the hardcoded ai-rulez MCP server
	mcpServers["ai-rulez"] = map[string]interface{}{
		keyCommand: cmdNPX,
		keyArgs: []string{
			"-y",
			"ai-rulez@latest",
			keyMCP,
		},
	}

	// Merge user-configured MCP servers
	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{}

		// Gemini has no transport/type key. Remote servers key on the URL field:
		// `httpUrl` for streamable HTTP, `url` for SSE. A stdio entry with an empty
		// command is invalid, so omit command/args for remote transports.
		switch server.GetTransport() {
		case config.TransportHTTP:
			if server.URL != "" {
				entry["httpUrl"] = server.URL
			}
		case config.TransportSSE:
			if server.URL != "" {
				entry["url"] = server.URL
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
		if t := server.GetTransport(); (t == config.TransportHTTP || t == config.TransportSSE) && len(server.Headers) > 0 {
			entry[keyHeaders] = server.Headers
		}
		if !server.IsEnabled() {
			entry[keyDisabled] = true
		}

		mcpServers[name] = entry
	}

	return mcpServers
}

func (g *GeminiPresetGenerator) renderGeminiMarkdown(content *config.ContentTree, cfg *config.Config) string {
	var builder strings.Builder

	// Calculate content counts
	allRules := rootRules(content, presetNameGemini, "GEMINI.md")
	allAgents := allAgents(content)

	// Add header before title
	header := generateGeminiPresetHeader(cfg, "GEMINI.md", len(allRules), 0, len(allAgents))
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
	allContext := rootContext(content, presetNameGemini, "GEMINI.md")
	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)

	// Add agents section listing available subagents (if agent-delegation builtin is enabled)
	renderAgentsSection(&builder, content, allAgents)

	// Skills are generated to .agents/skills/ directory, not inlined in GEMINI.md

	return builder.String()
}

// renderGeminiSkillFile renders a skill file in SKILL.md format for Gemini
func (g *GeminiPresetGenerator) renderGeminiSkillFile(skill config.ContentFile) string {
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

// renderGeminiAgentFile renders an agent file with YAML frontmatter for Gemini
func (g *GeminiPresetGenerator) renderGeminiAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildGeminiAgentFrontmatter(agent, cfg)

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

// geminiClaudeAlias matches the bare Claude model aliases, which no Gemini model
// id can be.
var geminiClaudeAlias = regexp.MustCompile(`(?i)^(sonnet|opus|haiku)$`)

// resolveGeminiModel returns the agent's model when Gemini CLI can use it,
// otherwise "". A bare Claude alias (sonnet, opus, haiku) is no Gemini model, so
// the agent is emitted without one and inherits the session model; an explicit
// gemini_model or defaults.model_by_preset.gemini is kept verbatim.
func resolveGeminiModel(agent config.ContentFile, cfg *config.Config) string {
	model := ResolveAgentModel(presetNameGemini, agent, cfg)
	if model == "" || !geminiClaudeAlias.MatchString(model) {
		return model
	}
	warnGeminiAliasOnce(agent.Name, model)
	return ""
}

// warnGeminiAliasOnce warns that an agent's Claude alias was dropped, once per
// agent and model: an overlay renders the shared baseline as well as the merged
// view, so the same agent is resolved twice per run. It reports whether it warned.
func warnGeminiAliasOnce(agent, model string) bool {
	if _, seen := geminiAliasWarned.LoadOrStore(agent+"\x00"+model, struct{}{}); seen {
		return false
	}
	logger.Warn("Gemini CLI does not know Claude model aliases; omitting the model so the agent inherits the session model",
		"agent", agent, "model", model,
		"hint", "set gemini_model in the agent frontmatter or defaults.model_by_preset.gemini")
	return true
}

// geminiAliasWarned records the agent/model pairs already warned about.
var geminiAliasWarned sync.Map

// buildGeminiAgentFrontmatter builds frontmatter for a Gemini agent file. Gemini
// requires name and description; an agent without a description gets a generic one.
func (g *GeminiPresetGenerator) buildGeminiAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName:        agent.Name,
		keyDescription: "Subagent " + agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := resolveGeminiModel(agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	geminiScalarFields := []string{keyDescription, keyKind, keyTemperature, "max_turns", "timeout_mins"}
	for _, field := range geminiScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
