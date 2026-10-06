package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
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
	generateRole       string
	generateEmitPlan   string
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
	GenerateCmd.Flags().BoolVarP(&generateWatch, "watch", "w", false,
		"Generate, then watch the configuration directory and local include sources and regenerate on every change (Ctrl-C to stop)")
	GenerateCmd.Flags().BoolVar(&generateLocked, "locked", false,
		"Require ai-rulez.lock to cover every remote include and installed skill and fetch exactly the pinned commits (for CI)")
	GenerateCmd.Flags().BoolVar(&generateFrozen, "frozen", false,
		"Like --locked, and never use the network: resolve only from the local cache, verified against the lock")
	GenerateCmd.Flags().BoolVarP(&updateGitignore, "gitignore", "i", false, "Update .gitignore files to include generated output patterns")
	GenerateCmd.Flags().BoolVar(&updateGitignore, "update-gitignore", false, "Deprecated alias for --gitignore")
	GenerateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Find and process configuration files recursively")
	registerRemovedCLIMCPFlags(GenerateCmd)
	GenerateCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile to generate, or a comma-separated list to compose several (default: from config or 'default')")
	GenerateCmd.Flags().StringVar(&generateRole, flagRole, "",
		"Generate the slice of content a role selects instead of a profile (see 'ai-rulez roles list'); mutually exclusive with --profile")
	GenerateCmd.Flags().BoolVarP(&noFetch, "no-fetch", "f", false, "Skip fetching remote includes, use cached content only")
	GenerateCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	GenerateCmd.Flags().StringArrayVarP(&mcpEnv, "env", "e", nil, "MCP env override in KEY=VALUE form (repeatable)")
	GenerateCmd.Flags().StringArrayVarP(&mcpEnvFiles, "env-file", "E", nil, "Dotenv file for MCP env placeholders (repeatable)")
	GenerateCmd.Flags().StringVar(&generateEmitPlan, "emit-plan", "",
		"Write the generation plan (every file that would be written, merged or removed, with digests; no secrets) as JSON to FILE ('-' for stdout) and apply nothing; see schema/plan.schema.json")
	GenerateCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	GenerateCmd.Flags().BoolVar(&allowLocalDrift, "allow-local-drift", false,
		"Write output even when machine-local config would change files shared with the team")
	GenerateCmd.Flags().BoolVar(&userScope, "user", false,
		"Generate the user-level config (default ~/.config/ai-rulez, or --config) into the home directories each harness reads: ~/.claude, ~/.agents/skills, ~/.codex, ~/.gemini, ~/.config/opencode, ~/.copilot, ~/.pi/agent")
	GenerateCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "With --user: write without the confirmation prompt; always: do not warn about new hook and MCP commands")
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
	applyLockFlags()

	exitOn(checkRoleFlags())
	exitOn(checkEmitPlanFlags())

	if generateWatch {
		if err := runGenerateWatch(watchParentContext(cmd), args); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		return
	}

	if generateCheck {
		runGenerateCheck(args)
		return
	}

	if handleUserGenerate(args) {
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
		if (generateLocked || generateFrozen) && errors.Is(err, config.ErrLockViolation) {
			os.Exit(exitDrift) // a missing or disagreeing lock is drift, the same code as a changed source
		}
		os.Exit(1)
	}

	// The organization policy clamped the configuration at load; refuse to
	// generate from one that tried to loosen it.
	if err := policyGate(cfg); err != nil {
		fmtError(err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		os.Exit(1)
	}

	exitOnLockedDrift(enforceLockedContent(cfg))
	exitOnMovedTags(cfg) // only with --verify-tags or [lock] verify_tags: a pinned tag that moved ends the run

	applyGenerateOverrides(cfg)
	warnFrontmatter(cfg)
	if err := importGate(cfg); err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if pluginMode && pluginIfConfigured && !cfg.HasPluginAuthoring() {
		logger.Info("Skipping plugin generation: no plugin authoring configuration")
		return
	}

	if generateEmitPlan != "" {
		exitOn(emitPlan(ctx, cfg, generateEmitPlan))
		return
	}

	// Create generator
	gen := generator.NewGenerator(cfg)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetContext(ctx)
	exitOn(applyRole(gen))
	exitOn(generatePreflight(cfg, gen))

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

// emitPlan renders the generation plan without writing any output and writes it
// as JSON to dest ("-" is standard output). Nothing is applied.
func emitPlan(ctx context.Context, cfg *config.Config, dest string) error {
	// The checks a real run starts with that only read: nothing is announced or recorded.
	if err := planPreflight(cfg); err != nil {
		return err
	}
	plan, err := generator.PlanOutputs(ctx, cfg, generator.PlanOptions{Profile: profile, Role: generateRole})
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	data, err := generator.MarshalPlan(plan)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if dest == "-" {
		_, err = os.Stdout.Write(data)
		return oops.Wrapf(err, "write the plan")
	}
	if err := config.WriteFileAtomic(dest, data, 0o644); err != nil {
		return oops.With("path", dest).Wrapf(err, "write the plan")
	}
	return nil
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
// Under --locked or --frozen, when every failure is the lock disagreeing with the
// sources it is exitDrift, the code a single root exits with.
func runRecursiveGenerate() int {
	found := discoverConfigFiles()
	configFiles := found.configs
	if pluginMode {
		var err error
		configFiles, err = selectRecursivePluginConfigs(configFiles)
		if err != nil {
			fmtError(err)
			return 1
		}
	}
	// A nested V2/V3 config is no longer read, but it must not vanish from the
	// run: loading it fails with the same ErrLegacyConfig message a single root gives.
	configFiles = append(configFiles, found.legacy...)
	if len(configFiles) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}

	progress.PrintIfNotQuiet("Found %d configuration file(s)\n", len(configFiles))
	totalGenerated, failed, drifted := processConfigFilesCounting(configFiles)
	progress.PrintIfNotQuiet("\n✅ Total: Generated %d file(s) from %d config(s)\n", totalGenerated, len(configFiles))
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ %d of %d config(s) failed:\n", len(failed), len(configFiles))
		for _, path := range failed {
			fmt.Fprintf(os.Stderr, "  - %s\n", path)
		}
		if drifted == len(failed) {
			return exitDrift // every failure is the lock disagreeing with the sources, as for a single root
		}
		return 1
	}
	return 0
}

// configBaseNames lists, in priority order, the file names that mark a
// `.ai-rulez/` directory as containing a config we should generate from.
var configBaseNames = [...]string{
	configFileTOML,
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
	return discoverConfigFiles().configs
}

// discoveredConfigs is what the recursive walk found: the config.toml files
// and the V2/V3 config files of config directories that have no config.toml.
type discoveredConfigs struct {
	configs []string
	legacy  []string
}

// discoverConfigFiles is findConfigFilesRecursively that also reports the
// legacy config files it met, so `generate --recursive` can refuse them by name.
func discoverConfigFiles() discoveredConfigs {
	var found discoveredConfigs
	spinner := progress.NewSpinner("Searching for configuration files...")

	walkErr := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		return walkConfigDir(path, d, err, &found, spinner)
	})

	if err := spinner.Finish(); err != nil {
		logger.Debug("Failed to finish spinner", "error", err)
	}

	if walkErr != nil {
		fmtError(walkErr)
		os.Exit(1)
	}

	return found
}

// walkConfigDir is the per-entry callback for findConfigFilesRecursively.
// It records any `.ai-rulez/config.*` it finds and returns SkipDir for
// directories that cannot contain user content.
func walkConfigDir(path string, d os.DirEntry, err error, out *discoveredConfigs, spinner *progress.Bar) error {
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
// the spinner. A directory with only a legacy V2/V3 config is recorded as
// legacy; one with no config at all is skipped.
func recordConfigDir(dir string, out *discoveredConfigs, spinner *progress.Bar) {
	cfg := findConfigInDir(dir)
	if cfg == "" {
		if legacy := config.LegacyConfigIn(dir); legacy != "" {
			out.legacy = append(out.legacy, legacy)
		}
		return
	}
	out.configs = append(out.configs, cfg)
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
	generated, failed, _ = processConfigFilesCounting(configFiles)
	return generated, failed
}

// processConfigFilesCounting is processConfigFiles that also returns how many of
// the failures were lock drift (see lockDriftError).
func processConfigFilesCounting(configFiles []string) (generated int, failed []string, drifted int) {
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
				if lockDriftError(err) {
					drifted++
				}
				failedMu.Unlock()
			}
		}()
	}
	wg.Wait()

	fileCounter.Finish()
	sort.Strings(failed)
	return int(totalGenerated), failed, drifted
}

// lockDriftError reports whether a failed `generate --locked` or `--frozen` was
// the lock disagreeing with the sources (content drift, or a remote the lock
// does not cover), which exits with exitDrift rather than 1.
func lockDriftError(err error) bool {
	if !generateLocked && !generateFrozen {
		return false
	}
	return isLockedDrift(err)
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

	// The organization policy applies to every root, not only a single one.
	if err := policyGate(cfg); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	if err := enforceLockedContent(cfg); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	applyGenerateOverrides(cfg)
	if err := importGate(cfg); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}

	// Create generator
	gen := generator.NewGenerator(cfg)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetContext(ctx)
	if err := applyRole(gen); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}
	if err := generatePreflight(cfg, gen); err != nil {
		fileCounter.ErrorFor(configPath, err)
		return 0, err
	}
	if pluginMode {
		return processPluginConfig(configPath, cfg, gen, fileCounter)
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

		if details, ok := oopsErr.Context()["errors"].([]string); ok && len(details) > 0 {
			fmt.Fprintf(os.Stderr, "\nValidation errors:\n")
			for _, e := range details {
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

// processPluginConfig is processConfigFile for `generate --plugin`.
func processPluginConfig(configPath string, cfg *config.Config, gen *generator.Generator, fileCounter *progress.FileCounter) (int, error) {
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

// applyLockFlags turns --locked and --frozen into the include lock policy.
func applyLockFlags() {
	includes.RequireWhenEnforced = true
	switch {
	case generateFrozen:
		includes.Mode, includes.SkipFetch = includes.LockFrozen, true
	case generateLocked:
		includes.Mode = includes.LockRequire
	}
}

// runGenerateCheck runs `generate --check` and exits with its code.
func runGenerateCheck(args []string) {
	if code := generateCheckCode(args); code != 0 {
		os.Exit(code)
	}
}

// generateCheckCode is `generate --check`: it first requires the sources to
// match ai-rulez.lock when --locked or --frozen is set or the lock is enforced
// (a lock exists and [lock] enforce is not false), the same gate `generate`
// applies before writing, then compares the generated files. Each root is
// loaded once for both.
func generateCheckCode(args []string) int {
	if err := checkGenerateCheckFlags(); err != nil {
		fmtError(err)
		return 1
	}
	return runDriftCheckGated(args, recursive, driftRender, func(cfg *config.Config) error {
		return enforceLockedContentFor(cfg, true)
	})
}

// importGate scans imported content before anything is written, when
// [lint.security] scan_imports is not "off". Dry runs and plugin bundles skip it.
func importGate(cfg *config.Config) error {
	if dryRun || pluginMode {
		return nil
	}
	return enforceScanImports(cfg)
}

// checkRoleFlags rejects --role together with --profile.
func checkRoleFlags() error {
	if generateRole != "" && profile != "" {
		return oops.Hint("A role replaces the profile selection; pass only one").
			Errorf("--role and --profile are mutually exclusive")
	}
	if generateRole != "" && pluginMode {
		return oops.Errorf("--role cannot be combined with --plugin: plugin bundles are built from the full content")
	}
	return nil
}

// checkEmitPlanFlags rejects --emit-plan with the modes that never reach it or
// plan something else: --watch, --check, --user and --recursive return before
// the plan is made, and --plugin renders bundles, not the in-repo outputs.
func checkEmitPlanFlags() error {
	if generateEmitPlan == "" {
		return nil
	}
	for _, f := range []struct {
		name string
		set  bool
	}{{"--watch", generateWatch}, {"--check", generateCheck}, {"--user", userScope}, {"--recursive", recursive}, {"--plugin", pluginMode}} {
		if f.set {
			return oops.Hint("Run --emit-plan on its own, from the project root").
				Errorf("--emit-plan cannot be combined with %s", f.name)
		}
	}
	return nil
}

// applyRole switches a Generator to the role given with --role, if any.
func applyRole(gen *generator.Generator) error {
	if generateRole == "" {
		return nil
	}
	return gen.SetRole(generateRole) //nolint:wrapcheck // already contextual
}

// errLockedSourceDrift marks a `generate --locked` refusal because authored
// content no longer matches ai-rulez.lock.
var errLockedSourceDrift = errors.New("authored content differs from " + "ai-rulez.lock")

// errLockedSignature marks a `generate --locked` refusal because [signing]
// require is not met by the lock attestation (AR720 to AR727). Re-locking does not
// fix it: it changes the lock and invalidates the signature.
var errLockedSignature = errors.New("the lock attestation does not satisfy [signing] require")

// isLockedDrift reports whether err is a lock refusal that exits with exitDrift:
// authored content or a remote disagreeing with the lock, or a missing or invalid
// attestation.
func isLockedDrift(err error) bool {
	return errors.Is(err, errLockedSourceDrift) || errors.Is(err, errLockedSignature) || errors.Is(err, config.ErrLockViolation)
}

// exitOnLockedDrift exits with the drift code when err says authored content no
// longer matches the lock, and with 1 on any other error.
func exitOnLockedDrift(err error) {
	if err == nil {
		return
	}
	fmtError(err)
	if errors.Is(err, errLockedSourceDrift) || errors.Is(err, errLockedSignature) {
		os.Exit(exitDrift)
	}
	os.Exit(1)
}

// enforceLockedContent is the content half of --locked and --frozen: when the
// lock pins authored content, every source must still match it. generate never
// writes the lock.
func enforceLockedContent(cfg *config.Config) error {
	return enforceLockedContentFor(cfg, false)
}

// enforceLockedContentFor is enforceLockedContent for a run that may also be
// `generate --check`, which verifies the content whenever the lock is enforced,
// not only under --locked or --frozen.
func enforceLockedContentFor(cfg *config.Config, check bool) error {
	if !generateLocked && !generateFrozen && !(check && cfg.LockEnforced()) {
		return nil
	}
	lines, err := verifyLockedSources(cfg)
	if err != nil {
		return err
	}
	signLines, err := signingRequiredLines(cfg)
	if err != nil {
		return err
	}
	switch {
	case len(lines) == 0 && len(signLines) == 0:
		return nil
	case len(lines) == 0:
		return oops.Hint("Sign the current lock with `ai-rulez sign --lock` (running `ai-rulez lock` again would invalidate the signature)").
			Wrapf(errLockedSignature, "%s lacks the attestation [signing] require asks for:\n  %s", "ai-rulez.lock", strings.Join(signLines, "\n  "))
	}
	lines = append(lines, signLines...)
	return oops.Hint("Review the change with `ai-rulez lock --diff`, then run `ai-rulez lock` to accept it (and `ai-rulez sign --lock` when [signing] require is set)").
		Wrapf(errLockedSourceDrift, "%s does not match the sources:\n  %s", "ai-rulez.lock", strings.Join(lines, "\n  "))
}
