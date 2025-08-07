// Package generator provides output file generation for ai_rules.
package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
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
		return errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the configuration").
			WithSuggestion("Example: outputs: [{file: 'CLAUDE.md', template: 'default'}]")
	}

	// Use concurrent generation for larger file sets
	if len(cfg.Outputs) >= 10 {
		return g.GenerateAllConcurrent(cfg)
	}

	// Serial generation for smaller file sets
	templateData := templates.NewTemplateData(cfg)

	for i := range cfg.Outputs {
		if err := g.writeOutputFile(&cfg.Outputs[i], templateData); err != nil {
			output := cfg.Outputs[i]
			return errors.New(errors.ErrorTypeGeneration, "generate output file", err).
				WithPath(output.GetPath()).
				WithContext("output_index", i).
				WithContext("template", output.Template).
				WithSuggestion("Check if the template '%s' is valid", output.Template).
				WithSuggestion("Verify the output file path is writable: %s", output.GetPath())
		}
	}

	return nil
}

// GenerateAllConcurrent generates all output files concurrently.
func (g *Generator) GenerateAllConcurrent(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the configuration")
	}

	templateData := templates.NewTemplateData(cfg)

	// Use a wait group and error channel for concurrent processing
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Outputs))

	for i := range cfg.Outputs {
		wg.Add(1)
		go func(idx int, out *config.Output) {
			defer wg.Done()
			if err := g.writeOutputFile(out, templateData); err != nil {
				errChan <- errors.New(errors.ErrorTypeGeneration, "generate output file", err).
					WithPath(out.GetPath()).
					WithContext("output_index", idx).
					WithContext("template", out.Template).
					WithContext("generation_mode", "concurrent")
			}
		}(i, &cfg.Outputs[i])
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err // Return first error
	}

	return nil
}

// GenerateOutput generates a single output file.
func (g *Generator) GenerateOutput(cfg *config.Config, outputFile string) error {
	templateData := templates.NewTemplateData(cfg)

	// Find the output configuration
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return errors.New(errors.ErrorTypeGenerationOutputNotFound, "find output config",
			fmt.Errorf("output file not found in configuration")).
			WithPath(outputFile).
			WithContext("requested_file", outputFile).
			WithSuggestion("Check if '%s' is defined in the outputs section", outputFile).
			WithSuggestion("Available outputs: %v", g.getOutputFileNames(cfg.Outputs))
	}

	return g.writeOutputFile(targetOutput, templateData)
}

// writeOutputFile writes output based on whether it's a file or directory.
func (g *Generator) writeOutputFile(output *config.Output, data *templates.TemplateData) error {
	if output.IsDirectory() {
		return g.writeDirectoryOutput(output, data)
	}
	return g.writeSingleFile(output, data)
}

// writeSingleFile writes a single output file.
func (g *Generator) writeSingleFile(output *config.Output, data *templates.TemplateData) error {
	// Set the file information for header generation
	data.ConfigFile = g.configFile
	data.OutputFile = output.GetPath()

	// Render the template
	content, err := g.renderTemplate(output, data)
	if err != nil {
		return err
	}

	// Prepend the header to the content
	header := templates.GenerateHeader(data)
	finalContent := header + content

	// Check if we need to write the file
	shouldWrite, err := g.shouldWriteFile(output.GetPath(), finalContent)
	if err != nil {
		return err
	}
	if !shouldWrite {
		return nil // File content is unchanged, skip writing
	}

	// Write the file
	return g.writeFile(output.GetPath(), finalContent)
}

// writeDirectoryOutput writes multiple files to a directory based on content type.
func (g *Generator) writeDirectoryOutput(output *config.Output, data *templates.TemplateData) error {
	outputType := output.GetOutputType()
	namingScheme := output.GetNamingScheme()
	dirPath := output.GetPath()

	// Ensure directory exists
	fullDirPath := filepath.Join(g.baseDir, dirPath)
	if err := os.MkdirAll(fullDirPath, 0o755); err != nil {
		return errors.FileWrite(fullDirPath, err).
			WithContext("operation", "create directory").
			WithSuggestion("Ensure you have write permissions for %s", fullDirPath)
	}

	switch outputType {
	case "agent":
		return g.writeAgentFiles(dirPath, namingScheme, output, data)
	case "rule":
		// For rules in a directory, write all rules to a single file
		return g.writeRulesFile(dirPath, namingScheme, output, data)
	default:
		return g.writeRulesFile(dirPath, namingScheme, output, data)
	}
}

// writeAgentFiles writes individual agent files to a directory.
func (g *Generator) writeAgentFiles(dirPath, namingScheme string, output *config.Output, data *templates.TemplateData) error {
	for i := range data.Agents {
		agent := &data.Agents[i]
		// Sanitize agent name for filename
		sanitizedName := sanitizeFilename(agent.Name)

		// Generate filename from naming scheme
		filename := namingScheme
		filename = strings.ReplaceAll(filename, "{name}", sanitizedName)
		filename = strings.ReplaceAll(filename, "{type}", "agent")
		filename = strings.ReplaceAll(filename, "{priority}", fmt.Sprintf("%d", agent.Priority))

		// Handle formatted index patterns like {index:03d}
		if strings.Contains(filename, "{index:") {
			// Extract format specifier
			start := strings.Index(filename, "{index:")
			end := strings.Index(filename[start:], "}") + start
			formatSpec := filename[start+7 : end]
			formatted := fmt.Sprintf("%0"+formatSpec, i+1)
			filename = filename[:start] + formatted + filename[end+1:]
		} else {
			filename = strings.ReplaceAll(filename, "{index}", fmt.Sprintf("%d", i+1))
		}

		// Handle formatted priority patterns like {priority:02d}
		if strings.Contains(filename, "{priority:") {
			start := strings.Index(filename, "{priority:")
			end := strings.Index(filename[start:], "}") + start
			formatSpec := filename[start+10 : end]
			formatted := fmt.Sprintf("%0"+formatSpec, agent.Priority)
			filename = filename[:start] + formatted + filename[end+1:]
		}

		filePath := filepath.Join(dirPath, filename)

		// Create agent-specific template data
		agentData := *data
		agentData.Agents = []config.Agent{*agent}
		agentData.OutputFile = filePath

		// Render agent template
		content, err := g.renderAgentTemplate(output, agent, &agentData)
		if err != nil {
			return err
		}

		// Write the file
		shouldWrite, err := g.shouldWriteFile(filePath, content)
		if err != nil {
			return err
		}
		if shouldWrite {
			if err := g.writeFile(filePath, content); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeRulesFile writes all rules to a single file in a directory.
func (g *Generator) writeRulesFile(dirPath, namingScheme string, output *config.Output, data *templates.TemplateData) error {
	// Generate filename from naming scheme
	// Use project name or "rules" as default name
	name := "rules"
	if data.ProjectName != "" {
		name = strings.ToLower(strings.ReplaceAll(data.ProjectName, " ", "-"))
	}

	filename := strings.ReplaceAll(namingScheme, "{name}", name)
	filename = strings.ReplaceAll(filename, "{type}", "rule")
	filename = strings.ReplaceAll(filename, "{index}", "1")

	// Handle formatted index patterns
	if strings.Contains(filename, "{index:") {
		start := strings.Index(filename, "{index:")
		end := strings.Index(filename[start:], "}") + start
		formatSpec := filename[start+7 : end]
		formatted := fmt.Sprintf("%0"+formatSpec, 1)
		filename = filename[:start] + formatted + filename[end+1:]
	}
	filePath := filepath.Join(dirPath, filename)

	// Update output path for single file write
	singleOutput := *output
	singleOutput.Path = filePath
	return g.writeSingleFile(&singleOutput, data)
}

// renderAgentTemplate renders a template specifically for an agent.
func (g *Generator) renderAgentTemplate(output *config.Output, agent *config.Agent, data *templates.TemplateData) (string, error) {
	// Create frontmatter data structure
	frontmatterData := map[string]interface{}{
		"name":        agent.Name,
		"description": agent.Description,
	}
	if len(agent.Tools) > 0 {
		frontmatterData["tools"] = agent.Tools
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(frontmatterData)
	if err != nil {
		return "", errors.New(errors.ErrorTypeGeneration, "marshal agent frontmatter", err).
			WithContext("agent_name", agent.Name)
	}

	frontmatter := "---\n" + string(yamlBytes) + "---\n\n"

	// Get the system prompt
	var systemPrompt string
	if agent.Template != "" {
		// Create a temporary output with the agent's template
		agentOutput := &config.Output{
			Template: agent.Template,
		}
		// Render template if specified
		renderedPrompt, err := g.renderTemplate(agentOutput, data)
		if err != nil {
			return "", err
		}
		systemPrompt = renderedPrompt
	} else {
		systemPrompt = agent.SystemPrompt
	}

	return frontmatter + systemPrompt, nil
}

// sanitizeFilename removes or replaces characters that are problematic in filenames
func sanitizeFilename(name string) string {
	// Replace path separators and other problematic characters
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, " ", "-")

	// Remove any remaining path elements
	name = filepath.Base(name)

	// Convert to lowercase for consistency
	return strings.ToLower(name)
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
		return false, errors.FileRead(fullPath, err).
			WithContext("operation", "check file status").
			WithSuggestion("Check if you have read permissions for the file")
	}

	// For small files (< 1MB), read into memory
	if stat.Size() < 1024*1024 {
		existingContent, err := os.ReadFile(fullPath)
		if err != nil {
			return false, errors.FileRead(fullPath, err).
				WithContext("operation", "read existing file for comparison")
		}
		existingHash := ComputeContentHashPooled(string(existingContent))
		newHash := ComputeContentHashPooled(newContent)
		return existingHash != newHash, nil
	}

	// For larger files, use streaming hash
	existingHash, err := computeFileHash(fullPath)
	if err != nil {
		return false, errors.FileRead(fullPath, err).
			WithContext("operation", "compute file hash").
			WithSuggestion("Check if the file is not locked by another process")
	}

	newHash := ComputeContentHashPooled(newContent)
	return existingHash != newHash, nil
}

// Helper methods

// findOutputConfig finds an output configuration by file path.
func (*Generator) findOutputConfig(outputs []config.Output, outputFile string) *config.Output {
	for _, output := range outputs {
		if output.GetPath() == outputFile {
			return &output
		}
	}
	return nil
}

// getOutputFileNames returns a list of all output file names.
func (*Generator) getOutputFileNames(outputs []config.Output) []string {
	names := make([]string, len(outputs))
	for i, output := range outputs {
		names[i] = output.GetPath()
	}
	return names
}

// renderTemplate renders a template for the given output configuration.
func (g *Generator) renderTemplate(output *config.Output, data *templates.TemplateData) (string, error) {
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
			return "", errors.TemplateNotFound(templatePath, g.GetSupportedTemplates()).
				WithPath(fullPath).
				WithContext("template_type", "file").
				WithSuggestion("Check if the template file exists: %s", fullPath).
				WithSuggestion("Verify the path is correct relative to %s", g.baseDir)
		}

		// Register and render the template
		templateID := fmt.Sprintf("file:%s", templatePath)
		if err := g.renderer.RegisterTemplate(templateID, string(templateContent)); err != nil {
			return "", errors.TemplateParse(templateID, err).
				WithPath(fullPath).
				WithContext("template_type", "file")
		}

		content, err := g.renderer.Render(templateID, data)
		if err != nil {
			return "", errors.TemplateExecute(templateID, err).
				WithPath(fullPath).
				WithContext("template_type", "file")
		}
		return content, nil
	}

	// Check if this is an inline template (contains newlines or template syntax)
	if strings.Contains(templateName, "\n") || strings.Contains(templateName, "{{") {
		// This is an inline template
		content, err := templates.RenderString(templateName, data)
		if err != nil {
			return "", errors.TemplateExecute("inline", err).
				WithContext("template_type", "inline").
				WithContext("template_content", templateName)
		}
		return content, nil
	}

	// Otherwise, treat as a named template
	content, err := g.renderer.Render(templateName, data)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", errors.TemplateNotFound(templateName, g.GetSupportedTemplates()).
				WithContext("template_type", "named")
		}
		return "", errors.TemplateExecute(templateName, err).
			WithContext("template_type", "named")
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
		return errors.FileWrite(outputDir, err).
			WithContext("operation", "create output directory").
			WithSuggestion("Check if you have write permissions for the parent directory")
	}

	// Write the file
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return errors.FileWrite(fullPath, err)
	}

	return nil
}

// computeFileHash computes SHA256 hash of a file without loading entire content into memory.
func computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Hash pool to reduce allocations
var hashPool = sync.Pool{
	New: func() any {
		return sha256.New()
	},
}

// ComputeContentHashPooled computes SHA256 hash using a pooled hasher.
func ComputeContentHashPooled(content string) string {
	h := hashPool.Get().(hash.Hash)
	defer func() {
		h.Reset()
		hashPool.Put(h)
	}()

	// Pre-allocate the result buffer
	result := make([]byte, 0, sha256.Size)
	_, _ = h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(result))
}

// RegisterTemplate adds a custom template to the generator's renderer.
func (g *Generator) RegisterTemplate(name, templateStr string) error {
	if err := g.renderer.RegisterTemplate(name, templateStr); err != nil {
		return errors.TemplateParse(name, err).
			WithContext("template_type", "custom").
			WithSuggestion("Check the template syntax for errors")
	}
	return nil
}

// GetSupportedTemplates returns all available template names.
func (g *Generator) GetSupportedTemplates() []string {
	return g.renderer.GetSupportedFormats()
}

// ValidateTemplate checks if a template string is valid.
func (*Generator) ValidateTemplate(templateStr string) error {
	if err := templates.ValidateTemplate(templateStr); err != nil {
		return errors.TemplateParse("validation", err).
			WithContext("template_content", templateStr).
			WithSuggestion("Check the template syntax for errors")
	}
	return nil
}

// PreviewOutput generates output content without writing to file.
func (g *Generator) PreviewOutput(cfg *config.Config, outputFile string) (string, error) {
	templateData := templates.NewTemplateData(cfg)

	// Find the output configuration
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return "", errors.New(errors.ErrorTypeGenerationOutputNotFound, "find output config",
			fmt.Errorf("output file not found in configuration")).
			WithPath(outputFile).
			WithContext("requested_file", outputFile).
			WithSuggestion("Check if '%s' is defined in the outputs section", outputFile)
	}

	// Set the file information for header generation
	templateData.ConfigFile = g.configFile
	templateData.OutputFile = targetOutput.GetPath()

	// Render the template
	content, err := g.renderTemplate(targetOutput, templateData)
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
		return nil, errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the configuration")
	}

	templateData := templates.NewTemplateData(cfg)
	results := make(map[string]string)

	for i := range cfg.Outputs {
		output := &cfg.Outputs[i]
		// Set the file information for header generation
		templateData.ConfigFile = g.configFile
		templateData.OutputFile = output.GetPath()

		content, err := g.renderTemplate(output, templateData)
		if err != nil {
			return nil, errors.New(errors.ErrorTypeGeneration, "preview output", err).
				WithPath(output.GetPath()).
				WithContext("output_index", i).
				WithContext("template", output.Template)
		}

		// Prepend the header
		header := templates.GenerateHeader(templateData)
		results[output.GetPath()] = header + content
	}

	return results, nil
}
