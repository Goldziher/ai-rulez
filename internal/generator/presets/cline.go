package presets

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func init() {
	config.RegisterPresetV3("cline", &ClinePresetGenerator{})
}

// ClinePresetGenerator generates Cline preset files
type ClinePresetGenerator struct{}

// generateClinePresetHeader creates a header for Cline preset files
func generateClinePresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
	// Create TemplateData for header generation
	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    time.Now(),
		ConfigFile:   "config.yaml", // V3 uses config.yaml
		OutputFile:   outputPath,
		Config:       cfg,
		RuleCount:    ruleCount,
		SectionCount: sectionCount,
		AgentCount:   agentCount,
	}

	return templates.GenerateHeader(data)
}

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
		outputPath := filepath.Join(".clinerules", sanitizeName(rule.Name)+".md")
		ruleContent := g.renderRuleFile(rule, cfg, outputPath, len(allRules))
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".clinerules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	return outputs, nil
}

func (g *ClinePresetGenerator) renderRuleFile(rule config.ContentFile, cfg *config.ConfigV3, outputPath string, ruleCount int) string {
	var builder strings.Builder

	// Generate and prepend header
	header := generateClinePresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

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
	processedContent := markdown.ProcessEmbeddedContent(rule.Content)
	builder.WriteString(processedContent)

	return builder.String()
}
