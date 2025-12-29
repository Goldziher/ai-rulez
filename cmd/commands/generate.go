package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/migration"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRun          bool
	updateGitignore bool
	recursive       bool
	skipCLIMCP      bool
	profile         string
	autoMigrate     string // "true", "false", or "ask" (default)
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
	GenerateCmd.Flags().StringVar(&profile, "profile", "", "Profile to generate (V3 only, default: from config or 'default')")
	GenerateCmd.Flags().StringVar(&autoMigrate, "auto-migrate", "ask", "Auto-migrate V2 config: true (auto-migrate), false (skip), ask (prompt)")
}

func runGenerate(cmd *cobra.Command, args []string) {
	progress.SetQuiet(viper.GetBool("quiet"))

	workingDir := "."
	if len(args) > 0 {
		workingDir = filepath.Dir(args[0])
	}

	if recursive {
		runRecursiveGenerate()
		return
	}

	ctx := context.Background()

	// Check for V2 config and handle auto-migration
	configVersion, err := config.DetectConfigVersion(workingDir)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	// Handle V2 config migration
	if configVersion == "v2" {
		if shouldAutoMigrateV2(workingDir) {
			progress.PrintlnIfNotQuiet("Migrating V2 configuration to V3...")
			if err := performAutoMigration(ctx, workingDir); err != nil {
				fmtError(err)
				os.Exit(1)
			}
			progress.PrintlnIfNotQuiet("Migration completed successfully!")
		} else {
			logger.Error("V2 configuration detected", "hint", "Run 'ai-rulez migrate v3' to migrate to V3 first")
			os.Exit(1)
		}
	}

	// V3 - load and generate
	cfg, err := config.LoadConfigV3(ctx, workingDir)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.ValidateV3(); err != nil {
		fmtError(err)
		os.Exit(1)
	}

	// Create V3 generator
	gen := generator.NewGeneratorV3(cfg)

	if dryRun {
		progress.PrintlnIfNotQuiet("Note: --dry-run not yet supported for V3 configs")
		return
	}

	// Generate files
	if err := gen.Generate(profile); err != nil {
		fmtError(err)
		os.Exit(1)
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
		if !info.IsDir() && isV3ConfigFile(path) {
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

func isV3ConfigFile(path string) bool {
	// Check for V3 config structure: .ai-rulez directory with config.yaml, ai-rulez.yaml, or ai-rulez.json
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	return filepath.Base(dir) == ".ai-rulez" && (base == "config.yaml" || base == "ai-rulez.yaml" || base == "ai-rulez.json")
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
	// configPath is .ai-rulez/config.yaml, we need the parent directory
	workingDir := filepath.Dir(filepath.Dir(configPath))
	fileCounter.StartFile(configPath)

	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, workingDir)
	if err != nil {
		fileCounter.Error(err)
		return 0
	}

	// Validate configuration
	if err := cfg.ValidateV3(); err != nil {
		fileCounter.Error(err)
		return 0
	}

	// Create V3 generator
	gen := generator.NewGeneratorV3(cfg)

	if dryRun {
		progress.PrintlnIfNotQuiet("  Note: dry-run not yet supported for V3 configs")
		fileCounter.FinishFile()
		return 0
	}

	// Generate files
	if err := gen.Generate(profile); err != nil {
		fileCounter.Error(err)
		return 0
	}

	fileCounter.FinishFile()

	// Count generated files (estimate based on presets)
	return len(cfg.Presets) * 3 // Rough estimate
}

// shouldAutoMigrateV2 determines whether to auto-migrate a V2 config based on --auto-migrate flag
func shouldAutoMigrateV2(workingDir string) bool {
	const (
		valYes = "yes"
	)
	switch autoMigrate {
	case "true", valYes, "1":
		return true
	case "false", "no", "0":
		return false
	case "ask", "":
		// In interactive mode, prompt the user
		return progress.PromptYesNo("V2 config detected. Migrate to V3?", true)
	default:
		// Invalid value, default to prompting
		logger.Warn("Invalid auto-migrate value, treating as 'ask'", "value", autoMigrate)
		return progress.PromptYesNo("V2 config detected. Migrate to V3?", true)
	}
}

// performAutoMigration performs the V2 to V3 migration during generate
func performAutoMigration(ctx context.Context, workingDir string) error {
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return oops.
			With("path", workingDir).
			Wrapf(err, "resolve working directory path")
	}

	// Find V2 config file
	v2ConfigPath := detectV2Config()
	if v2ConfigPath == "" {
		return oops.
			Errorf("no V2 config file found (expected ai-rulez.yaml or ai-rulez.yml)")
	}

	absV2ConfigPath, err := filepath.Abs(v2ConfigPath)
	if err != nil {
		return oops.
			With("path", v2ConfigPath).
			Wrapf(err, "resolve V2 config path")
	}

	// Create migrator
	migrator := &migration.V2ToV3Migrator{}
	// Initialize migrator with correct package path
	*migrator = *migration.NewV2ToV3Migrator(absV2ConfigPath, absWorkingDir)

	// Perform migration
	if err := migrator.Migrate(ctx); err != nil {
		return oops.
			With("v2_config", absV2ConfigPath).
			Wrapf(err, "migrate V2 to V3")
	}

	// Delete backup directory if it exists (migration was successful)
	DeleteBackupDirectory(absWorkingDir)

	return nil
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
