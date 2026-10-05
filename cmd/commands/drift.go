package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/samber/oops"
)

// Exit codes of the drift checks (`generate --check` and `verify`): 0 means the
// generated files match, 1 means the check could not run (bad configuration, no
// manifest), 2 means at least one generated file differs.
const exitDrift = 2

// driftMode selects how a config is checked.
type driftMode int

const (
	// driftRender re-renders in memory and compares with the disk.
	driftRender driftMode = iota
	// driftManifest compares the manifest's files with their own Content-Hash.
	driftManifest
)

// driftLoadOptions: the committed-manifest check never sees machine-local
// files; the render check honors --no-local like generate does.
func driftLoadOptions(mode driftMode) []config.LoadOption {
	if mode == driftManifest {
		return []config.LoadOption{config.WithoutLocal()}
	}
	return pluginLoadOptions(false)
}

// checkConfigDrift runs the drift check for one loaded config and prints the
// differing files. It returns how many differ.
func checkConfigDrift(cfg *config.Config, mode driftMode) (int, error) {
	gen := generator.NewGenerator(cfg)
	gen.SetContext(context.Background())
	if err := applyRole(gen); err != nil {
		return 0, err
	}
	var (
		drift   []generator.Drift
		checked int
		err     error
	)
	if mode == driftManifest {
		drift, checked, err = gen.VerifyGenerated()
	} else {
		drift, err = gen.CheckDrift(profile)
	}
	if err != nil {
		return 0, err //nolint:wrapcheck // already contextual
	}
	for _, d := range drift {
		fmt.Printf("%s: %s\n", d.Kind, displayDriftPath(cfg, d.Path))
	}
	if len(drift) == 0 && mode == driftManifest {
		progress.PrintlnIfNotQuiet(fmt.Sprintf("verified %d generated file(s) against their Content-Hash", checked))
	}
	return len(drift), nil
}

// displayDriftPath shows a project-relative path relative to the working
// directory, so a nested root reads as svc/CLAUDE.md.
func displayDriftPath(cfg *config.Config, rel string) string {
	abs, err := filepath.Abs(filepath.Join(cfg.BaseDir, filepath.FromSlash(rel)))
	if err != nil {
		return rel
	}
	cwd, err := os.Getwd()
	if err != nil {
		return rel
	}
	out, err := filepath.Rel(cwd, abs)
	if err != nil {
		return rel
	}
	return filepath.ToSlash(out)
}

// runDriftCheck checks a single root (args) or every root under the working
// directory (recursive) and returns the process exit code.
func runDriftCheck(args []string, isRecursive bool, mode driftMode) int {
	fix := "run `ai-rulez generate` and commit the result"
	if isRecursive {
		return runRecursiveDrift(mode, fix)
	}
	cfg, err := loadConfigForCommand(context.Background(), args, driftLoadOptions(mode)...)
	if err != nil {
		fmtError(err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		return 1
	}
	applyGenerateOverrides(cfg)
	n, err := checkConfigDrift(cfg, mode)
	if err != nil {
		fmtError(err)
		return 1
	}
	return finishDrift(n, 1, fix)
}

func runRecursiveDrift(mode driftMode, fix string) int {
	paths := findConfigFilesRecursively()
	if len(paths) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}
	total, failed := 0, 0
	for _, path := range paths {
		cfg, err := config.LoadConfigFromFile(context.Background(), path, driftLoadOptions(mode)...)
		if err == nil {
			err = cfg.Validate()
		}
		if err != nil {
			fmtError(oops.With("config", path).Wrapf(err, "load configuration"))
			failed++
			continue
		}
		applyGenerateOverrides(cfg)
		n, err := checkConfigDrift(cfg, mode)
		if err != nil {
			fmtError(oops.With("config", path).Wrapf(err, "check generated files"))
			failed++
			continue
		}
		total += n
	}
	if failed > 0 {
		return 1
	}
	return finishDrift(total, len(paths), fix)
}

func finishDrift(differing, roots int, fix string) int {
	if differing == 0 {
		logger.Success("Generated files are up to date", "roots", roots)
		return 0
	}
	fmt.Fprintf(os.Stderr, "%d generated file(s) differ from their sources; %s\n", differing, fix)
	return exitDrift
}
