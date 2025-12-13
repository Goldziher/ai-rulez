package presets

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("junie", &JuniePresetGenerator{})
}

// JuniePresetGenerator generates Junie preset files (.junie/guidelines.md)
type JuniePresetGenerator struct{}

func (g *JuniePresetGenerator) GetName() string {
	return "junie"
}

func (g *JuniePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".junie"),
		filepath.Join(baseDir, ".junie", "guidelines.md"),
	}
}

func (g *JuniePresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .junie directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".junie"),
		IsDir: true,
	})

	// Generate guidelines.md file
	guidelinesContent := g.renderGuidelinesMarkdown(content, cfg)

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".junie", "guidelines.md"),
		Content: guidelinesContent,
		IsDir:   false,
	})

	return outputs, nil
}

func (g *JuniePresetGenerator) renderGuidelinesMarkdown(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
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
