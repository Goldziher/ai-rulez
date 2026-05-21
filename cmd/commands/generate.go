package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/Goldziher/ai-rulez/internal/walkutil"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRun          bool
	updateGitignore bool
	recursive       bool
	skipCLIMCP      bool
	noFetch         bool
	profile         string
	configDir       string
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
	GenerateCmd.Flags().StringVar(&profile, "profile", "", "Profile to generate (default: from config or 'default')")
	GenerateCmd.Flags().BoolVar(&noFetch, "no-fetch", false, "Skip fetching remote includes, use cached content only")
	GenerateCmd.Flags().StringVar(&configDir, "config-dir", "", "Configuration directory name (default: .ai-rulez)")
}

func runGenerate(cmd *cobra.Command, args []string) {
	progress.SetQuiet(viper.GetBool("quiet"))

	// Set no-fetch flag for include resolution (before any config loading)
	includes.SkipFetch = noFetch

	if recursive {
		runRecursiveGenerate()
		return
	}

	ctx := context.Background()

	// Load configuration
	cfg, err := loadConfigForCommand(ctx, args)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		os.Exit(1)
	}

	// Suggest migration for YAML configs
	yamlPath := filepath.Join(cfg.ConfigDir, "config.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		logger.Info("Tip: run 'ai-rulez migrate v4' to convert config.yaml to TOML format")
	}

	// Honor --update-gitignore: force-enable gitignore update regardless of config
	if updateGitignore {
		enabled := true
		cfg.Gitignore = &enabled
	}

	// Create generator
	gen := generator.NewGenerator(cfg)

	if dryRun {
		plan, err := gen.DryRun(profile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		for _, line := range plan {
			progress.PrintlnIfNotQuiet(line)
		}
		return
	}

	// Generate files
	if err := gen.Generate(profile); err != nil {
		fmtError(err)
		os.Exit(1)
	}
}

func loadConfigForCommand(ctx context.Context, args []string) (*config.Config, error) {
	if len(args) > 0 {
		return config.LoadConfigFromFile(ctx, args[0])
	}
	if cfgFile != "" {
		return config.LoadConfigFromFile(ctx, cfgFile)
	}
	if configDir != "" {
		return config.LoadConfigFromDir(ctx, ".", configDir)
	}
	return config.LoadConfig(ctx, ".")
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

// configBaseNames lists, in priority order, the file names that mark a
// `.ai-rulez/` directory as containing a config we should generate from.
// TOML is preferred; YAML (.yaml/.yml) and JSON are also supported.
var configBaseNames = [...]string{
	configFileTOML,
	configFileYAML, "config.yml",
	configFileJSON,
}

// defaultConfigDirName is the directory base name that may contain a config file.
// Only this directory is inspected; everything else is pruned.
const defaultConfigDirName = ".ai-rulez"

// libraryDirName marks a shared rule library (a directory containing a
// root-level config plus included `.ai-rulez/` module configs that are
// meant to be consumed via `includes`, not generated from). When the walk
// encounters a directory by this name with a root config file, the entire
// subtree is pruned.
const libraryDirName = "ai-rulez"

// dirHasRootConfig reports whether dir contains at its root a config file
// matching one of [configBaseNames]. Used to detect shared rule libraries.
func dirHasRootConfig(dir string) bool {
	for _, base := range configBaseNames {
		if info, err := os.Stat(filepath.Join(dir, base)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// findConfigFilesRecursively walks the working tree starting at "." and
// returns paths to all `.ai-rulez/config.*` files it finds.
//
// It aggressively prunes directories that cannot contain user content
// (build outputs, dependency caches, VCS metadata, hidden directories) via
// [walkutil.ShouldSkipDir], and is robust against transient lstat failures
// on stale symlinks (common in Rust `target/` and similar) — such errors
// cause the offending entry to be skipped, never to abort the whole walk.
//
// Once a config directory is encountered, its subtree is not descended into:
// the config files are looked up directly with os.Stat.
func findConfigFilesRecursively() []string {
	var configFiles []string
	spinner := progress.NewSpinner("Searching for configuration files...")

	walkErr := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		return walkConfigDir(path, d, err, &configFiles, spinner)
	})

	if err := spinner.Finish(); err != nil {
		logger.Debug("Failed to finish spinner", "error", err)
	}

	if walkErr != nil {
		fmtError(walkErr)
		os.Exit(1)
	}

	return configFiles
}

// walkConfigDir is the per-entry callback for findConfigFilesRecursively.
// It records any `.ai-rulez/config.*` it finds and returns SkipDir for
// directories that cannot contain user content.
func walkConfigDir(path string, d os.DirEntry, err error, out *[]string, spinner *progress.Bar) error {
	if err != nil {
		logger.Debug("walk: skipping entry", "path", path, "error", err)
		if d != nil && d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if !d.IsDir() {
		return nil
	}

	name := d.Name()

	if name == targetConfigDirName() {
		if cfg := findConfigInDir(path); cfg != "" {
			*out = append(*out, cfg)
			if addErr := spinner.Add(1); addErr != nil {
				logger.Debug("Failed to update spinner", "error", addErr)
			}
		}
		return filepath.SkipDir
	}

	if path == "." {
		return nil
	}

	// Shared rule library subtree (`ai-rulez/` with a root config) — its
	// nested `.ai-rulez/` modules are for inclusion, not generation.
	if name == libraryDirName && dirHasRootConfig(path) {
		logger.Debug("walk: skipping shared rule library subtree", "path", path)
		return filepath.SkipDir
	}

	if walkutil.ShouldSkipDir(name) {
		return filepath.SkipDir
	}
	return nil
}

func targetConfigDirName() string {
	if configDir != "" {
		return configDir
	}
	return defaultConfigDirName
}

// findConfigInDir returns the path to the first config file in dir matching
// configBaseNames (in priority order), or "" if none exists.
func findConfigInDir(dir string) string {
	for _, base := range configBaseNames {
		candidate := filepath.Join(dir, base)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func processConfigFiles(configFiles []string) int {
	fileCounter := progress.NewFileCounter(len(configFiles), "Processing configurations")

	// Each config has its own working directory and produces independent
	// output, so we can process them concurrently. Cap parallelism at
	// NumCPU — past that we just contend on disk I/O without speedup.
	workers := runtime.NumCPU()
	if workers > len(configFiles) {
		workers = len(configFiles)
	}
	if workers < 1 {
		workers = 1
	}

	var totalGenerated int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for _, configPath := range configFiles {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			atomic.AddInt64(&totalGenerated, int64(processConfigFile(configPath, fileCounter)))
		}()
	}
	wg.Wait()

	fileCounter.Finish()
	return int(totalGenerated)
}

func processConfigFile(configPath string, fileCounter *progress.FileCounter) int {
	fileCounter.StartFile(configPath)

	ctx := context.Background()
	cfg, err := config.LoadConfigFromFile(ctx, configPath)
	if err != nil {
		fileCounter.Error(err)
		return 0
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fileCounter.Error(err)
		return 0
	}

	// Honor --update-gitignore: force-enable gitignore update regardless of config
	if updateGitignore {
		enabled := true
		cfg.Gitignore = &enabled
	}

	// Create generator
	gen := generator.NewGenerator(cfg)

	if dryRun {
		plan, err := gen.DryRun(profile)
		if err != nil {
			fileCounter.Error(err)
			return 0
		}
		for _, line := range plan {
			progress.PrintlnIfNotQuiet("  " + line)
		}
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
