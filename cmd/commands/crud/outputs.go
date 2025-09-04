package crud

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var (
	outputType          string
	outputNamingScheme  string
	outputTemplateType  string
	outputTemplateValue string
)

var AddOutputCmd = &cobra.Command{
	Use:   "output [path]",
	Short: "Add a new output to the configuration",
	Long:  `Adds a new output to the outputs list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		newOutput := &config.Output{
			Path:         path,
			Type:         outputType,
			NamingScheme: outputNamingScheme,
			Template:     crud.CreateTemplateConfig(outputTemplateType, outputTemplateValue),
		}

		crud.AddElement("outputs", newOutput)
	},
}

var UpdateOutputCmd = &cobra.Command{
	Use:   "output [path]",
	Short: "Update an existing output",
	Long:  `Updates an existing output in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("type") {
			updates["Type"] = outputType
		}
		if cmd.Flags().Changed("naming-scheme") {
			updates["NamingScheme"] = outputNamingScheme
		}
		if cmd.Flags().Changed("template-type") || cmd.Flags().Changed("template-value") {
			updates["Template"] = crud.CreateTemplateConfig(outputTemplateType, outputTemplateValue)
		}

		crud.UpdateElement("outputs", path, updates)
	},
}

var DeleteOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Delete an output from configuration",
	Long:  `Deletes an output by filename from your AI rules configuration.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		crud.DeleteElement("outputs", path)
	},
}

var GetOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Get an output from configuration",
	Long:  `Retrieves an output by filename from your AI rules configuration.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		crud.GetElement("outputs", path)
	},
}

var ListOutputsCmd = &cobra.Command{
	Use:     "outputs",
	Short:   "List all outputs in the configuration",
	Long:    `Lists all outputs defined in your AI rules configuration.`,
	Aliases: []string{"output"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("outputs")
	},
}

func init() {
	AddOutputCmd.Flags().StringVar(&outputType, "type", "", "Type of output (rule or agent)")
	AddOutputCmd.Flags().StringVar(&outputNamingScheme, "naming-scheme", "", "Naming pattern for files when path is a directory (e.g., '{name}.md')")
	AddOutputCmd.Flags().StringVar(&outputTemplateType, "template-type", "", "Template type: builtin, file, or inline")
	AddOutputCmd.Flags().StringVar(&outputTemplateValue, "template-value", "", "Template value (name, path, or content)")

	UpdateOutputCmd.Flags().StringVar(&outputType, "type", "", "New type for the output")
	UpdateOutputCmd.Flags().StringVar(&outputNamingScheme, "naming-scheme", "", "New naming pattern for the output")
	UpdateOutputCmd.Flags().StringVar(&outputTemplateType, "template-type", "", "New template type for the output")
	UpdateOutputCmd.Flags().StringVar(&outputTemplateValue, "template-value", "", "New template value for the output")
}
