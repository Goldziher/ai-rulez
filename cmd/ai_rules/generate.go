package ai_rules

import (
	"fmt"

	"github.com/spf13/cobra"
)

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate AI rules files",
	Long: `Generate AI rules files from your configuration.
	
This command will read your ai_rules.yaml configuration file and generate
the appropriate rule files for different AI assistants (Claude, Cursor, Windsurf).`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("generate called")
		// TODO: Implement generation logic
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	generateCmd.Flags().
		StringP("format", "f", "", "Specify output format (claude, cursor, windsurf)")
	generateCmd.Flags().StringP("output", "o", "", "Output file path")
}
