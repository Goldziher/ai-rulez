package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/markdown"
)

const devinPresetName = "devin"

// devinRulesTarget is the Devin rules folder. Devin truncates rule files beyond
// 12000 characters.
var devinRulesTarget = rulefiles.Target{
	Preset:   devinPresetName,
	Dir:      ".devin/rules",
	Ext:      extMarkdown,
	Dialect:  rulefiles.DialectTrigger,
	MaxChars: 12000,
	Banner:   true,
}

func init() {
	config.RegisterPreset(devinPresetName, &DevinPresetGenerator{alwaysFileLocalRules{target: &devinRulesTarget, routing: rulefiles.RoutingEverything}})
}

// DevinPresetGenerator generates Devin preset files
type DevinPresetGenerator struct{ alwaysFileLocalRules }

func (g *DevinPresetGenerator) GetName() string {
	return devinPresetName
}

func (g *DevinPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".devin"),
		filepath.Join(baseDir, ".devin", "rules"),
		filepath.Join(baseDir, ".devin", "skills"),
		filepath.Join(baseDir, ".devin", "agents"),
	}
}

// ProjectLayout is where the preset writes project-level files; user scope maps them
// onto GlobalOutputPaths.
func (g *DevinPresetGenerator) ProjectLayout() ProjectLayout {
	return ProjectLayout{RootFile: "AGENTS.md", RulesDir: ".devin/rules", SkillsDir: ".devin/skills", AgentsDir: ".devin/agents"}
}

// GlobalOutputPaths is the Devin user-scope layout under ~/.config/devin.
func (g *DevinPresetGenerator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	return GlobalLayout{
		RootFile:  ".config/devin/AGENTS.md",
		SkillsDir: ".config/devin/skills",
		AgentsDir: ".config/devin/agents",
		Sidecars: map[string]string{
			MergedDocDevinMCP:    ".config/devin/mcp_config.json",
			MergedDocDevinConfig: ".config/devin/config.json",
		},
	}.Resolve(home, getenv)
}

// renderCommandSkill renders a command as a user-invocable Devin skill: Devin has
// no commands folder, a skill is invoked as /{name}.
func renderCommandSkill(command config.ContentFile) (string, error) {
	desc := commandDescription(command)
	if desc == "" {
		desc = command.Name + " command"
	}
	data, err := yaml.Marshal(map[string]any{
		keyName:        sanitizeName(command.Name),
		keyDescription: desc,
		"triggers":     []string{"user"},
	})
	if err != nil {
		return "", fmt.Errorf("marshal command frontmatter: %w", err)
	}
	return "---\n" + string(data) + "---\n\n" + markdown.ProcessEmbeddedContent(command.Content), nil
}

func (g *DevinPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .devin directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".devin"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".devin", "skills"),
			IsDir: true,
		},
	)

	// Devin has no root file: every rule and context item is a file, whatever [rules] mode says.
	ruleOutputs, err := rulesFolderOutputs(devinRulesTarget, content, baseDir, cfg, routingWithSharedAgentsMD(cfg, devinPresetName, rulefiles.RoutingEverything), nil)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, rulesDirMarker(cfg, devinPresetName, filepath.Join(baseDir, ".devin", "rules"), ruleOutputs)...)
	outputs = append(outputs, ruleOutputs...)

	// Generate skill files to .devin/skills/
	allSkills := allSkills(content)
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".devin", "skills", skillID)
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

	// Commands become user-invocable skills, unless a skill of that id exists.
	skillIDs := map[string]bool{}
	for _, skill := range allSkills {
		skillIDs[extractSkillID(skill.Path)] = true
	}
	for _, command := range allCommands(content) {
		id := sanitizeName(command.Name)
		if skillIDs[id] || !commandTargets(command, devinPresetName) {
			continue
		}
		text, err := renderCommandSkill(command)
		if err != nil {
			return nil, err
		}
		skillDir := filepath.Join(baseDir, ".devin", "skills", id)
		outputs = append(outputs,
			config.OutputFile{Path: skillDir, IsDir: true},
			config.OutputFile{Path: filepath.Join(skillDir, "SKILL.md"), Content: text})
	}

	// Devin reads project MCP servers from .devin/mcp_config.json.
	if servers := mcpEntries(cfg, devinMCPEntry); len(servers) > 0 {
		path := filepath.Join(baseDir, filepath.FromSlash(MergedDocDevinMCP))
		doc, err := renderMergedMCP(path, docmerge.FormatJSON, []string{keyMCPServers}, servers)
		if err != nil {
			return nil, fmt.Errorf("render .devin/mcp_config.json: %w", err)
		}
		outputs = append(outputs, mergedOutput(path, doc))
	}

	settingsOutputs, err := g.settingsOutputs(cfg, baseDir)
	if err != nil {
		return nil, fmt.Errorf("render devin permissions and hooks: %w", err)
	}
	outputs = append(outputs, settingsOutputs...)

	// Add .devin/agents directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".devin", "agents"),
		IsDir: true,
	})

	// Generate agent files to .devin/agents/
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderDevinAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".devin", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Devin
func (g *DevinPresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

// renderDevinAgentFile renders an agent file with YAML frontmatter for Devin
func (g *DevinPresetGenerator) renderDevinAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildDevinAgentFrontmatter(agent, cfg)

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

// buildDevinAgentFrontmatter builds frontmatter for a Devin agent file
func (g *DevinPresetGenerator) buildDevinAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve effort before the metadata-nil short-circuit so a defaults-only effort
	// still applies to agents with no frontmatter.
	if effort := MapEffort(devinPresetName, ResolveAgentEffort(devinPresetName, agent, cfg)); effort != "" {
		frontmatter["reasoning_effort"] = effort
	}
	if model := ResolveAgentModel(devinPresetName, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	devinScalarFields := []string{keyDescription}
	for _, field := range devinScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
