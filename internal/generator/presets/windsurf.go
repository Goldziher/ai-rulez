package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
)

const windsurfPresetName = "windsurf"

// windsurfRulesTarget is the Windsurf rules folder. Windsurf truncates rule
// files beyond 12000 characters.
var windsurfRulesTarget = rulefiles.Target{
	Preset:   windsurfPresetName,
	Dir:      ".windsurf/rules",
	Ext:      extMarkdown,
	Dialect:  rulefiles.DialectTrigger,
	MaxChars: 12000,
	Banner:   true,
}

func init() {
	config.RegisterPreset(windsurfPresetName, &WindsurfPresetGenerator{alwaysFileLocalRules{target: &windsurfRulesTarget, routing: rulefiles.RoutingEverything}})
}

// WindsurfPresetGenerator generates Windsurf preset files
type WindsurfPresetGenerator struct{ alwaysFileLocalRules }

func (g *WindsurfPresetGenerator) GetName() string {
	return windsurfPresetName
}

func (g *WindsurfPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".windsurf"),
		filepath.Join(baseDir, ".windsurf", "rules"),
		filepath.Join(baseDir, ".windsurf", "skills"),
		filepath.Join(baseDir, ".windsurf", "agents"),
	}
}

func (g *WindsurfPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create .windsurf directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".windsurf"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".windsurf", "skills"),
			IsDir: true,
		},
	)

	if !rulefiles.InScope(cfg) {
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(baseDir, ".windsurf", "rules"), IsDir: true})
	}

	// Windsurf has no root file: every rule and context item is a file, whatever [rules] mode says.
	ruleOutputs, err := rulesFolderOutputs(windsurfRulesTarget, content, baseDir, cfg, rulefiles.RoutingEverything, nil)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, ruleOutputs...)

	// Generate skill files to .windsurf/skills/
	allSkills := allSkills(content)
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".windsurf", "skills", skillID)
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

	// Add .windsurf/agents directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".windsurf", "agents"),
		IsDir: true,
	})

	// Generate agent files to .windsurf/agents/
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderWindsurfAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".windsurf", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Windsurf
func (g *WindsurfPresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

// renderWindsurfAgentFile renders an agent file with YAML frontmatter for Windsurf
func (g *WindsurfPresetGenerator) renderWindsurfAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildWindsurfAgentFrontmatter(agent, cfg)

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

// buildWindsurfAgentFrontmatter builds frontmatter for a Windsurf agent file
func (g *WindsurfPresetGenerator) buildWindsurfAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve effort before the metadata-nil short-circuit so a defaults-only effort
	// still applies to agents with no frontmatter.
	if effort := MapEffort(windsurfPresetName, ResolveAgentEffort(windsurfPresetName, agent, cfg)); effort != "" {
		frontmatter["reasoning_effort"] = effort
	}
	if model := ResolveAgentModel(windsurfPresetName, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	windsurfScalarFields := []string{keyDescription}
	for _, field := range windsurfScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
