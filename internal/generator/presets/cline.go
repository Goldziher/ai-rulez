package presets

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("cline", &ClinePresetGenerator{})
}

// ClinePresetGenerator generates Cline preset files
type ClinePresetGenerator struct{}

func (g *ClinePresetGenerator) GetName() string {
	return "cline"
}

func (g *ClinePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".clinerules"),
	}
}

func (g *ClinePresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .clinerules directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".clinerules"),
		IsDir: true,
	})

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		ruleContent := g.renderRuleFile(rule)
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".clinerules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	return outputs, nil
}

func (g *ClinePresetGenerator) renderRuleFile(rule config.ContentFile) string {
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
