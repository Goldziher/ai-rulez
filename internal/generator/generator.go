// Package generator provides output file generation for ai_rules.
package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// Generator handles the generation of output files from configuration.
type Generator struct {
	renderer   *templates.Renderer
	baseDir    string
	configFile string // Source configuration file name
}

// New creates a new generator with the default template renderer.
func New() *Generator {
	return &Generator{
		renderer: templates.NewRenderer(),
		baseDir:  ".",
	}
}

// NewWithBaseDir creates a new generator with a specific base directory.
func NewWithBaseDir(baseDir string) *Generator {
	return &Generator{
		renderer: templates.NewRenderer(),
		baseDir:  baseDir,
	}
}

// NewWithConfigFile creates a new generator with a specific config file.
func NewWithConfigFile(configFile string) *Generator {
	return &Generator{
		renderer:   templates.NewRenderer(),
		baseDir:    filepath.Dir(configFile),
		configFile: filepath.Base(configFile),
	}
}

// NewWithRenderer creates a generator with a custom renderer.
func NewWithRenderer(renderer *templates.Renderer) *Generator {
	return &Generator{
		renderer: renderer,
		baseDir:  ".",
	}
}

// GenerateAll generates all output files defined in the configuration.
func (g *Generator) GenerateAll(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return errors.New("no outputs defined in configuration")
	}

	// Use concurrent generation for larger file sets
	if len(cfg.Outputs) >= 10 {
		return g.GenerateAllConcurrent(cfg)
	}

	// Serial generation for smaller file sets
	templateData := templates.NewTemplateData(cfg)

	for i, output := range cfg.Outputs {
		if err := g.writeOutputFile(output, templateData); err != nil {
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

	return g.writeOutputFile(*targetOutput, templateData)
}

// writeOutputFile writes a single output file.
func (g *Generator) writeOutputFile(output config.Output, data *templates.TemplateData) error {
	// Set the file information for header generation
	data.ConfigFile = g.configFile
	data.OutputFile = output.File

	// Render the template
	content, err := g.renderTemplate(output, data)
	if err != nil {
		return err
	}

	// Prepend the header to the content
	header := templates.GenerateHeader(data)
	finalContent := header + content

	// Check if we need to write the file
	shouldWrite, err := g.shouldWriteFile(output.File, finalContent)
	if err != nil {
		return fmt.Errorf("failed to check if file should be written: %w", err)
	}
	if !shouldWrite {
		return nil // File content is unchanged, skip writing
	}

	// Write the file
	return g.writeFile(output.File, finalContent)
}

// shouldWriteFile determines if a file should be written by comparing content hashes.
func (g *Generator) shouldWriteFile(filePath, newContent string) (bool, error) {
	// Resolve the full path relative to base directory
	fullPath := filepath.Join(g.baseDir, filePath)

	// If file doesn't exist, we should write it
	stat, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat file %s: %w", fullPath, err)
	}

	// For small files (< 1MB), read into memory
	if stat.Size() < 1024*1024 {
		existingContent, err := os.ReadFile(fullPath)
		if err != nil {
			return false, fmt.Errorf("failed to read existing file %s: %w", fullPath, err)
		}
		existingHash := computeContentHash(string(existingContent))
		newHash := computeContentHash(newContent)
		return existingHash != newHash, nil
	}

	// For larger files, use streaming hash
	existingHash, err := computeFileHashStreaming(fullPath)
	if err != nil {
		return false, fmt.Errorf("failed to compute hash for %s: %w", fullPath, err)
	}

	newHash := computeContentHash(newContent)
	return existingHash != newHash, nil
}

// Helper methods

// findOutputConfig finds an output configuration by file path.
func (*Generator) findOutputConfig(outputs []config.Output, outputFile string) *config.Output {
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

	// Check if this is a file reference (starts with @)
	if strings.HasPrefix(templateName, "@") {
		templatePath := strings.TrimPrefix(templateName, "@")
		// Resolve the template path relative to base directory
		fullPath := filepath.Join(g.baseDir, templatePath)

		// Read the template file
		templateContent, err := os.ReadFile(fullPath)
		if err != nil {
			return "", fmt.Errorf("failed to read template file %s: %w", fullPath, err)
		}

		// Register and render the template
		templateID := fmt.Sprintf("file:%s", templatePath)
		if err := g.renderer.RegisterTemplate(templateID, string(templateContent)); err != nil {
			return "", fmt.Errorf("failed to register template from %s: %w", templatePath, err)
		}

		return g.renderer.Render(templateID, data)
	}

	// Check if this is an inline template (contains newlines or template syntax)
	if strings.Contains(templateName, "\n") || strings.Contains(templateName, "{{") {
		// This is an inline template
		return templates.RenderString(templateName, data)
	}

	// Otherwise, treat as a named template
	content, err := g.renderer.Render(templateName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", templateName, err)
	}

	return content, nil
}

// writeFile writes content to a file, creating directories as needed.
func (g *Generator) writeFile(filePath, content string) error {
	// Resolve the full path relative to base directory
	fullPath := filepath.Join(g.baseDir, filePath)

	// Ensure output directory exists
	outputDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	// Write the file
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write output file %s: %w", fullPath, err)
	}

	return nil
}

// computeContentHash computes SHA256 hash of content.
// Consider using ComputeContentHashPooled for better performance.
func computeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// computeFileHashStreaming computes SHA256 hash of a file without loading entire content into memory.
func computeFileHashStreaming(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
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
func (*Generator) ValidateTemplate(templateStr string) error {
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

	// Set the file information for header generation
	templateData.ConfigFile = g.configFile
	templateData.OutputFile = targetOutput.File

	// Render the template
	content, err := g.renderTemplate(*targetOutput, templateData)
	if err != nil {
		return "", err
	}

	// Prepend the header and return
	header := templates.GenerateHeader(templateData)
	return header + content, nil
}

// PreviewAll generates all output content without writing files.
// Returns a map of file paths to their generated content.
func (g *Generator) PreviewAll(cfg *config.Config) (map[string]string, error) {
	if len(cfg.Outputs) == 0 {
		return nil, errors.New("no outputs defined in configuration")
	}

	templateData := templates.NewTemplateData(cfg)
	results := make(map[string]string)

	for i, output := range cfg.Outputs {
		// Set the file information for header generation
		templateData.ConfigFile = g.configFile
		templateData.OutputFile = output.File

		content, err := g.renderTemplate(output, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to generate output %d (%s): %w", i, output.File, err)
		}

		// Prepend the header
		header := templates.GenerateHeader(templateData)
		results[output.File] = header + content
	}

	return results, nil
}
