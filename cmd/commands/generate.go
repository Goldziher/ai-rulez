package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRun          bool
	updateGitignore bool
	recursive       bool
)

// GenerateCmd represents the generate command
var GenerateCmd = &cobra.Command{
	Use:   "generate [config-file]",
	Short: "Generate AI assistant rule files from configuration",
	Long: `Generate AI assistant rule files based on the configuration.
This will create markdown files for various AI assistants like Claude, 
Cursor, Windsurf, etc. based on your configuration.`,
	Aliases: []string{"gen", "g"},
	Args:    cobra.MaximumNArgs(1),
	Run:     runGenerate,
}

func init() {
	GenerateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing files")
	GenerateCmd.Flags().BoolVar(&updateGitignore, "update-gitignore", false, "Update .gitignore files to include generated output files")
	GenerateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Find and process configuration files recursively")
}

func runGenerate(cmd *cobra.Command, args []string) {
	progress.SetQuiet(viper.GetBool("quiet"))

	if recursive {
		runRecursiveGenerate()
		return
	}

	// Single file generation
	var configPath string
	//nolint:gocritic // Simple if-else chain is more readable than switch
	if len(args) > 0 {
		configPath = args[0]
	} else if cfgFile != "" {
		configPath = cfgFile
	} else {
		// Auto-discover config file
		var err error
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
	}

	cfg, err := config.LoadConfigWithIncludes(context.Background(), configPath)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	gen := generator.NewWithConfigFile(configPath)

	if dryRun {
		preview, err := gen.PreviewAll(cfg)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		fmt.Println("Would generate the following files:")
		for output := range preview {
			fmt.Printf("  - %s\n", output)
		}
		return
	}

	if err := gen.GenerateAll(cfg); err != nil {
		fmtError(err)
		os.Exit(1)
	}

	if updateGitignore {
		if err := gitignore.UpdateGitignoreFiles(configPath, cfg); err != nil {
			if !progress.IsQuiet() {
				fmt.Printf("  ⚠️  Warning: Could not update .gitignore: %v\n", err)
			}
		}
	}

	progress.PrintIfNotQuiet("✅ Generated %d file(s) successfully\n", len(cfg.Outputs))
	for _, output := range cfg.Outputs {
		progress.PrintIfNotQuiet("  - %s\n", output.Path)
	}
}

func runRecursiveGenerate() {
	configFiles := findConfigFilesRecursively()
	if len(configFiles) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return
	}

	progress.PrintIfNotQuiet("Found %d configuration file(s)\n", len(configFiles))
	totalGenerated := processConfigFiles(configFiles)
	progress.PrintIfNotQuiet("\n✅ Total: Generated %d file(s) from %d config(s)\n", totalGenerated, len(configFiles))
}

func findConfigFilesRecursively() []string {
	var configFiles []string
	spinner := progress.NewSpinner("Searching for configuration files...")

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isConfigFile(filepath.Base(path)) {
			configFiles = append(configFiles, path)
			if err := spinner.Add(1); err != nil {
				logger.Debug("Failed to update spinner", "error", err)
			}
		}
		return nil
	})

	if err := spinner.Finish(); err != nil {
		logger.Debug("Failed to finish spinner", "error", err)
	}

	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	return configFiles
}

func isConfigFile(basename string) bool {
	return basename == "ai_rules.yaml" || basename == "ai_rulez.yaml" || basename == "ai-rules.yaml"
}

func processConfigFiles(configFiles []string) int {
	fileCounter := progress.NewFileCounter(len(configFiles), "Processing configurations")
	totalGenerated := 0

	for _, configPath := range configFiles {
		generated := processConfigFile(configPath, fileCounter)
		totalGenerated += generated
	}

	fileCounter.Finish()
	return totalGenerated
}

func processConfigFile(configPath string, fileCounter *progress.FileCounter) int {
	fileCounter.StartFile(configPath)

	cfg, err := config.LoadConfigWithIncludes(context.Background(), configPath)
	if err != nil {
		fileCounter.Error(err)
		return 0
	}

	gen := generator.NewWithConfigFile(configPath)

	if dryRun {
		return handleDryRun(gen, cfg, fileCounter)
	}

	return handleGeneration(gen, cfg, configPath, fileCounter)
}

func handleDryRun(gen *generator.Generator, cfg *config.Config, fileCounter *progress.FileCounter) int {
	preview, err := gen.PreviewAll(cfg)
	if err != nil {
		fileCounter.Error(err)
		return 0
	}

	progress.PrintlnIfNotQuiet("  Would generate:")
	for output := range preview {
		progress.PrintIfNotQuiet("    - %s\n", output)
	}
	fileCounter.FinishFile()
	return 0
}

func handleGeneration(gen *generator.Generator, cfg *config.Config, configPath string, fileCounter *progress.FileCounter) int {
	if err := gen.GenerateAll(cfg); err != nil {
		fileCounter.Error(err)
		return 0
	}

	handleGitignoreUpdate(configPath, cfg)
	fileCounter.FinishFile()
	return len(cfg.Outputs)
}

func handleGitignoreUpdate(configPath string, cfg *config.Config) {
	if updateGitignore {
		if err := gitignore.UpdateGitignoreFiles(configPath, cfg); err != nil {
			if !progress.IsQuiet() {
				fmt.Printf("  ⚠️  Warning: Could not update .gitignore: %v\n", err)
			}
		}
	}
}

// fmtError formats an error for display
func fmtError(err error) {
	if oopsErr, ok := oops.AsOops(err); ok {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if hint := oopsErr.Hint(); hint != "" {
			fmt.Fprintf(os.Stderr, "Hint: %s\n", hint)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
