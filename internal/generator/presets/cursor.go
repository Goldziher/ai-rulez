package presets

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPreset("cursor", &CursorPresetGenerator{alwaysFileLocalRules{target: &cursorRulesTarget, routing: rulefiles.RoutingEverything}})
}

const presetNameCursor = "cursor"

// CursorPresetGenerator generates Cursor preset files
type CursorPresetGenerator struct{ alwaysFileLocalRules }

func (g *CursorPresetGenerator) GetName() string {
	return presetNameCursor
}

func (g *CursorPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".cursor"),
		filepath.Join(baseDir, ".cursor", "rules"),
		filepath.Join(baseDir, ".cursor", "commands"),
		filepath.Join(baseDir, ".agents"),
		filepath.Join(baseDir, ".agents", "skills"),
		filepath.Join(baseDir, ".cursor", "agents"),
	}
}

// ProjectLayout is where the preset writes project-level files; user scope maps them
// onto GlobalOutputPaths.
func (g *CursorPresetGenerator) ProjectLayout() ProjectLayout {
	return ProjectLayout{RulesDir: ".cursor/rules", SkillsDir: ".agents/skills", AgentsDir: ".cursor/agents", CommandsDir: ".cursor/commands"}
}

// GlobalOutputPaths is the Cursor user-scope layout under ~/.cursor. User rules
// live in Cursor's settings UI, so there is no root file.
func (g *CursorPresetGenerator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	return GlobalLayout{
		// Cursor reads ~/.agents/skills, the directory the project skills use too, so a
		// skill shared with codex and gemini is written once.
		SkillsDir:   ".agents/skills",
		AgentsDir:   ".cursor/agents",
		CommandsDir: ".cursor/commands",
		Sidecars: map[string]string{
			MergedDocCursorMCP:   ".cursor/mcp.json",
			MergedDocCursorHooks: ".cursor/hooks.json",
		},
		SkillReaders: []string{".cursor/skills", ".agents/skills", ".claude/skills", ".codex/skills"},
	}.Resolve(home, getenv)
}

func (g *CursorPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cursor"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cursor", "commands"),
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
			Path:  filepath.Join(baseDir, ".cursor", "agents"),
			IsDir: true,
		},
	)

	ruleOutputs, err := g.renderRuleFiles(content, baseDir, cfg)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, rulesDirMarker(cfg, presetNameCursor, filepath.Join(baseDir, ".cursor", "rules"), ruleOutputs)...)
	outputs = append(outputs, ruleOutputs...)

	// Combine all commands from root and domains
	allCommands := allCommands(content)

	// Generate command files to .cursor/commands/
	for _, command := range allCommands {
		// Check if command should be included (enabled and targets Cursor if specified)
		if g.shouldIncludeCommand(command) {
			commandContent := g.renderCommandFile(command)
			sanitized := sanitizeName(command.Name)

			outputs = append(outputs, config.OutputFile{
				Path:    filepath.Join(baseDir, ".cursor", "commands", sanitized+".md"),
				Content: commandContent,
			})
		}
	}

	// Checks collapse into Bugbot's single repository-root instruction file.
	checkOutputs, err := cursorCheckOutputs(content, baseDir, cfg)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, checkOutputs...)

	// Combine all skills from root and domains
	allSkills := allSkills(content)

	// Generate skill files to .agents/skills/
	for _, skill := range allSkills {
		// Check if skill should be included (enabled and targets Cursor if specified)
		if !g.shouldIncludeSkill(skill) {
			continue
		}

		skillID := extractSkillID(skill.Path)

		// Create skill directory
		skillDir := filepath.Join(baseDir, ".agents", "skills", skillID)
		outputs = append(outputs, config.OutputFile{
			Path:  skillDir,
			IsDir: true,
		})

		// Generate SKILL.md file
		skillContent := g.renderSkillFile(skill, cfg)
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})

		// Emit bundled resources alongside SKILL.md.
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Generate agent files to .cursor/agents/ (Cursor does not read .agents/agents)
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderCursorAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".cursor", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	// Cursor reads project MCP servers from .cursor/mcp.json, merged so the user's
	// own servers survive.
	if servers := mcpEntries(cfg, cursorMCPEntry); len(servers) > 0 {
		path := filepath.Join(baseDir, filepath.FromSlash(MergedDocCursorMCP))
		doc, err := renderMergedMCP(path, docmerge.FormatJSON, []string{keyMCPServers}, servers)
		if err != nil {
			return nil, fmt.Errorf("render .cursor/mcp.json: %w", err)
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
		return nil, fmt.Errorf("render cursor permissions: %w", err)
	}
	outputs = append(outputs, permissionOutputs...)

	hookOutputs, err := g.hooksOutputs(cfg, baseDir)
	if err != nil {
		return nil, err
	}
	return append(outputs, hookOutputs...), nil
}

// cursorRulesTarget is the Cursor rules folder. Every .mdc carries frontmatter
// (a file without it is manual-only in Cursor); the banner after it holds the
// freshness hashes.
var cursorRulesTarget = rulefiles.Target{
	Preset:    presetNameCursor,
	Dir:       ".cursor/rules",
	Ext:       ".mdc",
	Dialect:   rulefiles.DialectCursor,
	Recursive: true,
	Banner:    true,
}

// renderRuleFiles writes one .mdc per rule and per context file. Cursor always
// writes one file per item, so the `[rules] mode` setting does not apply.
func (g *CursorPresetGenerator) renderRuleFiles(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	return rulesFolderOutputs(cursorRulesTarget, content, baseDir, cfg, routingWithSharedAgentsMD(cfg, presetNameCursor, rulefiles.RoutingEverything), nil)
}

// shouldIncludeCommand checks if a command should be included in the Cursor preset
func (g *CursorPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if command.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Cursor is in targets
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == presetNameCursor {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// renderCommandFile renders a command file in Markdown format for Cursor commands
func (g *CursorPresetGenerator) renderCommandFile(command config.ContentFile) string {
	var builder strings.Builder

	// Add title
	builder.WriteString("# /")
	builder.WriteString(command.Name)
	builder.WriteString("\n\n")

	// Add description if present in metadata
	if command.Metadata != nil && command.Metadata.Extra != nil {
		if desc, ok := command.Metadata.Extra["description"]; ok && desc != "" {
			builder.WriteString("**Description:** ")
			builder.WriteString(desc)
			builder.WriteString("\n\n")
		}
	}

	// Add usage if present in metadata
	if command.Metadata != nil && command.Metadata.Usage != "" {
		builder.WriteString("**Usage:** `")
		builder.WriteString(command.Metadata.Usage)
		builder.WriteString("`\n\n")
	}

	// Add aliases if present in metadata
	if command.Metadata != nil && len(command.Metadata.Aliases) > 0 {
		builder.WriteString("**Aliases:** ")
		for i, alias := range command.Metadata.Aliases {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("`/")
			builder.WriteString(alias)
			builder.WriteString("`")
		}
		builder.WriteString("\n\n")
	}

	// Add command content
	builder.WriteString(command.Content)

	return builder.String()
}

// shouldIncludeSkill checks if a skill should be included in the Cursor preset
func (g *CursorPresetGenerator) shouldIncludeSkill(skill config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if skill.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Cursor is in targets
	if len(skill.Metadata.Targets) > 0 {
		for _, target := range skill.Metadata.Targets {
			if target == presetNameCursor {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// renderSkillFile renders a skill file in SKILL.md format for Cursor: the shared
// .agents/skills rendering, which carries Cursor's paths and
// disable-model-invocation keys.
func (g *CursorPresetGenerator) renderSkillFile(skill config.ContentFile, _ *config.Config) string {
	return renderAgentSkillFile(skill)
}

// renderCursorAgentFile renders an agent file with YAML frontmatter for Cursor
func (g *CursorPresetGenerator) renderCursorAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildCursorAgentFrontmatter(agent, cfg)

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

// buildCursorAgentFrontmatter builds frontmatter for a Cursor agent file
func (g *CursorPresetGenerator) buildCursorAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := ResolveAgentModel(presetNameCursor, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	if val := agent.Metadata.Extra[keyDescription]; val != "" {
		frontmatter[keyDescription] = val
	}
	// Cursor documents readonly and is_background as booleans.
	for _, field := range []string{"readonly", "is_background"} {
		raw := agent.Metadata.Extra[field]
		if raw == "" {
			continue
		}
		flag, err := strconv.ParseBool(raw)
		if err != nil {
			logger.Warn("Cursor agent field "+field+" must be true or false; omitting it", "agent", agent.Name, "value", raw)
			continue
		}
		frontmatter[field] = flag
	}

	return frontmatter
}

// renderMCPJSON renders the mcpServers key ai-rulez owns into the root .mcp.json at
// mcpPath, preserving every other top-level key that is already there, in the
// shape every writer of that file shares (see renderSharedMCPJSON). Cursor reads
// its servers from .cursor/mcp.json; this legacy file stays for other tools. An
// empty mcpPath renders a fresh document, which is what the unit tests exercise.
func (g *CursorPresetGenerator) renderMCPJSON(mcpPath string, cfg *config.Config) (jsonmerge.Result, error) {
	return renderSharedMCPJSON(mcpPath, cfg)
}
