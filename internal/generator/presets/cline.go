package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"gopkg.in/yaml.v3"
)

const presetNameCline = "cline"

var clineRulesTarget = rulefiles.Target{
	Preset: presetNameCline, Dir: ".clinerules", Ext: extMarkdown, Dialect: rulefiles.DialectCline, Banner: true,
}

func init() {
	config.RegisterPreset(presetNameCline, &ClinePresetGenerator{alwaysFileLocalRules{target: &clineRulesTarget, routing: rulefiles.RoutingEverything}})
}

// ClinePresetGenerator generates Cline preset files
type ClinePresetGenerator struct{ alwaysFileLocalRules }

func (g *ClinePresetGenerator) GetName() string {
	return presetNameCline
}

func (g *ClinePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".clinerules"),
		filepath.Join(baseDir, ".cline"),
		filepath.Join(baseDir, ".cline", "skills"),
		filepath.Join(baseDir, ".cline", "agents"),
	}
}

func (g *ClinePresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cline"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cline", "skills"),
			IsDir: true,
		},
	)

	// Rules and context are written as native rule files with frontmatter
	ruleOutputs, err := rulesFolderOutputs(clineRulesTarget, content, baseDir, cfg, routingWithSharedAgentsMD(cfg, presetNameCline, rulefiles.RoutingEverything), nil)
	if err != nil {
		return nil, fmt.Errorf("generate rule files: %w", err)
	}
	outputs = append(outputs, rulesDirMarker(cfg, presetNameCline, filepath.Join(baseDir, ".clinerules"), ruleOutputs)...)
	outputs = append(outputs, ruleOutputs...)

	// Generate skill files to .cline/skills/
	allSkills := allSkills(content)
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".cline", "skills", skillID)
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

	// Add .cline/agents directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".cline", "agents"),
		IsDir: true,
	})

	// Generate agent files to .cline/agents/
	allAgents := allAgents(content)
	for _, agent := range allAgents {
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderClineAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".cline", "agents", agentID+".md"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Cline
func (g *ClinePresetGenerator) renderSkillFile(skill config.ContentFile) string {
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

// renderClineAgentFile renders an agent file with YAML frontmatter for Cline
func (g *ClinePresetGenerator) renderClineAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildClineAgentFrontmatter(agent, cfg)

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

// buildClineAgentFrontmatter builds frontmatter for a Cline agent file
func (g *ClinePresetGenerator) buildClineAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := ResolveAgentModel(presetNameCline, agent, cfg); model != "" {
		frontmatter[keyModel] = model
	}

	if agent.Metadata == nil {
		return frontmatter
	}

	clineScalarFields := []string{keyDescription}
	for _, field := range clineScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") && len(agent.Metadata.Tools) > 0 {
		frontmatter["tools"] = agent.Metadata.Tools
	}

	return frontmatter
}
