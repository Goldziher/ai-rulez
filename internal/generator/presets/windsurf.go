package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

const windsurfPresetName = "windsurf"

// windsurfRulesTarget is the Windsurf rules folder. Windsurf truncates rule
// files beyond 12000 characters.
var windsurfRulesTarget = rulefiles.Target{
	Preset:    windsurfPresetName,
	Dir:       ".windsurf/rules",
	Ext:       extMarkdown,
	Dialect:   rulefiles.DialectTrigger,
	Recursive: true,
	MaxChars:  12000,
	Banner:    true,
}

func init() {
	config.RegisterPreset(windsurfPresetName, &WindsurfPresetGenerator{})
}

// WindsurfPresetGenerator generates Windsurf preset files
type WindsurfPresetGenerator struct{}

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
			Path:  filepath.Join(baseDir, ".windsurf", "rules"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".windsurf", "skills"),
			IsDir: true,
		},
	)

	// Windsurf always writes one file per rule, so rules route to files regardless of [rules] mode.
	ruleItems, ctxItems, err := windsurfRuleItems(allInlineRules(content), allInlineContext(content))
	if err != nil {
		return nil, err
	}
	items := append(append([]rulefiles.Item(nil), ruleItems...), ctxItems...)
	for i := range items {
		it := &items[i]
		text, notes, err := rulefiles.Render(windsurfRulesTarget, *it, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate rule %s: %w", it.File.Name, err)
		}
		for _, note := range notes {
			logger.Warn("Windsurf rule file", "note", note)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, filepath.FromSlash(windsurfRulesTarget.Dir), rulefiles.FileName(windsurfRulesTarget, *it)),
			Content: text,
		})
	}

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

// windsurfRuleItems routes every rule and context file to a rule file. Plan
// keeps unscoped context inline, but Windsurf has no root file, so those
// become always-on context files.
func windsurfRuleItems(rules, context []config.ContentFile) (ruleItems, ctxItems []rulefiles.Item, err error) {
	files, _, inlineCtx, err := rulefiles.Plan(rules, context, &windsurfRulesTarget, rulefiles.RoutingAll,
		rulefiles.ScopeInfo{}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("plan windsurf rule files: %w", err)
	}
	for i := range files {
		if files[i].Kind == rulefiles.KindContext {
			ctxItems = append(ctxItems, files[i])
		} else {
			ruleItems = append(ruleItems, files[i])
		}
	}
	for _, c := range inlineCtx {
		id := rulefiles.ID(c.Name)
		if id == "" {
			return nil, nil, fmt.Errorf("context %q (%s) yields an empty rule file id", c.Name, c.Path)
		}
		ctxItems = append(ctxItems, rulefiles.Item{
			File: c, Kind: rulefiles.KindContext, ID: id, Activation: c.Metadata.ResolveActivation(),
		})
	}
	return ruleItems, ctxItems, nil
}
