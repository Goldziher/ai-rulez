package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

// Exit codes of the drift checks (`generate --check` and `verify`): 0 means the
// generated files match, 1 means the check could not run (bad configuration, no
// manifest), 2 means at least one generated file differs.
const exitDrift = 2

// exitRank orders exit codes by severity: a tool error (1) outranks drift (2),
// which outranks a partial result (3, `lock` left skills unpinned).
func exitRank(code int) int {
	switch code {
	case 0:
		return 0
	case 1:
		return 3
	case exitDrift:
		return 2
	default:
		return 1
	}
}

// worstExit returns the more severe of two exit codes (1 wins over 2 wins over
// 3 wins over 0), the rule `generate` and `lock` share over several roots.
func worstExit(a, b int) int {
	if exitRank(b) > exitRank(a) {
		return b
	}
	return a
}

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
func checkConfigDrift(cfg *config.Config, mode driftMode) (differing, blocked int, err error) {
	gen := generator.NewGenerator(cfg)
	gen.SetContext(cmdContext())
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetOverwriteUnowned(generateForce)
	if err := applyRole(gen); err != nil {
		return 0, 0, err
	}
	var (
		drift   []generator.Drift
		checked int
	)
	if mode == driftManifest {
		drift, checked, err = gen.VerifyGenerated()
	} else {
		drift, err = gen.CheckDrift(profile)
	}
	if err != nil {
		return 0, 0, err //nolint:wrapcheck // already contextual
	}
	for _, d := range drift {
		if d.Kind == generator.DriftBlocked {
			blocked++
		}
		fmt.Printf("%s: %s\n", d.Kind, displayDriftPath(cfg, d.Path))
	}
	if len(drift) == 0 && mode == driftManifest {
		progress.PrintlnIfNotQuiet(fmt.Sprintf("verified %d generated file(s) against their Content-Hash", checked))
	}
	return len(drift), blocked, nil
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
	return runDriftCheckGated(args, isRecursive, mode, nil)
}

// runDriftCheckGated is runDriftCheck with a gate run on every loaded config
// before its generated files are compared, so one load serves both. A gate error
// that is lock drift (see isLockedDrift) counts as
// drift (exit 2), any other gate error as a failure (exit 1).
func runDriftCheckGated(args []string, isRecursive bool, mode driftMode, gate func(*config.Config) error) int {
	fix := "run `ai-rulez generate` and commit the result"
	if isRecursive {
		return runRecursiveDrift(mode, fix, gate)
	}
	cfg, err := loadConfigForCommand(cmdContext(), args, driftLoadOptions(mode)...)
	if err != nil {
		fmtError(err)
		if gate != nil && errors.Is(err, config.ErrLockViolation) {
			return exitDrift // remote content disagrees with the lock: drift, not a tool failure
		}
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		return 1
	}
	gateDrift, gateErr := runGate(gate, cfg)
	if gateErr {
		return 1
	}
	applyGenerateOverrides(cfg)
	n, blocked, err := checkConfigDrift(cfg, mode)
	if err != nil {
		fmtError(err)
		return 1
	}
	if gateDrift {
		return exitDrift
	}
	return finishDrift(n, blocked, 1, fix)
}

// runGate runs the gate on cfg and reports whether it found lock drift or failed.
func runGate(gate func(*config.Config) error, cfg *config.Config) (drift, failed bool) {
	if gate == nil {
		return false, false
	}
	err := gate(cfg)
	if err == nil {
		return false, false
	}
	fmtError(err)
	if isLockedDrift(err) {
		return true, false
	}
	return false, true
}

func runRecursiveDrift(mode driftMode, fix string, gate func(*config.Config) error) int {
	paths := findConfigFilesRecursively()
	if len(paths) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}
	total, totalBlocked, failed, gateDrift := 0, 0, 0, 0
	for _, path := range paths {
		cfg, err := loadProjectFile(cmdContext(), path, driftLoadOptions(mode)...)
		if err == nil {
			err = cfg.Validate()
		}
		if err != nil {
			fmtError(oops.With("config", path).Wrapf(err, "load configuration"))
			if gate != nil && errors.Is(err, config.ErrLockViolation) {
				gateDrift++
			} else {
				failed++
			}
			continue
		}
		drift, gateFailed := runGate(gate, cfg)
		if gateFailed {
			failed++
			continue
		}
		if drift {
			gateDrift++
		}
		applyGenerateOverrides(cfg)
		n, blocked, err := checkConfigDrift(cfg, mode)
		if err != nil {
			fmtError(oops.With("config", path).Wrapf(err, "check generated files"))
			failed++
			continue
		}
		total += n
		totalBlocked += blocked
	}
	if failed > 0 {
		return 1
	}
	if total == 0 && gateDrift > 0 {
		return exitDrift
	}
	return finishDrift(total, totalBlocked, len(paths), fix)
}

func finishDrift(differing, blocked, roots int, fix string) int {
	if differing == 0 {
		logger.Success("Generated files are up to date", "roots", roots)
		return 0
	}
	fmt.Fprintln(os.Stderr, driftMessage(differing, blocked, fix))
	return exitDrift
}

// blockedRemedy is the way out for a file generate refuses to overwrite.
const blockedRemedy = "import it with `ai-rulez convert --write`, move or delete it, or pass --force"

// driftMessage is the closing line of a drift check. A blocked file is one
// generate will not overwrite, so telling the user to run generate would lead
// into the refusal: it names the remedy instead.
func driftMessage(differing, blocked int, fix string) string {
	switch {
	case blocked == 0:
		return fmt.Sprintf("%d generated file(s) differ from their sources; %s", differing, fix)
	case blocked == differing:
		return fmt.Sprintf("%d generated file(s) differ from their sources, %d blocked: generate will not overwrite a file ai-rulez did not write; %s",
			differing, blocked, blockedRemedy)
	default:
		return fmt.Sprintf("%d generated file(s) differ from their sources; %s. %d blocked: generate will not overwrite a file ai-rulez did not write; %s",
			differing, fix, blocked, blockedRemedy)
	}
}
