package ai_rules

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai_rules/internal/config"
	"github.com/spf13/cobra"
)

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate AI rules configuration",
	Long: `Validate your AI rules configuration file for syntax errors,
missing includes, and other potential issues.`,
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
			fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
			os.Exit(1)
		}

		// Validate configuration
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Configuration file '%s' is valid\n", configFile)
		fmt.Printf("  - Project: %s\n", cfg.Metadata.Name)
		if cfg.Metadata.Version != "" {
			fmt.Printf("  - Version: %s\n", cfg.Metadata.Version)
		}
		fmt.Printf("  - Rules: %d\n", len(cfg.Rules))
		if len(cfg.Sections) > 0 {
			fmt.Printf("  - Sections: %d\n", len(cfg.Sections))
		}
		fmt.Printf("  - Outputs: %d\n", len(cfg.Outputs))
		if len(cfg.Includes) > 0 {
			fmt.Printf("  - Includes: %d\n", len(cfg.Includes))
		}
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// validateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// validateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
