package presets

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("copilot", &CopilotPresetGenerator{})
}

// CopilotPresetGenerator generates GitHub Copilot preset files
type CopilotPresetGenerator struct{}

func (g *CopilotPresetGenerator) GetName() string {
	return "copilot"
}

func (g *CopilotPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".github"),
		filepath.Join(baseDir, ".github", "copilot-instructions.md"),
	}
}

func (g *CopilotPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .github directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".github"),
		IsDir: true,
	})

	// Generate copilot-instructions.md
	instructionsContent := g.renderInstructionsFile(content, cfg)
	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".github", "copilot-instructions.md"),
		Content: instructionsContent,
	})

	return outputs, nil
}

func (g *CopilotPresetGenerator) renderInstructionsFile(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Add header
	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	// Add rules section
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	if len(allRules) > 0 {
		builder.WriteString("## Rules\n\n")
		for _, rule := range allRules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n")

			if rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}

			builder.WriteString(rule.Content)
			builder.WriteString("\n\n")
		}
	}

	// Add context section
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	if len(allContext) > 0 {
		builder.WriteString("## Context\n\n")
		for _, ctx := range allContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")
			builder.WriteString(ctx.Content)
			builder.WriteString("\n\n")
		}
	}

	// Add skills section
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	if len(allSkills) > 0 {
		builder.WriteString("## Skills\n\n")
		for _, skill := range allSkills {
			builder.WriteString("### ")
			builder.WriteString(skill.Name)
			builder.WriteString("\n\n")
			builder.WriteString(skill.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}
