package presets

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
)

// SharedAgentsMD renders the shared AGENTS.md written once when agents_md is on.
// It is the file codex, opencode, xum and amp each render today (their roots are
// byte-identical by design), so the shared file matches what any one of them
// produced alone. owners are the configured presets relying on the file; they
// extend the default AGENTS.md owners, so an item targeted at claude or gemini
// lands in the shared file when that preset imports or reads it.
func SharedAgentsMD(content *config.ContentTree, baseDir string, cfg *config.Config, owners []string) config.OutputFile {
	all := rulefiles.RootOwners("AGENTS.md")
	for _, owner := range owners {
		if !slices.Contains(all, owner) {
			all = append(all, owner)
		}
	}
	return config.OutputFile{
		Path:    filepath.Join(baseDir, "AGENTS.md"),
		Content: (&CodexPresetGenerator{}).renderAgentsMarkdownFor(content, cfg, all),
	}
}

// SharedAgentSkills renders the shared .agents/skills tree: one
// <name>/SKILL.md per skill, in the generic Agent Skills format (name and
// description frontmatter), plus bundled resources. It returns nothing when the
// content has no skills.
func SharedAgentSkills(content *config.ContentTree, baseDir string) []config.OutputFile {
	skills := allSkills(content)
	if len(skills) == 0 {
		return nil
	}
	root := filepath.Join(baseDir, ".agents", "skills")
	outputs := []config.OutputFile{
		{Path: filepath.Join(baseDir, ".agents"), IsDir: true},
		{Path: root, IsDir: true},
	}
	for _, skill := range skills {
		skillDir := filepath.Join(root, extractSkillID(skill.Path))
		outputs = append(outputs,
			config.OutputFile{Path: skillDir, IsDir: true},
			config.OutputFile{Path: filepath.Join(skillDir, "SKILL.md"), Content: renderAgentSkillFile(skill)},
		)
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}
	return outputs
}

func renderAgentSkillFile(skill config.ContentFile) string {
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
