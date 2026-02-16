package presets

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func init() {
	config.RegisterPresetV3("windsurf", &WindsurfPresetGenerator{})
}

// WindsurfPresetGenerator generates Windsurf preset files
type WindsurfPresetGenerator struct{}

// generateWindsurfPresetHeader creates a header for Windsurf preset files
func generateWindsurfPresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *WindsurfPresetGenerator) GetName() string {
	return "windsurf"
}

func (g *WindsurfPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".windsurf", "rules"),
	}
}

func (g *WindsurfPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .windsurf/rules directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".windsurf", "rules"),
		IsDir: true,
	})

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		outputPath := filepath.Join(".windsurf", "rules", sanitizeName(rule.Name)+".md")
		ruleContent := g.renderRuleFile(rule, cfg, outputPath, len(allRules))
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".windsurf", "rules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	return outputs, nil
}

// renderTriggerFrontmatter renders Windsurf trigger frontmatter if needed
func (g *WindsurfPresetGenerator) renderTriggerFrontmatter(builder *strings.Builder, rule config.ContentFile) {
	if rule.Metadata == nil {
		return
	}

	// Check if we should render trigger frontmatter
	if !rule.Metadata.ShouldRenderTriggerFrontmatter() {
		return
	}

	mode := rule.Metadata.GetTriggerMode()

	// Validate mode and warn if invalid
	if !config.IsValidTriggerMode(mode) {
		logger.Warn(
			"Unknown trigger mode in Windsurf rule, using default",
			"mode", mode,
			"rule", rule.Name,
			"default", config.TriggerManual,
		)
		mode = config.TriggerManual
	}

	// Only render if non-default
	if mode == config.TriggerManual {
		desc := rule.Metadata.GetTriggerDescription()
		glob := rule.Metadata.GetTriggerGlob()

		// Still render if has extra config
		if desc == "" && glob == "" {
			return
		}
	}

	builder.WriteString("---\n")
	fmt.Fprintf(builder, "trigger: %s\n", mode)

	if desc := rule.Metadata.GetTriggerDescription(); desc != "" {
		fmt.Fprintf(builder, "description: %s\n", desc)
	}

	if glob := rule.Metadata.GetTriggerGlob(); glob != "" {
		fmt.Fprintf(builder, "glob: %s\n", glob)
	}

	builder.WriteString("---\n\n")
}

func (g *WindsurfPresetGenerator) renderRuleFile(rule config.ContentFile, cfg *config.ConfigV3, outputPath string, ruleCount int) string {
	var builder strings.Builder

	// Render trigger frontmatter FIRST (must be at line 1 for Windsurf)
	g.renderTriggerFrontmatter(&builder, rule)

	// Generate and add header after frontmatter
	header := generateWindsurfPresetHeader(cfg, outputPath, ruleCount, 0, 0)
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
