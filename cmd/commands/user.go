package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

var (
	userScope bool
	assumeYes bool
)

const (
	userConfigSubdir = "ai-rulez"
	userConfigRoot   = ".config"
)

// userHomeDir is the home directory user scope writes below; a variable so tests
// can point it at a temporary directory.
var userHomeDir = os.UserHomeDir

// userConfigPath resolves where the user config lives: the global --config flag
// when given (a config directory or a config file), otherwise
// $XDG_CONFIG_HOME/ai-rulez, otherwise ~/.config/ai-rulez.
func userConfigPath(home string) string {
	if cfgFile != "" {
		return cfgFile
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, userConfigSubdir)
	}
	return filepath.Join(home, userConfigRoot, userConfigSubdir)
}

// newUserGenerator loads the user config and returns a Generator in user scope
// whose base directory is the home directory. The user config has the same schema
// as a project config but is loaded without machine-local overlays, and never
// from the project in the working directory.
func newUserGenerator(ctx context.Context) (*generator.Generator, *config.Config, error) {
	home, err := userHomeDir()
	if err != nil || home == "" {
		return nil, nil, oops.Wrapf(err, "find the home directory")
	}
	if !filepath.IsAbs(home) {
		return nil, nil, oops.With("home", home).
			Hint("Set HOME to an absolute path; --user never writes relative to the working directory").
			Errorf("the home directory %q is not an absolute path", home)
	}
	path := userConfigPath(home)
	if err := requireUserConfig(path); err != nil {
		return nil, nil, err
	}
	cfg, err := config.LoadConfigFromFile(ctx, path, config.WithoutLocal())
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // already contextual
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, err //nolint:wrapcheck // already contextual
	}
	cfg.BaseDir = home
	gen := generator.NewGenerator(cfg)
	gen.SetUserScope()
	gen.SetContext(ctx)
	if wd, err := os.Getwd(); err == nil {
		gen.SetProjectDir(wd)
	}
	return gen, cfg, nil
}

// requireUserConfig fails with a hint when nothing is at the user config path, so
// an absent user config is never confused with the project's.
func requireUserConfig(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return userConfigMissing(path, err)
	}
	if !info.IsDir() {
		return nil
	}
	for _, name := range configBaseNames {
		if _, err := os.Stat(filepath.Join(path, name)); err == nil {
			return nil
		}
	}
	return userConfigMissing(path, nil)
}

func userConfigMissing(path string, cause error) error {
	err := oops.With("path", path).
		Hint(fmt.Sprintf("Create %s/config.toml with version, name and presets (for example presets = [\"claude\"]) plus rules/ and skills/ beside it, "+
			"or pass --config <dir>", path))
	if cause != nil {
		return err.Wrapf(cause, "no user config found")
	}
	return err.Errorf("no user config found in %s", path)
}

// runUserGenerate handles `generate --user`.
func runUserGenerate(ctx context.Context) error {
	gen, cfg, err := newUserGenerator(ctx)
	if err != nil {
		return err
	}
	plan, err := gen.PlanUser(profile)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	printUserPlan(cfg.BaseDir, plan)
	if dryRun {
		logger.Info("Dry run: nothing was written")
		return nil
	}
	if len(plan.Writes)+len(plan.Merges)+len(plan.Stale)+len(plan.Unmerge) == 0 {
		logger.Info("Nothing to write")
		return nil
	}
	if !assumeYes && !confirmProceed(fmt.Sprintf("Write %d file(s), merge into %d shared file(s) and remove %d stale file(s) in %s?",
		len(plan.Writes), len(plan.Merges), len(plan.Stale), cfg.BaseDir)) {
		return oops.Hint("Pass --yes to proceed without the prompt (required in non-interactive shells), or --dry-run to preview").
			Errorf("aborted: nothing was written")
	}
	_, err = gen.GenerateUser(profile)
	return err //nolint:wrapcheck // already contextual
}

// printUserPlan lists every path a user-scope run touches before anything is written.
func printUserPlan(home string, plan *generator.UserPlan) {
	progress.PrintlnIfNotQuiet(fmt.Sprintf("user-level plan (profile %s, home %s)", plan.Profile, home))
	for _, p := range plan.Writes {
		progress.PrintlnIfNotQuiet("  write:   " + p)
	}
	for _, p := range plan.Merges {
		progress.PrintlnIfNotQuiet("  merge:   " + p + " (ai-rulez keys only)")
	}
	for _, p := range plan.Unmerge {
		progress.PrintlnIfNotQuiet("  unmerge: " + p + " (take back keys no longer generated)")
	}
	for _, p := range plan.Stale {
		progress.PrintlnIfNotQuiet("  remove:  " + p + " (no longer generated)")
	}
	for _, skip := range plan.Skips {
		progress.PrintlnIfNotQuiet(fmt.Sprintf("  skip:    %s (%s)", skip.Path, skip.Reason))
	}
	if plan.Dropped > 0 {
		progress.PrintlnIfNotQuiet(fmt.Sprintf("  %d project-shaped output(s) have no documented user-level location and are not written", plan.Dropped))
		for _, entry := range plan.Unmapped {
			logger.Debug("not written: no user-level location", "output", entry)
		}
	}
	for _, w := range plan.Warnings {
		logger.Warn(w)
	}
}

// runUserClean handles `clean --user`.
func runUserClean() error {
	gen, cfg, err := newUserGenerator(context.Background())
	if err != nil {
		return err
	}
	opts := generator.CleanOptions{DryRun: true, KeepGitignore: true, KeepManifest: cleanKeepManifest}
	plan, err := gen.Clean(profile, opts)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if plan.Empty() {
		logger.Success("Nothing to clean: no user-level generated files found", "profile", plan.Profile)
		return nil
	}
	printCleanPlan(cfg.BaseDir, plan)
	if cleanDryRun {
		logger.Info("Dry run: no files were removed")
		return nil
	}
	if !cleanForce && !confirmRemoval("", fmt.Sprintf("%d generated file(s) in %s", len(plan.Files), cfg.BaseDir)) {
		logger.Info("Aborted: nothing removed")
		return nil
	}
	opts.DryRun = false
	if _, err := gen.Clean(profile, opts); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	logger.Success("Removed user-level generated files", "files", len(plan.Files), "directories", len(plan.Dirs))
	return nil
}

// confirmProceed asks a yes/no question on an interactive terminal; anything else
// (a pipe, CI) answers no.
func confirmProceed(question string) bool { return askYesNo(question + " (y/N): ") }

// handleUserGenerate runs `generate --user` when the flag is set and reports
// whether it did, exiting non-zero on failure.
func handleUserGenerate(args []string) bool {
	if !userScope {
		return false
	}
	if recursive || pluginMode || len(args) > 0 {
		fmtError(oops.Errorf("--user cannot be combined with --recursive, --plugin or a config-file argument; use --config to choose the user config"))
		os.Exit(1)
	}
	if err := runUserGenerate(context.Background()); err != nil {
		fmtError(err)
		os.Exit(1)
	}
	return true
}

// handleUserClean runs `clean --user` when the flag is set and reports whether it
// did, exiting non-zero on failure.
func handleUserClean(args []string) bool {
	if !userScope {
		return false
	}
	if len(args) > 0 {
		fmtError(oops.Errorf("--user cannot be combined with a config-file argument; use --config to choose the user config"))
		os.Exit(1)
	}
	if err := runUserClean(); err != nil {
		fmtError(err)
		os.Exit(1)
	}
	return true
}
