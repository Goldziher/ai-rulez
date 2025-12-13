package presets

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("windsurf", &WindsurfPresetGenerator{})
}

// WindsurfPresetGenerator generates Windsurf preset files
type WindsurfPresetGenerator struct{}

func (g *WindsurfPresetGenerator) GetName() string {
	return "windsurf"
}

func (g *WindsurfPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".windsurf"),
	}
}

func (g *WindsurfPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .windsurf directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".windsurf"),
		IsDir: true,
	})

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		ruleContent := g.renderRuleFile(rule)
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".windsurf", sanitized+".md"),
			Content: ruleContent,
		})
	}

	return outputs, nil
}

func (g *WindsurfPresetGenerator) renderRuleFile(rule config.ContentFile) string {
	var builder strings.Builder

	// Add title
	builder.WriteString("# ")
	builder.WriteString(rule.Name)
	builder.WriteString("\n\n")

	// Add priority if present
	if rule.Metadata != nil && rule.Metadata.Priority != "" {
		builder.WriteString("**Priority:** ")
		builder.WriteString(rule.Metadata.Priority)
		builder.WriteString("\n\n")
	}

	// Add content
	builder.WriteString(rule.Content)

	return builder.String()
}
