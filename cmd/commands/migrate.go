package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/migration"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	migrateOutput   string
	migrateValidate bool
	migrateDryRun   bool
	migrateForce    bool
)

var MigrateCmd = &cobra.Command{
	Use:   "migrate <version> [config-file]",
	Short: "Migrate configuration between versions",
	Long: `Migrate AI-Rulez configuration between different versions.

Supported migrations:
  v2 → v3: Convert ai-rulez.yaml to .ai-rulez/ directory structure

Examples:
  ai-rulez migrate v3 ai-rulez.yaml
  ai-rulez migrate v3 ai-rulez.yaml --output .ai-rulez
  ai-rulez migrate v3 --validate  # Validate migration without writing files
  ai-rulez migrate v3 --dry-run   # Show what would be migrated
`,
	Args: cobra.MinimumNArgs(1),
	Run:  runMigrate,
}

func init() {
	MigrateCmd.Flags().StringVarP(&migrateOutput, "output", "o", ".ai-rulez", "Output directory for V3 migration")
	MigrateCmd.Flags().BoolVar(&migrateValidate, "validate", false, "Validate migration without writing files")
	MigrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be migrated without actually migrating")
	MigrateCmd.Flags().BoolVar(&migrateForce, "force", false, "Force migration even if output directory exists")

	RootCmd.AddCommand(MigrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) {
	targetVersion := args[0]

	switch targetVersion {
	case "v3", "3", "3.0":
		runMigrateToV3(cmd, args)
	default:
		logger.Error("Unsupported migration target", "version", targetVersion)
		fmt.Fprintf(os.Stderr, "Supported versions: v3\n")
		os.Exit(1)
	}
}

//nolint:gocyclo // Complex logic, acceptable for this use case
func runMigrateToV3(cmd *cobra.Command, args []string) {
	// 1. Determine config file path
	var configPath string
	if len(args) > 1 {
		configPath = args[1]
	} else {
		// Auto-detect ai-rulez.yaml in current directory
		configPath = detectV2Config()
	}

	if configPath == "" {
		logger.Error("No V2 config found", "hint", "Specify config file or run from directory containing ai-rulez.yaml")
		os.Exit(1)
	}

	// Resolve absolute paths
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		logger.Error("Failed to resolve config path", "path", configPath, "error", err)
		os.Exit(1)
	}

	// Determine the base output directory
	// If --output is ".ai-rulez", use current directory; otherwise use specified path
	outputBaseDir := "."
	if migrateOutput != ".ai-rulez" {
		outputBaseDir = migrateOutput
	}

	absOutputPath, err := filepath.Abs(outputBaseDir)
	if err != nil {
		logger.Error("Failed to resolve output path", "path", outputBaseDir, "error", err)
		os.Exit(1)
	}

	// The actual .ai-rulez directory that will be created
	aiRulezPath := filepath.Join(absOutputPath, ".ai-rulez")

	// 2. Check if .ai-rulez directory exists
	if !migrateDryRun && !migrateValidate {
		if info, err := os.Stat(aiRulezPath); err == nil {
			if !migrateForce {
				var msg string
				if info.IsDir() {
					msg = fmt.Sprintf(".ai-rulez directory already exists: %s", aiRulezPath)
				} else {
					msg = fmt.Sprintf(".ai-rulez path exists but is not a directory: %s", aiRulezPath)
				}
				logger.Error(msg, "hint", "Remove it, specify different --output, or use --force to overwrite")
				os.Exit(1)
			}
			// Create timestamped backup before force delete
			if info.IsDir() {
				backupPath := CreateBackup(aiRulezPath)
				if backupPath != "" {
					logger.Info("Created backup of existing .ai-rulez directory", "backup", backupPath)
				}
			}
			logger.Info("Forcing migration, will remove and recreate .ai-rulez directory", "path", aiRulezPath)
			if err := os.RemoveAll(aiRulezPath); err != nil {
				logger.Error("Failed to remove existing directory", "path", aiRulezPath, "error", err)
				os.Exit(1)
			}
		}
	}

	// 3. Create migrator
	migrator := migration.NewV2ToV3Migrator(absConfigPath, absOutputPath)

	// 4. Dry run mode
	if migrateDryRun {
		showMigrationPlan(migrator)
		return
	}

	// 5. Validation-only mode
	if migrateValidate {
		showMigrationValidation(migrator)
		return
	}

	// 6. Perform migration
	ctx := context.Background()
	if err := migrator.Migrate(ctx); err != nil {
		logger.Error("Migration failed", "error", err)
		if oopsErr, ok := oops.AsOops(err); ok {
			if hint := oopsErr.Hint(); hint != "" {
				fmt.Fprintf(os.Stderr, "Hint: %s\n", hint)
			}
		}
		os.Exit(1)
	}

	// 7. Delete backup directory if it exists (migration was successful)
	DeleteBackupDirectory(absOutputPath)

	// 8. Display success message
	displayMigrationSuccess(absConfigPath, aiRulezPath)
}

func detectV2Config() string {
	candidates := []string{
		"ai-rulez.yaml",
		"ai-rulez.yml",
		".ai-rulez.yaml",
		".ai-rulez.yml",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func showMigrationPlan(migrator *migration.V2ToV3Migrator) {
	fmt.Println("Migration Plan (dry-run):")
	fmt.Println()

	// Load V2 config to show what would be migrated
	plan, err := migrator.GetPlan()
	if err != nil {
		logger.Error("Failed to generate migration plan", "error", err)
		if oopsErr, ok := oops.AsOops(err); ok {
			if hint := oopsErr.Hint(); hint != "" {
				fmt.Fprintf(os.Stderr, "Hint: %s\n", hint)
			}
		}
		os.Exit(1)
	}

	fmt.Printf("Project: %s\n", plan.ProjectName)
	fmt.Printf("Source: %s\n", plan.SourcePath)
	fmt.Printf("Output: %s\n", plan.OutputDir)
	fmt.Println()

	if len(plan.Rules) > 0 {
		fmt.Printf("Rules to migrate: %d\n", len(plan.Rules))
		for _, rule := range plan.Rules {
			fmt.Printf("  - %s → rules/%s.md\n", rule.Name, rule.Filename)
		}
		fmt.Println()
	}

	if len(plan.Context) > 0 {
		fmt.Printf("Context to migrate: %d\n", len(plan.Context))
		for _, ctx := range plan.Context {
			fmt.Printf("  - %s → context/%s.md\n", ctx.Name, ctx.Filename)
		}
		fmt.Println()
	}

	if len(plan.Skills) > 0 {
		fmt.Printf("Skills to migrate: %d\n", len(plan.Skills))
		for _, skill := range plan.Skills {
			fmt.Printf("  - %s → skills/%s/SKILL.md\n", skill.Name, skill.ID)
		}
		fmt.Println()
	}

	if len(plan.Presets) > 0 {
		fmt.Printf("Presets detected: %v\n", strings.Join(plan.Presets, ", "))
	}
}

func showMigrationValidation(migrator *migration.V2ToV3Migrator) {
	fmt.Println("Validating V2 configuration...")
	fmt.Println()

	// Load V2 config to show what would be migrated
	plan, err := migrator.GetPlan()
	if err != nil {
		logger.Error("Validation failed", "error", err)
		if oopsErr, ok := oops.AsOops(err); ok {
			if hint := oopsErr.Hint(); hint != "" {
				fmt.Fprintf(os.Stderr, "Hint: %s\n", hint)
			}
		}
		os.Exit(1)
	}

	fmt.Printf("Project: %s\n", plan.ProjectName)
	fmt.Printf("Source: %s\n", plan.SourcePath)
	fmt.Println()

	if len(plan.Rules) > 0 {
		fmt.Printf("Rules to migrate: %d\n", len(plan.Rules))
	}

	if len(plan.Context) > 0 {
		fmt.Printf("Context to migrate: %d\n", len(plan.Context))
	}

	if len(plan.Skills) > 0 {
		fmt.Printf("Skills to migrate: %d\n", len(plan.Skills))
	}

	if len(plan.Presets) > 0 {
		fmt.Printf("Presets detected: %v\n", strings.Join(plan.Presets, ", "))
	}

	fmt.Println()
	fmt.Println("V2 configuration is valid and ready for migration.")
}

// CreateBackup creates a timestamped backup of the given directory
func CreateBackup(sourcePath string) string {
	parentDir := filepath.Dir(sourcePath)
	filename := filepath.Base(sourcePath)

	// Create timestamped backup directory name
	timestamp := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s.backup.%s", filename, timestamp)
	backupPath := filepath.Join(parentDir, backupName)

	// Copy directory recursively
	if err := CopyDir(sourcePath, backupPath); err != nil {
		logger.Warn("Failed to create backup", "source", sourcePath, "backup", backupPath, "error", err)
		return ""
	}

	return backupPath
}

// CopyDir recursively copies a directory from src to dst
func CopyDir(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	// List entries in source
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}

func displayMigrationSuccess(source, output string) {
	fmt.Println()
	fmt.Println("Migration completed successfully!")
	fmt.Println()
	fmt.Printf("Source:  %s\n", source)
	fmt.Printf("Output:  %s\n", output)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Review the migrated files in %s\n", output)
	fmt.Println("  2. Run 'ai-rulez generate' to test the migration")
	fmt.Println("  3. Compare generated files with your previous outputs")
	fmt.Println()
	fmt.Println("Note: Your original V2 config file is unchanged.")
}

// DeleteBackupDirectory removes any timestamped backup directories created during migration
// If deletion fails, it logs a warning but does not fail the migration
// This is exported for testing purposes
func DeleteBackupDirectory(outputDir string) {
	// Find all .ai-rulez.backup.* directories in the parent directory
	parentDir := outputDir
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		logger.Warn("Failed to check for backup directories", "path", parentDir, "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), ".ai-rulez.backup.") {
			backupPath := filepath.Join(parentDir, entry.Name())
			if err := os.RemoveAll(backupPath); err != nil {
				logger.Warn("Failed to delete backup directory", "path", backupPath, "error", err)
				// Don't fail the migration, just warn
			} else {
				logger.Info("Deleted backup directory", "path", backupPath)
			}
		}
	}
}
