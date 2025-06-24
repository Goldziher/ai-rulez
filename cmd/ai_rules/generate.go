package ai_rules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai_rules/internal/config"
	"github.com/Goldziher/ai_rules/internal/generator"
	"github.com/spf13/cobra"
)

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate [config-file]",
	Short: "Generate AI rules files",
	Long: `Generate AI rules files from your configuration.
	
This command will read your ai_rules.yaml configuration file and generate
the appropriate rule files for different AI assistants (Claude, Cursor, Windsurf).`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		configFile := "ai_rules.yaml"
		if len(args) > 0 {
			configFile = args[0]
		}

		// Check if config file exists
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: Configuration file '%s' not found\n", configFile)
			os.Exit(1)
		}

		// Load configuration with includes
		cfg, err := config.LoadConfigWithIncludes(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Validate configuration
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
			os.Exit(1)
		}

		// Generate output files
		configDir := filepath.Dir(configFile)
		if configDir == "" {
			configDir = "."
		}
		gen := generator.NewWithBaseDir(configDir)
		err = gen.GenerateAll(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating files: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Generated %d output file(s):\n", len(cfg.Outputs))
		for _, output := range cfg.Outputs {
			fmt.Printf("  %s\n", output.File)
		}
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
}
