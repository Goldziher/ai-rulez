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
	"github.com/Goldziher/ai-rulez/internal/mcp/cli"
	"github.com/Goldziher/ai-rulez/internal/migration"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRun             bool
	updateGitignore    bool
	updateGitignoreSet bool
	recursive          bool
	skipCLIMCP         bool
)

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
	GenerateCmd.Flags().BoolVar(&skipCLIMCP, "no-configure-cli-mcp", false, "Skip configuring CLI-based MCP tools (claude, gemini, etc.)")
	GenerateCmd.Flags().BoolVar(&skipCLIMCP, "skip-cli-mcp", false, "Skip configuring CLI-based MCP tools (alias)")
}

func runGenerate(cmd *cobra.Command, args []string) {
	progress.SetQuiet(viper.GetBool("quiet"))

	updateGitignoreSet = cmd.Flags().Changed("update-gitignore")

	if recursive {
		runRecursiveGenerate()
		return
	}

	configPath := resolveConfigPath(args)
	handleMigration(configPath)
	cfg := loadConfig(configPath)

	gen := generator.NewWithConfigFile(configPath)

	if dryRun {
		showDryRunPreview(gen, cfg)
		return
	}

	generateFiles(gen, cfg, configPath)
}

func resolveConfigPath(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	if cfgFile != "" {
		return cfgFile
	}

	configPath, err := config.FindConfigFile(".")
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	return configPath
}

func handleMigration(configPath string) {
	version, err := migration.DetectAndMigrate(configPath)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	if version == "v1" && !progress.IsQuiet() {
		fmt.Println("Successfully migrated configuration from v1 to v2")
	}
}

func loadConfig(configPath string) *config.Config {
	cfg, err := config.LoadConfigWithIncludes(context.Background(), configPath)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	return cfg
}

func showDryRunPreview(gen *generator.Generator, cfg *config.Config) {
	preview, err := gen.PreviewAll(cfg)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	fmt.Println("Would generate the following files:")
	for output := range preview {
		fmt.Printf("  - %s\n", output)
	}
}

func generateFiles(gen *generator.Generator, cfg *config.Config, configPath string) {
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

	if !skipCLIMCP && len(cfg.MCPServers) > 0 {
		if err := cli.ConfigureCLIToolsWithOutputs(cfg.MCPServers, cfg.Outputs); err != nil {
			progress.PrintIfNotQuiet("⚠️  Warning: CLI MCP configuration failed: %v\n", err)
		}
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
		if !info.IsDir() && migration.IsConfigFile(filepath.Base(path)) {
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

	version, err := migration.DetectAndMigrate(configPath)
	if err != nil {
		fileCounter.Error(err)
		return 0
	}

	if version == "v1" {
		logger.Debug("Migrated config", "path", configPath, "from", "v1", "to", "v2")
	}

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

	if !skipCLIMCP && len(cfg.MCPServers) > 0 {
		if err := cli.ConfigureCLIToolsWithOutputs(cfg.MCPServers, cfg.Outputs); err != nil {
			logger.Debug("CLI MCP configuration failed", "error", err)
		}
	}

	fileCounter.FinishFile()
	return len(cfg.Outputs)
}

func handleGitignoreUpdate(configPath string, cfg *config.Config) {
	shouldUpdate := cfg.ShouldUpdateGitignore()
	if updateGitignoreSet {
		shouldUpdate = updateGitignore
	}

	if shouldUpdate {
		if err := gitignore.UpdateGitignoreFiles(configPath, cfg); err != nil {
			if !progress.IsQuiet() {
				fmt.Printf("  ⚠️  Warning: Could not update .gitignore: %v\n", err)
			}
		}
	}
}

func fmtError(err error) {
	if oopsErr, ok := oops.AsOops(err); ok {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		if errors, ok := oopsErr.Context()["errors"].([]string); ok && len(errors) > 0 {
			fmt.Fprintf(os.Stderr, "\nValidation errors:\n")
			for _, e := range errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
		}

		if hint := oopsErr.Hint(); hint != "" {
			fmt.Fprintf(os.Stderr, "\nHint: %s\n", hint)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
