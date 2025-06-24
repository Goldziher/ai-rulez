package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/airules/internal/config"
	"github.com/Goldziher/airules/internal/generator"
	"github.com/spf13/cobra"
)

var (
	recursive bool
	dryRun    bool
)

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate [config-file]",
	Short: "Generate AI rules files",
	Long: `Generate AI rules files from your configuration.
	
This command will search for configuration files starting from the current
directory and traversing up. Supported file names:
  - .airules.yaml, .airules.yml
  - airules.yaml, airules.yml
  - .ai_rules.yaml, .ai_rules.yml
  - ai_rules.yaml, ai_rules.yml

With the -r/--recursive flag, it will find and process all configuration
files in the current directory tree.

With the --dry-run flag, it will validate the configuration and show what would
be generated without writing any files.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		if recursive {
			runRecursiveGenerate()
			return
		}

		var configFile string
		if len(args) > 0 {
			configFile = args[0]
			// Check if specified config file exists
			if _, err := os.Stat(configFile); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Error: Configuration file '%s' not found\n", configFile)
				os.Exit(1)
			}
		} else {
			// Find config file by traversing up
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to get current directory: %v\n", err)
				os.Exit(1)
			}

			configFile, err = config.FindConfigFile(cwd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				fmt.Fprintln(os.Stderr, "\nTo create a new configuration file, run: airules init")
				os.Exit(1)
			}

			// Show which config file we found
			relPath, _ := filepath.Rel(cwd, configFile)
			if relPath == "" {
				relPath = configFile
			}
			fmt.Printf("Using config: %s\n", relPath)
		}

		// Process single config file
		processConfigFile(configFile)
	},
}

func processConfigFile(configFile string) {
	// Load the configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config %s: %v\n", configFile, err)
		os.Exit(1)
	}

	// Create generator with the config's base directory
	gen := generator.NewWithBaseDir(filepath.Dir(configFile))

	if dryRun {
		// Dry run mode: validate and preview without writing
		results, err := gen.PreviewAll(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating outputs: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Configuration valid. Would generate %d file(s):\n", len(results))
		for filePath := range results {
			fullPath := filepath.Join(filepath.Dir(configFile), filePath)
			fmt.Printf("  • %s\n", fullPath)
		}
	} else {
		// Normal mode: generate all outputs
		err = gen.GenerateAll(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating outputs: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Generated %d output file(s)\n", len(cfg.Outputs))
	}
}

func runRecursiveGenerate() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	// Find all config files recursively
	configs, err := config.FindAllConfigFiles(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nNo configuration files found in the current directory tree.")
		os.Exit(1)
	}

	// Sort configs for consistent output
	sort.Strings(configs)

	if dryRun {
		fmt.Printf("Found %d configuration file(s) for validation:\n", len(configs))
	} else {
		fmt.Printf("Found %d configuration file(s):\n", len(configs))
	}

	for _, cfg := range configs {
		relPath, _ := filepath.Rel(cwd, cfg)
		if relPath == "" {
			relPath = cfg
		}
		if dryRun {
			fmt.Printf("\n• Validating: %s\n", relPath)
		} else {
			fmt.Printf("\n• Processing: %s\n", relPath)
		}
		processConfigFile(cfg)
	}
}

func init() {
	rootCmd.AddCommand(generateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	generateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively find and process all configuration files")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration and show what would be generated without writing files")
}
