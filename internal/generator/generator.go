// Package generator provides output file generation for ai_rules.
package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/naamanhirschfeld/ai_rules/internal/config"
	"github.com/naamanhirschfeld/ai_rules/internal/templates"
)

// Generator handles the generation of output files from configuration.
type Generator struct {
	renderer *templates.Renderer
}

// New creates a new generator with the default template renderer.
func New() *Generator {
	return &Generator{
		renderer: templates.NewRenderer(),
	}
}

// NewWithRenderer creates a generator with a custom renderer.
func NewWithRenderer(renderer *templates.Renderer) *Generator {
	return &Generator{
		renderer: renderer,
	}
}

// GenerateAll generates all output files defined in the configuration.
func (g *Generator) GenerateAll(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return fmt.Errorf("no outputs defined in configuration")
	}

	templateData := templates.NewTemplateData(cfg)

	for i, output := range cfg.Outputs {
		if err := g.generateOutput(output, templateData); err != nil {
			return fmt.Errorf("failed to generate output %d (%s): %w", i, output.File, err)
		}
	}

	return nil
}

// GenerateOutput generates a single output file.
func (g *Generator) GenerateOutput(cfg *config.Config, outputFile string) error {
	templateData := templates.NewTemplateData(cfg)

	// Find the output configuration
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return fmt.Errorf("output file %s not found in configuration", outputFile)
	}

	return g.generateOutput(*targetOutput, templateData)
}

// generateOutput generates a single output file.
func (g *Generator) generateOutput(output config.Output, data *templates.TemplateData) error {
	// Render the template
	content, err := g.renderTemplate(output, data)
	if err != nil {
		return err
	}

	// Check if we need to write the file
	shouldWrite, err := g.shouldWriteFile(output.File, content)
	if err != nil {
		return fmt.Errorf("failed to check if file should be written: %w", err)
	}
	if !shouldWrite {
		return nil // File content is unchanged, skip writing
	}

	// Write the file
	return g.writeOutputFile(output.File, content)
}

// shouldWriteFile determines if a file should be written by comparing content hashes.
func (g *Generator) shouldWriteFile(filePath, newContent string) (bool, error) {
	// If file doesn't exist, we should write it
	existingContent, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read existing file %s: %w", filePath, err)
	}

	// Compare content hashes
	existingHash := computeContentHash(string(existingContent))
	newHash := computeContentHash(newContent)

	return existingHash != newHash, nil
}

// Helper methods

// findOutputConfig finds an output configuration by file path.
func (g *Generator) findOutputConfig(outputs []config.Output, outputFile string) *config.Output {
	for _, output := range outputs {
		if output.File == outputFile {
			return &output
		}
	}
	return nil
}

// renderTemplate renders a template for the given output configuration.
func (g *Generator) renderTemplate(output config.Output, data *templates.TemplateData) (string, error) {
	templateName := "default"
	if output.Template != "" {
		templateName = output.Template
	}

	content, err := g.renderer.Render(templateName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", templateName, err)
	}

	return content, nil
}

// writeOutputFile writes content to a file, creating directories as needed.
func (g *Generator) writeOutputFile(filePath, content string) error {
	// Ensure output directory exists
	outputDir := filepath.Dir(filePath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
		}
	}

	// Write the file
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write output file %s: %w", filePath, err)
	}

	return nil
}

// computeContentHash computes SHA256 hash of content.
func computeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// RegisterTemplate adds a custom template to the generator's renderer.
func (g *Generator) RegisterTemplate(name, templateStr string) error {
	return g.renderer.RegisterTemplate(name, templateStr)
}

// GetSupportedTemplates returns all available template names.
func (g *Generator) GetSupportedTemplates() []string {
	return g.renderer.GetSupportedFormats()
}

// ValidateTemplate checks if a template string is valid.
func (g *Generator) ValidateTemplate(templateStr string) error {
	return templates.ValidateTemplate(templateStr)
}

// PreviewOutput generates output content without writing to file.
func (g *Generator) PreviewOutput(cfg *config.Config, outputFile string) (string, error) {
	templateData := templates.NewTemplateData(cfg)

	// Find the output configuration
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return "", fmt.Errorf("output file %s not found in configuration", outputFile)
	}

	// Render and return content
	return g.renderTemplate(*targetOutput, templateData)
}