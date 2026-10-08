package commands

import (
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
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
)

// configBaseNames lists, in priority order, the file names that mark a
// `.ai-rulez/` directory as containing a config we should generate from.
var configBaseNames = [...]string{
	configFileTOML,
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
				if lockDriftError(err) || errors.Is(err, config.ErrPolicyLoosens) {
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

// processConfigFile generates from one config. A non-nil error means the config
// failed (and was already reported through fileCounter).
func processConfigFile(configPath string, fileCounter *progress.FileCounter) (int, error) {
	fileCounter.StartFile(configPath)

	ctx := gitutil.WithMemo(cmdContext())
	cfg, err := loadProjectFile(ctx, configPath, append(pluginLoadOptions(pluginMode), config.WithFrontmatterErrors())...)
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
	if err := movedTagsErr(cfg); err != nil { // --verify-tags or [lock] verify_tags, per root
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
	gen.SetOverwriteUnowned(generateForce)
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
			return exitDrift // every failure is the lock disagreeing with the sources or a policy loosening, the same exit 2 as for a single root
		}
		return 1
	}
	return 0
}
