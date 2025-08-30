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

type Generator struct {
	renderer   *templates.Renderer
	baseDir    string
	configFile string
}

func New() *Generator {
	return &Generator{
		renderer: templates.NewRenderer(),
		baseDir:  ".",
	}
}

func NewWithBaseDir(baseDir string) *Generator {
	return &Generator{
		renderer: templates.NewRenderer(),
		baseDir:  baseDir,
	}
}

func NewWithConfigFile(configFile string) *Generator {
	return &Generator{
		renderer:   templates.NewRenderer(),
		baseDir:    filepath.Dir(configFile),
		configFile: filepath.Base(configFile),
	}
}

func NewWithRenderer(renderer *templates.Renderer) *Generator {
	return &Generator{
		renderer: renderer,
		baseDir:  ".",
	}
}

func (g *Generator) GenerateAll(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the configuration").
			WithSuggestion("Example: outputs: [{file: 'CLAUDE.md', template: 'default'}]")
	}

	if len(cfg.Outputs) >= 10 {
		return g.GenerateAllConcurrent(cfg)
	}

	for i := range cfg.Outputs {
		templateData := templates.NewTemplateDataForOutput(cfg, cfg.Outputs[i].GetPath())
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

func (g *Generator) GenerateAllConcurrent(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the configuration")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Outputs))

	for i := range cfg.Outputs {
		wg.Add(1)
		go func(idx int, out *config.Output) {
			defer wg.Done()
			templateData := templates.NewTemplateDataForOutput(cfg, out.GetPath())
			if err := g.writeOutputFile(out, templateData); err != nil {
				errChan <- errors.New(errors.ErrorTypeGeneration, "generate output file", err).
					WithPath(out.GetPath()).
					WithContext("output_index", idx).
					WithContext("template", out.Template).
					WithContext("generation_mode", "concurrent")
			}
		}(i, &cfg.Outputs[i])
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}

	return nil
}

func (g *Generator) GenerateOutput(cfg *config.Config, outputFile string) error {
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return errors.New(errors.ErrorTypeGenerationOutputNotFound, "find output config",
			fmt.Errorf("output file not found in configuration")).
			WithPath(outputFile).
			WithContext("requested_file", outputFile).
			WithSuggestion("Check if '%s' is defined in the outputs section", outputFile).
			WithSuggestion("Available outputs: %v", g.getOutputFileNames(cfg.Outputs))
	}

	templateData := templates.NewTemplateDataForOutput(cfg, targetOutput.GetPath())
	return g.writeOutputFile(targetOutput, templateData)
}

func (g *Generator) writeOutputFile(output *config.Output, data *templates.TemplateData) error {
	if output.IsDirectory() {
		return g.writeDirectoryOutput(output, data)
	}
	return g.writeSingleFile(output, data)
}

func (g *Generator) writeSingleFile(output *config.Output, data *templates.TemplateData) error {
	localData := *data
	localData.ConfigFile = g.configFile
	localData.OutputFile = output.GetPath()
	data = &localData

	content, err := g.renderTemplate(output, data)
	if err != nil {
		return err
	}

	header := templates.GenerateHeader(data)
	finalContent := header + content

	shouldWrite, err := g.shouldWriteFile(output.GetPath(), finalContent)
	if err != nil {
		return err
	}
	if !shouldWrite {
		return nil
	}

	return g.writeFile(output.GetPath(), finalContent)
}

func (g *Generator) writeDirectoryOutput(output *config.Output, data *templates.TemplateData) error {
	outputType := output.GetOutputType()
	namingScheme := output.GetNamingScheme()
	dirPath := output.GetPath()

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
		return g.writeRulesFile(dirPath, namingScheme, output, data)
	default:
		return g.writeRulesFile(dirPath, namingScheme, output, data)
	}
}

func (g *Generator) writeAgentFiles(dirPath, namingScheme string, output *config.Output, data *templates.TemplateData) error {
	for i := range data.Agents {
		agent := &data.Agents[i]
		sanitizedName := sanitizeFilename(agent.Name)

		filename := namingScheme
		filename = strings.ReplaceAll(filename, "{name}", sanitizedName)
		filename = strings.ReplaceAll(filename, "{type}", "agent")
		filename = strings.ReplaceAll(filename, "{priority}", fmt.Sprintf("%d", agent.Priority))

		if strings.Contains(filename, "{index:") {
			start := strings.Index(filename, "{index:")
			end := strings.Index(filename[start:], "}") + start
			formatSpec := filename[start+7 : end]
			formatted := fmt.Sprintf("%0"+formatSpec, i+1)
			filename = filename[:start] + formatted + filename[end+1:]
		} else {
			filename = strings.ReplaceAll(filename, "{index}", fmt.Sprintf("%d", i+1))
		}

		if strings.Contains(filename, "{priority:") {
			start := strings.Index(filename, "{priority:")
			end := strings.Index(filename[start:], "}") + start
			formatSpec := filename[start+10 : end]
			formatted := fmt.Sprintf("%0"+formatSpec, agent.Priority)
			filename = filename[:start] + formatted + filename[end+1:]
		}

		filePath := filepath.Join(dirPath, filename)

		agentData := *data
		agentData.Agents = []config.Agent{*agent}
		agentData.OutputFile = filePath

		content, err := g.renderAgentTemplate(output, agent, &agentData)
		if err != nil {
			return err
		}

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

func (g *Generator) writeRulesFile(dirPath, namingScheme string, output *config.Output, data *templates.TemplateData) error {
	name := "rules"
	if data.ProjectName != "" {
		name = strings.ToLower(strings.ReplaceAll(data.ProjectName, " ", "-"))
	}

	filename := strings.ReplaceAll(namingScheme, "{name}", name)
	filename = strings.ReplaceAll(filename, "{type}", "rule")
	filename = strings.ReplaceAll(filename, "{index}", "1")

	if strings.Contains(filename, "{index:") {
		start := strings.Index(filename, "{index:")
		end := strings.Index(filename[start:], "}") + start
		formatSpec := filename[start+7 : end]
		formatted := fmt.Sprintf("%0"+formatSpec, 1)
		filename = filename[:start] + formatted + filename[end+1:]
	}
	filePath := filepath.Join(dirPath, filename)

	singleOutput := *output
	singleOutput.Path = filePath
	return g.writeSingleFile(&singleOutput, data)
}

func (g *Generator) renderAgentTemplate(output *config.Output, agent *config.Agent, data *templates.TemplateData) (string, error) {
	frontmatterData := map[string]interface{}{
		"name":        agent.Name,
		"description": agent.Description,
	}
	if len(agent.Tools) > 0 {
		frontmatterData["tools"] = agent.Tools
	}

	yamlBytes, err := yaml.Marshal(frontmatterData)
	if err != nil {
		return "", errors.New(errors.ErrorTypeGeneration, "marshal agent frontmatter", err).
			WithContext("agent_name", agent.Name)
	}

	frontmatter := "---\n" + string(yamlBytes) + "---\n\n"

	var systemPrompt string
	if agent.Template != "" {
		agentOutput := &config.Output{
			Template: agent.Template,
		}
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

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, " ", "-")

	name = filepath.Base(name)

	return strings.ToLower(name)
}

func (g *Generator) shouldWriteFile(filePath, newContent string) (bool, error) {
	fullPath := filepath.Join(g.baseDir, filePath)

	stat, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, errors.FileRead(fullPath, err).
			WithContext("operation", "check file status").
			WithSuggestion("Check if you have read permissions for the file")
	}

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

	existingHash, err := computeFileHash(fullPath)
	if err != nil {
		return false, errors.FileRead(fullPath, err).
			WithContext("operation", "compute file hash").
			WithSuggestion("Check if the file is not locked by another process")
	}

	newHash := ComputeContentHashPooled(newContent)
	return existingHash != newHash, nil
}

func (*Generator) findOutputConfig(outputs []config.Output, outputFile string) *config.Output {
	for _, output := range outputs {
		if output.GetPath() == outputFile {
			return &output
		}
	}
	return nil
}

func (*Generator) getOutputFileNames(outputs []config.Output) []string {
	names := make([]string, len(outputs))
	for i, output := range outputs {
		names[i] = output.GetPath()
	}
	return names
}

func (g *Generator) renderTemplate(output *config.Output, data *templates.TemplateData) (string, error) {
	templateName := "default"
	if output.Template != "" {
		templateName = output.Template
	}

	if strings.HasPrefix(templateName, "@") {
		templatePath := strings.TrimPrefix(templateName, "@")
		fullPath := filepath.Join(g.baseDir, templatePath)

		templateContent, err := os.ReadFile(fullPath)
		if err != nil {
			return "", errors.TemplateNotFound(templatePath, g.GetSupportedTemplates()).
				WithPath(fullPath).
				WithContext("template_type", "file").
				WithSuggestion("Check if the template file exists: %s", fullPath).
				WithSuggestion("Verify the path is correct relative to %s", g.baseDir)
		}

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

	if strings.Contains(templateName, "\n") || strings.Contains(templateName, "{{") {
		content, err := templates.RenderString(templateName, data)
		if err != nil {
			return "", errors.TemplateExecute("inline", err).
				WithContext("template_type", "inline").
				WithContext("template_content", templateName)
		}
		return content, nil
	}

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

func (g *Generator) writeFile(filePath, content string) error {
	fullPath := filepath.Join(g.baseDir, filePath)

	outputDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return errors.FileWrite(outputDir, err).
			WithContext("operation", "create output directory").
			WithSuggestion("Check if you have write permissions for the parent directory")
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return errors.FileWrite(fullPath, err)
	}

	return nil
}

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

var hashPool = sync.Pool{
	New: func() any {
		return sha256.New()
	},
}

func ComputeContentHashPooled(content string) string {
	h := hashPool.Get().(hash.Hash)
	defer func() {
		h.Reset()
		hashPool.Put(h)
	}()

	result := make([]byte, 0, sha256.Size)
	_, _ = h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(result))
}

func (g *Generator) RegisterTemplate(name, templateStr string) error {
	if err := g.renderer.RegisterTemplate(name, templateStr); err != nil {
		return errors.TemplateParse(name, err).
			WithContext("template_type", "custom").
			WithSuggestion("Check the template syntax for errors")
	}
	return nil
}

func (g *Generator) GetSupportedTemplates() []string {
	return g.renderer.GetSupportedFormats()
}

func (*Generator) ValidateTemplate(templateStr string) error {
	if err := templates.ValidateTemplate(templateStr); err != nil {
		return errors.TemplateParse("validation", err).
			WithContext("template_content", templateStr).
			WithSuggestion("Check the template syntax for errors")
	}
	return nil
}

func (g *Generator) PreviewOutput(cfg *config.Config, outputFile string) (string, error) {
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return "", errors.New(errors.ErrorTypeGenerationOutputNotFound, "find output config",
			fmt.Errorf("output file not found in configuration")).
			WithPath(outputFile).
			WithContext("requested_file", outputFile).
			WithSuggestion("Check if '%s' is defined in the outputs section", outputFile)
	}

	templateData := templates.NewTemplateDataForOutput(cfg, targetOutput.GetPath())
	templateData.ConfigFile = g.configFile
	templateData.OutputFile = targetOutput.GetPath()

	content, err := g.renderTemplate(targetOutput, templateData)
	if err != nil {
		return "", err
	}

	header := templates.GenerateHeader(templateData)
	return header + content, nil
}

func (g *Generator) PreviewAll(cfg *config.Config) (map[string]string, error) {
	if len(cfg.Outputs) == 0 {
		return nil, errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the configuration")
	}

	results := make(map[string]string)

	for i := range cfg.Outputs {
		output := &cfg.Outputs[i]
		templateData := templates.NewTemplateDataForOutput(cfg, output.GetPath())
		templateData.ConfigFile = g.configFile
		templateData.OutputFile = output.GetPath()

		content, err := g.renderTemplate(output, templateData)
		if err != nil {
			return nil, errors.New(errors.ErrorTypeGeneration, "preview output", err).
				WithPath(output.GetPath()).
				WithContext("output_index", i).
				WithContext("template", output.Template)
		}

		header := templates.GenerateHeader(templateData)
		results[output.GetPath()] = header + content
	}

	return results, nil
}
