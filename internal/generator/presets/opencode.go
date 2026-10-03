package presets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/opencodev1"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
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

	// A v1 plugin in this project builds fine but never loads in OpenCode v2,
	// and OpenCode only logs that to its own server log; surface it here.
	opencodev1.WarnProject(baseDir)

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
	// $schema, mcp.servers and its own entry of instructions, so a fresh document
	// is entirely ours; any other key a hand-authored opencode.json carries is
	// preserved (#185, #194). The instructions entry is written at the project
	// root whether or not MCP servers exist (see renderMCPConfig); a monorepo
	// scope writes the document only for its servers.
	if len(cfg.MCPServers) > 0 || !rulefiles.InScope(cfg) {
		mcpPath := filepath.Join(baseDir, MergedDocOpencodeConfig)
		mcpFile, write, err := g.renderMCPDocument(mcpPath, cfg)
		if err != nil {
			return nil, fmt.Errorf("render opencode.json: %w", err)
		}
		if write {
			outputs = append(outputs, config.OutputFile{
				Path:           mcpPath,
				Content:        mcpFile.Body,
				PartiallyOwned: mcpFile.PartiallyOwned,
				MergeClaims:    mcpFile.Claims,
			})
		}
	}

	return outputs, nil
}

// renderMCPConfig is renderMCPDocument without the write decision, for callers
// that only want the merged body.
func (g *OpencodePresetGenerator) renderMCPConfig(mcpPath string, cfg *config.Config) (jsonmerge.Result, error) {
	result, _, err := g.renderMCPDocument(mcpPath, cfg)
	return result, err
}

// renderMCPDocument renders OpenCode's native v2 MCP servers and the machine-local
// instructions entry into opencode.json. It owns the nested mcp.servers key (when
// servers are configured) and, at the project root, the AGENTS.local.md entry of
// instructions: OpenCode reads no AGENTS.local.md on its own, and a listed file
// that is missing is skipped silently, so the entry is written whether or not
// local content exists. The top-level $schema is written only into a document
// ai-rulez creates (or already wrote). Every sibling key under mcp (such as
// mcp.timeout), every other top-level key and every other instructions entry
// survive the merge.
//
// write is false when the document uses comments (OpenCode accepts them) and no
// MCP servers need writing: merging would delete the comments, and the
// instructions entry is not worth that, so a warning says what is missing.
func (g *OpencodePresetGenerator) renderMCPDocument(mcpPath string, cfg *config.Config) (result jsonmerge.Result, write bool, err error) {
	var owned []jsonmerge.OwnedKey
	if g.ownsSchema(mcpPath, cfg) {
		owned = append(owned, jsonmerge.OwnedKey{Path: []string{keySchema}, Value: opencodeSchemaURL})
	}
	userEntries := false
	if !rulefiles.InScope(cfg) {
		entries, claimed, user, err := opencodeInstructions(mcpPath, g.LocalRootFile(), previousClaimedInstructions(cfg, mcpPath))
		if err != nil {
			return jsonmerge.Result{}, false, err
		}
		userEntries = user
		if entries != nil {
			owned = append(owned, jsonmerge.OwnedKey{Path: []string{opencodeInstructionsKey}, Value: entries, Elements: claimed})
		}
	}
	if len(cfg.MCPServers) > 0 {
		owned = append(owned, jsonmerge.OwnedKey{Path: []string{"mcp", "servers"}, Value: g.mcpServersValue(cfg), Members: true})
	}
	result, err = applyMergedDocument(mcpPath, owned)
	if err != nil && len(cfg.MCPServers) == 0 {
		rulefiles.Warn("opencode.json could not be merged into, so "+g.LocalRootFile()+
			" is not listed in its instructions and OpenCode does not load machine-local content: "+err.Error(),
			"hint", "add \""+g.LocalRootFile()+"\" to instructions by hand, or remove the comments", "path", mcpPath)
		return jsonmerge.Result{}, false, nil
	}
	// An entry the user listed is theirs even when it is the only key left.
	result.PartiallyOwned = result.PartiallyOwned || userEntries
	return result, true, err
}

// ownsSchema reports whether $schema is ai-rulez's in the document at path: it is
// when ai-rulez creates the document, or when the document already carries the
// value ai-rulez wrote (the previous run recorded it, or generated the whole
// file). A hand-authored document does not get one added.
func (g *OpencodePresetGenerator) ownsSchema(path string, cfg *config.Config) bool {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the config base dir
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return true
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		return false
	}
	var current string
	if json.Unmarshal(doc["$schema"], &current) != nil || current != opencodeSchemaURL {
		return false
	}
	rel := projectRelative(cfg, path)
	if cfg.Run.WasGenerated(rel) {
		return true
	}
	for _, claim := range cfg.Run.PreviousClaims(rel) {
		if slices.Equal(claim.Path, []string{keySchema}) {
			return true
		}
	}
	return false
}

// opencodeInstructionsKey is the top-level array of extra instruction files.
const opencodeInstructionsKey = "instructions"

// opencodeLocalEntries are the spellings of the local root file OpenCode resolves
// to the same path.
func opencodeLocalEntries() []any {
	local := config.LocalVariantPath("AGENTS.md")
	return []any{local, "./" + local}
}

// previousClaimedInstructions lists the instructions entries the previous run
// recorded as its own. A document the previous run wrote whole is all ai-rulez's,
// so every spelling of the entry in it is.
func previousClaimedInstructions(cfg *config.Config, path string) []any {
	rel := projectRelative(cfg, path)
	if cfg.Run.WasGenerated(rel) {
		return opencodeLocalEntries()
	}
	var entries []any
	for _, claim := range cfg.Run.PreviousClaims(rel) {
		if slices.Equal(claim.Path, []string{opencodeInstructionsKey}) {
			entries = append(entries, claim.Elements...)
		}
	}
	return entries
}

// opencodeInstructions returns the instructions array of the document at path
// with entry added once, the entries of it that are ai-rulez's (added now, or
// recorded by the previous run), and whether the document lists anything else (a
// user entry). Either spelling of the entry ("AGENTS.local.md" or
// "./AGENTS.local.md") counts as present. The other entries are kept verbatim and
// in order. A nil result means the existing value is not an array, which is the
// user's to fix: it is warned about and left alone.
func opencodeInstructions(path, entry string, previous []any) (entries, claimed []any, userEntries bool, err error) {
	entries, isArray, err := readOpencodeInstructions(path)
	if err != nil {
		return nil, nil, false, err
	}
	if !isArray {
		rulefiles.Warn("opencode.json instructions is not an array, so OpenCode cannot load "+entry+
			"; the value is yours and is left alone", "path", path)
		return nil, nil, true, nil
	}
	claimed = []any{}
	present := false
	for _, spelled := range opencodeLocalEntries() {
		if slices.Contains(entries, spelled) {
			present = true
			if slices.Contains(previous, spelled) {
				claimed = append(claimed, spelled)
			}
		}
	}
	if !present {
		entries = append(entries, entry)
		claimed = append(claimed, entry)
	}
	return entries, claimed, len(entries) > len(claimed), nil
}

// readOpencodeInstructions reads the instructions entries of the document at path.
// A missing file, an unparseable one (the merge reports its syntax) and a missing
// key all have none; isArray is false only for a value that is not an array.
func readOpencodeInstructions(path string) (entries []any, isArray bool, err error) {
	entries = []any{}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the config base dir
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return entries, true, nil
	case err != nil:
		return nil, false, oops.With("path", path).Wrapf(err, "read opencode.json")
	}
	var doc map[string]json.RawMessage
	parsed := json.Unmarshal(data, &doc) == nil
	raw, present := doc[opencodeInstructionsKey]
	if !parsed || !present {
		return entries, true, nil
	}
	return entries, json.Unmarshal(raw, &entries) == nil, nil
}

// mcpServersValue renders the configured MCP servers in OpenCode's v2 shape.
func (g *OpencodePresetGenerator) mcpServersValue(cfg *config.Config) map[string]interface{} {
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

	return servers
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

// OpencodeAgentSettings returns the agent settings OpenCode understands (the
// frontmatter keys the opencode preset writes), for callers that register the
// agent through OpenCode's plugin API instead of a markdown file. The model is
// "provider/model" with any variant as a separate "variant" key, and is absent
// when it cannot be resolved (a bare alias such as "sonnet" is dropped with a
// warning).
func OpencodeAgentSettings(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	return (&OpencodePresetGenerator{}).buildOpencodeAgentFrontmatter(agent, cfg)
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
