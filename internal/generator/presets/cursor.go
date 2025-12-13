package presets

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("cursor", &CursorPresetGenerator{})
}

// CursorPresetGenerator generates Cursor preset files
type CursorPresetGenerator struct{}

func (g *CursorPresetGenerator) GetName() string {
	return "cursor"
}

func (g *CursorPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".cursor"),
		filepath.Join(baseDir, ".cursor", "rules"),
	}
}

func (g *CursorPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .cursor directory
	outputs = append(outputs,
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cursor"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cursor", "rules"),
			IsDir: true,
		},
	)

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		ruleContent := g.renderRuleFile(rule)
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".cursor", "rules", sanitized+".mdc"),
			Content: ruleContent,
		})
	}

	return outputs, nil
}

func (g *CursorPresetGenerator) renderRuleFile(rule config.ContentFile) string {
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
