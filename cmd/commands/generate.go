package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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
	dryRun             bool
	updateGitignore    bool
	recursive          bool
	noFetch            bool
	profile            string
	configDir          string
	mcpEnv             []string
	mcpEnvFiles        []string
	pluginMode         bool
	noLocal            bool
	allowLocalDrift    bool
	pluginIfConfigured bool
	generateCheck      bool
	generateLocked     bool
	generateFrozen     bool
)

var GenerateCmd = &cobra.Command{
	Use:   "generate [config-file]",
	Short: "Generate AI assistant rule files from configuration",
	Long: `Generate AI assistant rule files based on the configuration.
This will create markdown files for various AI assistants like Claude,
Cursor, Devin, etc. based on your configuration.`,
	Aliases: []string{"gen", "g"},
	Args:    cobra.MaximumNArgs(1),
	Run:     runGenerate,
}

func init() {
	GenerateCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Show what would be generated without writing files")
	GenerateCmd.Flags().BoolVar(&generateCheck, "check", false,
		"Verify the committed output matches the sources without writing: list differing files and exit 2 on drift (for CI)")
	GenerateCmd.Flags().BoolVar(&generateLocked, "locked", false,
		"Require ai-rulez.lock to cover every remote include and installed skill and fetch exactly the pinned commits (for CI)")
	GenerateCmd.Flags().BoolVar(&generateFrozen, "frozen", false,
		"Like --locked, and never use the network: resolve only from the local cache, verified against the lock")
	GenerateCmd.Flags().BoolVarP(&updateGitignore, "gitignore", "i", false, "Update .gitignore files to include generated output patterns")
	GenerateCmd.Flags().BoolVar(&updateGitignore, "update-gitignore", false, "Deprecated alias for --gitignore")
	GenerateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Find and process configuration files recursively")
	registerRemovedCLIMCPFlags(GenerateCmd)
	GenerateCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile to generate, or a comma-separated list to compose several (default: from config or 'default')")
	GenerateCmd.Flags().BoolVarP(&noFetch, "no-fetch", "f", false, "Skip fetching remote includes, use cached content only")
	GenerateCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	GenerateCmd.Flags().StringArrayVarP(&mcpEnv, "env", "e", nil, "MCP env override in KEY=VALUE form (repeatable)")
	GenerateCmd.Flags().StringArrayVarP(&mcpEnvFiles, "env-file", "E", nil, "Dotenv file for MCP env placeholders (repeatable)")
	GenerateCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	GenerateCmd.Flags().BoolVar(&allowLocalDrift, "allow-local-drift", false,
		"Write output even when machine-local config would change files shared with the team")
	GenerateCmd.Flags().BoolVar(&pluginMode, "plugin", false, "Generate distributable plugin bundles and a marketplace index from the [plugin] block")
	GenerateCmd.Flags().BoolVar(&pluginIfConfigured, "if-configured", false, "Skip plugin generation when no plugin authoring configuration is present")
	if err := GenerateCmd.Flags().MarkDeprecated("update-gitignore", "use --gitignore instead"); err != nil {
		logger.Debug("Failed to mark update-gitignore as deprecated", "error", err)
	}
	if err := GenerateCmd.Flags().MarkHidden("update-gitignore"); err != nil {
		logger.Debug("Failed to hide update-gitignore flag", "error", err)
	}
}

// registerRemovedCLIMCPFlags keeps --no-configure-cli-mcp / -M and --skip-cli-mcp
// / -S accepted so existing scripts do not break. generate never configures
// CLI-based MCP tools (it writes MCP config files only), so the flags have
// nothing to skip; they are hidden and print a deprecation notice.
func registerRemovedCLIMCPFlags(cmd *cobra.Command) {
	var unused bool
	for _, f := range []struct{ name, short string }{{"no-configure-cli-mcp", "M"}, {"skip-cli-mcp", "S"}} {
		cmd.Flags().BoolVarP(&unused, f.name, f.short, false, "Has no effect: generate does not configure CLI-based MCP tools")
		if err := cmd.Flags().MarkDeprecated(f.name, "it has no effect, generate only writes MCP config files"); err != nil {
			logger.Debug("Failed to mark flag as deprecated", "flag", f.name, "error", err)
		}
	}
}

func runGenerate(cmd *cobra.Command, args []string) {
	progress.SetQuiet(viper.GetBool("quiet"))

	// Set no-fetch flag for include resolution (before any config loading)
	includes.SkipFetch = noFetch
	switch {
	case generateFrozen:
		includes.Mode, includes.SkipFetch = includes.LockFrozen, true
	case generateLocked:
		includes.Mode = includes.LockRequire
	}

	if generateCheck {
		if err := checkGenerateCheckFlags(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if code := runDriftCheck(args, recursive, driftRender); code != 0 {
			os.Exit(code)
		}
		return
	}

	if recursive {
		if code := runRecursiveGenerate(); code != 0 {
			os.Exit(code)
		}
		return
	}

	ctx := context.Background()

	// Load configuration
	cfg, err := loadConfigForCommand(ctx, args, pluginLoadOptions(pluginMode)...)
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

	applyGenerateOverrides(cfg)
	if pluginMode && pluginIfConfigured && !cfg.HasPluginAuthoring() {
		logger.Info("Skipping plugin generation: no plugin authoring configuration")
		return
	}

	// Create generator
	gen := generator.NewGenerator(cfg)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetContext(ctx)

	if pluginMode {
		runPluginGenerate(gen)
		return
	}

	if dryRun {
		if err := printDryRun(gen, ""); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		return
	}

	// Generate files
	if err := gen.Generate(profile); err != nil {
		fmtError(err)
		os.Exit(1)
	}
}

// printDryRun prints the generation plan and returns an error when the plan
// holds local drift a real run would refuse, so a blocked dry run exits non-zero.
func printDryRun(gen *generator.Generator, indent string) error {
	plan, err := gen.DryRun(profile)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	for _, line := range plan {
		progress.PrintlnIfNotQuiet(indent + line)
	}
	return gen.DryRunBlocked() //nolint:wrapcheck // already contextual
}

// runPluginGenerate handles `generate --plugin`: it renders (or, with --dry-run,
// previews) the distributable plugin bundles and marketplace index.
func runPluginGenerate(gen *generator.Generator) {
	if dryRun {
		plan, err := gen.DryRunPlugin(profile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		for _, line := range plan {
			progress.PrintlnIfNotQuiet(line)
		}
		return
	}
	if err := gen.GeneratePlugin(profile); err != nil {
		fmtError(err)
		os.Exit(1)
	}
}

func loadConfigForCommand(ctx context.Context, args []string, opts ...config.LoadOption) (*config.Config, error) {
	if noLocal {
		opts = append(opts, config.WithoutLocal())
	}
	if len(args) > 0 {
		return config.LoadConfigFromFile(ctx, args[0], opts...)
	}
	if cfgFile != "" {
		return config.LoadConfigFromFile(ctx, cfgFile, opts...)
	}
	if configDir != "" {
		return config.LoadConfigFromDir(ctx, ".", configDir, opts...)
	}
	return config.LoadConfig(ctx, ".", opts...)
}

// pluginLoadOptions returns the load options for plugin bundle work. Plugin
// bundles are distributable, so machine-local overlays never apply to them.
func pluginLoadOptions(plugin bool) []config.LoadOption {
	if plugin || noLocal {
		return []config.LoadOption{config.WithoutLocal()}
	}
	return nil
}

// runRecursiveGenerate processes every discovered config and returns the
// process exit code: 1 when any config failed to load, validate, or generate
// (the remaining configs are still processed and every error is printed), else 0.
func runRecursiveGenerate() int {
	configFiles := findConfigFilesRecursively()
	if pluginMode {
		var err error
		configFiles, err = selectRecursivePluginConfigs(configFiles)
		if err != nil {
			fmtError(err)
			return 1
		}
	}
	if len(configFiles) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}

	progress.PrintIfNotQuiet("Found %d configuration file(s)\n", len(configFiles))
	totalGenerated, failed := processConfigFiles(configFiles)
	progress.PrintIfNotQuiet("\n✅ Total: Generated %d file(s) from %d config(s)\n", totalGenerated, len(configFiles))
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ %d of %d config(s) failed:\n", len(failed), len(configFiles))
		for _, path := range failed {
			fmt.Fprintf(os.Stderr, "  - %s\n", path)
		}
		return 1
	}
	return 0
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

// configConventionRoot is the generic project-level config directory from the
// pi0/config-dir proposal. The ai-rulez subtree inside it mirrors .ai-rulez/.
const (
	configConventionRoot   = ".config"
	configConventionSubdir = "ai-rulez"
)

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

	// Config directory at any depth: ".ai-rulez" by default, or the value of
	// --config-dir (which may itself be a nested path such as .config/ai-rulez).
	if isConfigDirPath(path) {
		recordConfigDir(path, out, spinner)
		return filepath.SkipDir
	}

	// Project-level .config/ai-rulez/ convention. Handled explicitly (and
	// pruned) so the walk never descends into other tools' .config/ subtrees.
	// A sibling .ai-rulez/ wins, matching the non-recursive precedence.
	if configDir == "" && name == configConventionRoot {
		if findConfigInDir(filepath.Join(filepath.Dir(path), defaultConfigDirName)) == "" {
			recordConfigDir(filepath.Join(path, configConventionSubdir), out, spinner)
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

	// Descend through a wrapper directory (e.g. .config/) that is an ancestor
	// of a nested --config-dir target such as .config/ai-rulez.
	if config.IsConfigDirAncestor(path, targetConfigDirName()) {
		return nil
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

// isConfigDirPath reports whether path is a config directory for the active
// target. The comparison is by trailing path suffix so both a top-level
// ".ai-rulez" and a nested "svc/.ai-rulez" (or an explicit nested --config-dir
// such as ".config/ai-rulez") match.
func isConfigDirPath(path string) bool {
	target := filepath.ToSlash(targetConfigDirName())
	rel := filepath.ToSlash(path)
	return rel == target || strings.HasSuffix(rel, "/"+target)
}

// recordConfigDir appends dir's config file to out when one exists and bumps
// the spinner; directories without a config file are silently skipped.
func recordConfigDir(dir string, out *[]string, spinner *progress.Bar) {
	cfg := findConfigInDir(dir)
	if cfg == "" {
		return
	}
	*out = append(*out, cfg)
	if err := spinner.Add(1); err != nil {
		logger.Debug("Failed to update spinner", "error", err)
	}
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

// processConfigFiles generates from every config concurrently and returns the
// number of files written plus the (sorted) paths of the configs that failed.
func processConfigFiles(configFiles []string) (generated int, failed []string) {
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
	var failedMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for _, configPath := range configFiles {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			written, err := processConfigFile(configPath, fileCounter)
			atomic.AddInt64(&totalGenerated, int64(written))
			if err != nil {
				failedMu.Lock()
				failed = append(failed, configPath)
				failedMu.Unlock()
			}
		}()
	}
	wg.Wait()

	fileCounter.Finish()
	sort.Strings(failed)
	return int(totalGenerated), failed
}

// processConfigFile generates from one config. A non-nil error means the config
// failed (and was already reported through fileCounter).
func processConfigFile(configPath string, fileCounter *progress.FileCounter) (int, error) {
	fileCounter.StartFile(configPath)

	ctx := context.Background()
	cfg, err := config.LoadConfigFromFile(ctx, configPath, pluginLoadOptions(pluginMode)...)
	if err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	applyGenerateOverrides(cfg)

	// Create generator
	gen := generator.NewGenerator(cfg)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetContext(ctx)
	if pluginMode {
		if pluginIfConfigured && !cfg.HasPluginAuthoring() {
			fileCounter.FinishFile()
			return 0, nil
		}
		if dryRun {
			plan, err := gen.DryRunPlugin(profile)
			if err != nil {
				fileCounter.ErrorFor(configPath, err)
				return 0, err
			}
			for _, line := range plan {
				progress.PrintlnIfNotQuiet("  " + line)
			}
			fileCounter.FinishFile()
			return 0, nil
		}
		written, err := gen.GeneratePluginFiles(profile)
		if err != nil {
			fileCounter.ErrorFor(configPath, err)
			return 0, err
		}
		fileCounter.FinishFile()
		return written, nil
	}

	if dryRun {
		if err := printDryRun(gen, "  "); err != nil {
			fileCounter.ErrorFor(configPath, err)
			return 0, err
		}
		fileCounter.FinishFile()
		return 0, nil
	}

	// Generate files
	written, err := gen.GenerateFiles(profile)
	if err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	fileCounter.FinishFile()

	return written, nil
}

func applyGenerateOverrides(cfg *config.Config) {
	if updateGitignore {
		enabled := true
		cfg.Gitignore = &enabled
	}
	if len(mcpEnv) > 0 {
		cfg.MCPEnvOverrides = parseMCPEnvOverrides(mcpEnv)
	}
	if len(mcpEnvFiles) > 0 {
		cfg.MCPEnvFiles = append([]string(nil), mcpEnvFiles...)
	}
}

func parseMCPEnvOverrides(values []string) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			logger.Error("Invalid --env value; expected KEY=VALUE", "value", value)
			os.Exit(1)
		}
		out[key] = val
	}
	return out
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

// checkGenerateCheckFlags rejects flags that make no sense with --check, which
// must never write.
func checkGenerateCheckFlags() error {
	switch {
	case dryRun:
		return oops.Errorf("--check and --dry-run are mutually exclusive: --check already writes nothing")
	case pluginMode:
		return oops.Errorf("--check does not cover plugin bundles; use `ai-rulez verify --plugin`")
	case updateGitignore:
		return oops.Errorf("--check cannot be combined with --gitignore, which writes .gitignore files")
	}
	return nil
}
