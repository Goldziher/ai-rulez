package generator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
)

type Generator struct {
	renderer             *templates.Renderer
	baseDir              string
	configFile           string
	concurrencyThreshold int
}

func getConcurrencyThreshold() int {
	if threshold := os.Getenv("AI_RULEZ_CONCURRENCY_THRESHOLD"); threshold != "" {
		if val, err := strconv.Atoi(threshold); err == nil && val > 0 {
			return val
		}
	}
	return 10
}

func New() *Generator {
	return &Generator{
		renderer:             templates.NewRenderer(),
		baseDir:              ".",
		concurrencyThreshold: getConcurrencyThreshold(),
	}
}

func NewWithBaseDir(baseDir string) *Generator {
	return &Generator{
		renderer:             templates.NewRenderer(),
		baseDir:              baseDir,
		concurrencyThreshold: getConcurrencyThreshold(),
	}
}

func NewWithConfigFile(configFile string) *Generator {
	return &Generator{
		renderer:             templates.NewRenderer(),
		baseDir:              filepath.Dir(configFile),
		configFile:           filepath.Base(configFile),
		concurrencyThreshold: getConcurrencyThreshold(),
	}
}

func (g *Generator) GenerateAll(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return oops.
			With("field", "outputs").
			With("parent", "configuration").
			Hint("Add at least one output file in the configuration\nExample: outputs: [{path: 'CLAUDE.md', template: {type: 'builtin', value: 'default'}}]").
			Errorf("validation error: outputs is required in configuration")
	}

	if len(cfg.Outputs) >= g.concurrencyThreshold {
		return g.generateConcurrent(cfg)
	}

	for i := range cfg.Outputs {
		if err := g.generateOutput(cfg, &cfg.Outputs[i], i); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) generateConcurrent(cfg *config.Config) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Outputs))

	for i := range cfg.Outputs {
		wg.Add(1)
		go func(idx int, out *config.Output) {
			defer wg.Done()
			if err := g.generateOutput(cfg, out, idx); err != nil {
				errChan <- err
			}
		}(i, &cfg.Outputs[i])
	}

	wg.Wait()
	close(errChan)

	// Collect all errors instead of returning just the first one
	var allErrs []error
	for err := range errChan {
		if err != nil {
			allErrs = append(allErrs, err)
		}
	}

	// Return combined error if any occurred
	if len(allErrs) > 0 {
		return errors.Join(allErrs...)
	}
	return nil
}

func (g *Generator) generateOutput(cfg *config.Config, output *config.Output, index int) error {
	templateData := templates.NewTemplateDataForOutput(cfg, output.Path, nil)
	if err := g.writeOutputFile(output, templateData); err != nil {
		return oops.
			With("path", output.Path).
			With("output_index", index).
			With("template", output.Template).
			Hint(fmt.Sprintf("Check if the template '%v' is valid\nVerify the output file path is writable: %s", output.Template, output.Path)).
			Wrapf(err, "generate output file")
	}
	return nil
}

func (g *Generator) GenerateOutput(cfg *config.Config, outputFile string) error {
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return oops.
			With("path", outputFile).
			With("requested_file", outputFile).
			Hint(fmt.Sprintf("Check if '%s' is defined in the outputs section\nAvailable outputs: %v", outputFile, g.getOutputFileNames(cfg.Outputs))).
			Errorf("output file not found in configuration")
	}

	templateData := templates.NewTemplateDataForOutput(cfg, targetOutput.Path, nil)
	return g.writeOutputFile(targetOutput, templateData)
}

func (g *Generator) RegisterTemplate(name, templateStr string) error {
	if err := g.renderer.RegisterTemplate(name, templateStr); err != nil {
		return oops.
			With("template_name", name).
			With("template_type", "custom").
			Hint("Check the template syntax for errors").
			Wrapf(err, "template parse error")
	}
	return nil
}

func (g *Generator) GetSupportedTemplates() []string {
	return g.renderer.GetSupportedFormats()
}

func (*Generator) ValidateTemplate(templateStr string) error {
	if err := templates.ValidateTemplate(templateStr); err != nil {
		return oops.
			With("template_name", "validation").
			With("template_content", templateStr).
			Hint("Check the template syntax for errors").
			Wrapf(err, "template parse error")
	}
	return nil
}

func (g *Generator) PreviewOutput(cfg *config.Config, outputFile string) (string, error) {
	targetOutput := g.findOutputConfig(cfg.Outputs, outputFile)
	if targetOutput == nil {
		return "", oops.
			With("path", outputFile).
			With("requested_file", outputFile).
			Hint(fmt.Sprintf("Check if '%s' is defined in the outputs section", outputFile)).
			Errorf("output file not found in configuration")
	}

	templateData := templates.NewTemplateDataForOutput(cfg, targetOutput.Path, nil)
	templateData.ConfigFile = g.configFile
	templateData.OutputFile = targetOutput.Path

	content, err := g.renderTemplate(targetOutput, templateData)
	if err != nil {
		return "", err
	}

	header := templates.GenerateHeader(templateData)
	return header + content, nil
}

func (g *Generator) PreviewAll(cfg *config.Config) (map[string]string, error) {
	if len(cfg.Outputs) == 0 {
		return nil, oops.
			With("field", "outputs").
			With("parent", "configuration").
			Hint("Add at least one output file in the configuration").
			Errorf("validation error: outputs is required in configuration")
	}

	results := make(map[string]string)

	for i := range cfg.Outputs {
		output := &cfg.Outputs[i]
		templateData := templates.NewTemplateDataForOutput(cfg, output.Path, nil)
		templateData.ConfigFile = g.configFile
		templateData.OutputFile = output.Path

		content, err := g.renderTemplate(output, templateData)
		if err != nil {
			return nil, oops.
				With("path", output.Path).
				With("output_index", i).
				With("template", output.Template).
				Wrapf(err, "preview output")
		}

		header := templates.GenerateHeader(templateData)
		results[output.Path] = header + content
	}

	return results, nil
}
