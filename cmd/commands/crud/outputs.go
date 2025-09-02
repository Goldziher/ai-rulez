package crud

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/spf13/cobra"
)

var (
	outputTemplate string
	outputType     string
)

// AddOutputCmd adds a new output to the configuration
var AddOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Add a new output file to configuration",
	Long: `Add a new output file to your AI rules configuration.
The filename is provided as an argument, and you can optionally specify
a template to use for rendering the output.`,
	Args: cobra.ExactArgs(1),
	Run:  runAddOutput,
}

// UpdateOutputCmd updates an existing output
var UpdateOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Update an existing output",
	Long: `Update an existing output in your AI rules configuration file.
You can update the template or other properties.`,
	Args: cobra.ExactArgs(1),
	Run:  runUpdateOutput,
}

// DeleteOutputCmd deletes an output
var DeleteOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Delete an output from configuration",
	Long:  `Delete an output by filename from your AI rules configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runDeleteOutput,
}

func init() {
	AddOutputCmd.Flags().StringVar(&outputTemplate, "template", "", "Template to use for rendering")
	AddOutputCmd.Flags().StringVar(&outputType, "type", "", "Type of output (rule, section, both)")

	UpdateOutputCmd.Flags().StringVar(&outputTemplate, "template", "", "New template for the output")
	UpdateOutputCmd.Flags().StringVar(&outputType, "type", "", "New type for the output")
}

func runAddOutput(cmd *cobra.Command, args []string) {
	filename := args[0]
	configPath, cfg := loadConfiguration()

	for _, output := range cfg.Outputs {
		if output.Path == filename {
			fmt.Fprintf(os.Stderr, "Error: Output file '%s' already exists in configuration\n", filename)
			os.Exit(1)
		}
	}

	newOutput := config.Output{
		Path:     filename,
		Template: outputTemplate,
		Type:     outputType,
	}
	cfg.Outputs = append(cfg.Outputs, newOutput)

	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Added output '%s'", filename)
	if outputTemplate != "" {
		fmt.Printf(" with template '%s'", outputTemplate)
	}
	fmt.Printf(" to %s\n", configPath)
}

func runUpdateOutput(cmd *cobra.Command, args []string) {
	filename := args[0]
	configPath, cfg := loadConfiguration()

	outputIndex := -1
	for i, output := range cfg.Outputs {
		if output.Path == filename {
			outputIndex = i
			break
		}
	}

	if outputIndex == -1 {
		fmt.Fprintf(os.Stderr, "Error: Output '%s' not found\n", filename)
		os.Exit(1)
	}

	if outputTemplate != "" {
		cfg.Outputs[outputIndex].Template = outputTemplate
	}
	if outputType != "" {
		cfg.Outputs[outputIndex].Type = outputType
	}

	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Updated output '%s' in %s\n", filename, configPath)
}

func runDeleteOutput(cmd *cobra.Command, args []string) {
	filename := args[0]
	configPath, cfg := loadConfiguration()

	outputIndex := -1
	for i, output := range cfg.Outputs {
		if output.Path == filename {
			outputIndex = i
			break
		}
	}

	if outputIndex == -1 {
		fmt.Fprintf(os.Stderr, "Error: Output '%s' not found\n", filename)
		os.Exit(1)
	}

	cfg.Outputs = append(cfg.Outputs[:outputIndex], cfg.Outputs[outputIndex+1:]...)

	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Deleted output '%s' from %s\n", filename, configPath)
}
