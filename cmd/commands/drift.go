package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
)

// Exit codes of the drift check (`generate --check`): 0 means the
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

// driftLoadOptions honors --no-local like generate does.
func driftLoadOptions() []config.LoadOption {
	return pluginLoadOptions(false)
}

// checkConfigDrift runs the drift check for one loaded config and prints the
// differing files. It returns how many differ.
func checkConfigDrift(cfg *config.Config, rep *driftReport, sink *generator.GitignoreSinks) (differing, blocked int, err error) {
	items, blocked, err := collectConfigDrift(gitutil.WithMemo(cmdContext()), cfg, sink)
	if err != nil {
		return 0, 0, err
	}
	for _, it := range items {
		rep.add(generator.DriftKind(it.Kind), it.Path, it.Note)
	}
	return len(items), blocked, nil
}

// collectConfigDrift is the comparison for one loaded config; it prints nothing,
// so recursive runs can do it on several roots at once and report in order. The
// shared sink only collects patterns (under a mutex, into a set), so the block it
// judges afterwards does not depend on the order the roots finish in.
func collectConfigDrift(ctx context.Context, cfg *config.Config, sink *generator.GitignoreSinks) (items []driftItem, blocked int, err error) {
	gen := generator.NewGenerator(cfg)
	attachRecursiveGitignoreSink(ctx, gen, cfg, sink)
	gen.SetContext(ctx)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetOverwriteUnowned(generateForce)
	if err := applyRole(gen); err != nil {
		return nil, 0, err
	}
	drift, err := gen.CheckDrift(profile)
	if err != nil {
		return nil, 0, err //nolint:wrapcheck // already contextual
	}
	for _, d := range drift {
		if d.Kind == generator.DriftBlocked {
			blocked++
		}
		items = append(items, driftItem{Kind: string(d.Kind), Path: displayDriftPath(cfg, d.Path), Note: d.Note})
	}
	// The OKF index.md files of a migrated configuration directory are rewritten by
	// generate too, so a stale one is drift (it is what `migrate okf --check` reports).
	staleIndexes, err := staleOKFIndexDrift(ctx, cfg)
	if err != nil {
		return nil, 0, err
	}
	items = append(items, staleIndexes...)
	return items, blocked, nil
}

// driftReport collects what a drift check found. The differing files are the
// command's result: they print on stdout as "kind: path" lines, or, for
// `generate --check --format json`, as one document written when the check ends.
type driftReport struct {
	out   render.Out
	json  bool
	items []driftItem
}

type driftItem struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

// driftDocument is the `generate --check --format json` document.
type driftDocument struct {
	Status    string      `json:"status"`
	Roots     int         `json:"roots"`
	Blocked   int         `json:"blocked"`
	Differing []driftItem `json:"differing"`
}

func newDriftReport() *driftReport {
	return &driftReport{out: defaultOut(), json: defaultOut().JSON()}
}

// writeJSON writes the --format json document; it does nothing for text.
func (r *driftReport) writeJSON(status string, roots, blocked int) {
	if !r.json {
		return
	}
	items := r.items
	if items == nil {
		items = []driftItem{}
	}
	_ = jsondoc.Write(r.out.Stdout(), driftDocument{Status: status, Roots: roots, Blocked: blocked, Differing: items}) //nolint:errcheck // stdout write failure has nowhere to be reported
}

func (r *driftReport) add(kind generator.DriftKind, path, note string) {
	if r.json {
		r.items = append(r.items, driftItem{Kind: string(kind), Path: path, Note: note})
		return
	}
	if note != "" {
		r.out.Result("%s: %s (%s)\n", kind, path, note)
		return
	}
	r.out.Result("%s: %s\n", kind, path)
}

// displayDriftPath shows a project-relative path relative to the working
// directory, so a nested root reads as svc/CLAUDE.md.
func displayDriftPath(cfg *config.Config, rel string) string {
	return displayDriftAbs(filepath.Join(cfg.BaseDir, filepath.FromSlash(rel)), rel)
}

// displayDriftAbs shows abs relative to the working directory, or fallback when
// no relative form exists.
func displayDriftAbs(abs, fallback string) string {
	abs, err := filepath.Abs(abs)
	if err != nil {
		return fallback
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fallback
	}
	out, err := filepath.Rel(realPath(cwd), realPath(abs))
	if err != nil {
		return fallback
	}
	return filepath.ToSlash(out)
}

// realPath resolves symlinks (and, on Windows, 8.3 short names) in the longest
// existing prefix of p, so two spellings of one directory compare equal.
func realPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	dir, base := filepath.Split(filepath.Clean(p))
	if dir == "" || dir == p {
		return p
	}
	return filepath.Join(realPath(filepath.Clean(dir)), base)
}

// runDriftCheck checks a single root (-C/--config-dir or discovery) or every root under the working
// directory (recursive) and returns the process exit code.
func runDriftCheck(isRecursive bool) int {
	return runDriftCheckGated(isRecursive, nil)
}

// runDriftCheckGated is runDriftCheck with a gate run on every loaded config
// before its generated files are compared, so one load serves both. A gate error
// that is lock drift (see isLockedDrift) counts as
// drift (exit 2), any other gate error as a failure (exit 1).
func runDriftCheckGated(isRecursive bool, gate func(*config.Config) error) int {
	fix := "run `ai-rulez generate` and commit the result"
	rep := newDriftReport()
	if isRecursive {
		return runRecursiveDrift(rep, fix, gate)
	}
	cfg, err := loadConfigForCommand(cmdContext(), driftLoadOptions()...)
	if err != nil {
		renderError(os.Stderr, err)
		if gate != nil && errors.Is(err, config.ErrLockViolation) {
			return exitDrift // remote content disagrees with the lock: drift, not a tool failure
		}
		return 1
	}
	if err := cfg.Validate(); err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	warnLegacyLayout(cfg.ConfigDir)
	gateDrift, gateErr := runGate(gate, cfg)
	if gateErr {
		return 1
	}
	if err := applyGenerateOverrides(cmdContext(), cfg); err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	n, blocked, err := checkConfigDrift(cfg, rep, nil)
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	if gateDrift {
		rep.writeJSON(statusDrift, 1, blocked)
		return exitDrift
	}
	return finishDrift(rep, n, blocked, 1, fix)
}

// runGate runs the gate on cfg and reports whether it found lock drift or failed.
func runGate(gate func(*config.Config) error, cfg *config.Config) (drift, failed bool) {
	drift, failed, err := evalGate(gate, cfg)
	if err != nil {
		renderError(os.Stderr, err)
	}
	return drift, failed
}

// evalGate runs the gate and classifies its error without printing it.
func evalGate(gate func(*config.Config) error, cfg *config.Config) (drift, failed bool, err error) {
	if gate == nil {
		return false, false, nil
	}
	err = gate(cfg)
	if err == nil {
		return false, false, nil
	}
	if isLockedDrift(err) {
		return true, false, err
	}
	return false, true, err
}

// driftRoot is what checking one root found. The check runs concurrently with
// the other roots and prints nothing; the run reports it, in root order, from
// this record.
type driftRoot struct {
	configDir string  // set once the config loaded, for the legacy-layout notice
	errs      []error // printed to stderr, in order
	failed    bool
	gateDrift bool
	items     []driftItem
	blocked   int
	notes     *rootNotes // what the root's work logged, printed in root order
}

// checkDriftRoot loads one root, runs the gate and compares its generated files.
func checkDriftRoot(ctx context.Context, path string, gate func(*config.Config) error, sink *generator.GitignoreSinks) driftRoot {
	var r driftRoot
	ctx, r.notes = withRootNotes(ctx)
	cfg, err := loadProjectFile(ctx, path, driftLoadOptions()...)
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		r.errs = append(r.errs, oops.With("config", path).Wrapf(err, "load configuration"))
		if gate != nil && errors.Is(err, config.ErrLockViolation) {
			r.gateDrift = true
		} else {
			r.failed = true
		}
		return r
	}
	r.configDir = cfg.ConfigDir
	if cfg.IsPluginAuthoringRoot() {
		// generate --check never covers bundles; verify --plugin does.
		logger.Info("Skipping a plugin-authoring root: check its bundle with `ai-rulez verify --plugin`", "config", path)
		return r
	}
	gateDrift, gateFailed, gateErr := evalGate(gate, cfg)
	if gateErr != nil {
		r.errs = append(r.errs, gateErr)
	}
	if gateFailed {
		r.failed = true
		return r
	}
	r.gateDrift = gateDrift
	if err := applyGenerateOverrides(ctx, cfg); err != nil {
		r.errs = append(r.errs, oops.With("config", path).Wrapf(err, "check generated files"))
		r.failed = true
		return r
	}
	r.items, r.blocked, err = collectConfigDrift(ctx, cfg, sink)
	if err != nil {
		r.errs = append(r.errs, oops.With("config", path).Wrapf(err, "check generated files"))
		r.failed, r.items, r.blocked = true, nil, 0
	}
	return r
}

func runRecursiveDrift(rep *driftReport, fix string, gate func(*config.Config) error) int {
	paths := findConfigFilesRecursively()
	if len(paths) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}
	// One memo for every root: the roots of a monorepo ask git the same questions,
	// and a read-only check can share one listing of the index between them.
	ctx := gitutil.WithSharedIndex(cmdContext())
	total, totalBlocked, failed, gateDrift := 0, 0, 0, 0
	// The roots share one managed block at the repository toplevel; each adds its
	// patterns here and the block is judged once, after the last root.
	sink := generator.NewGitignoreSinks()
	forEachRoot(ctx, paths, func(path string) driftRoot { return checkDriftRoot(ctx, path, gate, sink) },
		func(i int, path string, r driftRoot, elapsed time.Duration) {
			r.notes.flush(logger.Std(), os.Stderr)
			if r.configDir != "" {
				warnLegacyLayout(r.configDir)
			}
			for _, e := range r.errs {
				renderError(os.Stderr, e)
			}
			for _, it := range r.items {
				rep.add(generator.DriftKind(it.Kind), it.Path, it.Note)
			}
			total += len(r.items)
			totalBlocked += r.blocked
			if r.failed {
				failed++
			}
			if r.gateDrift {
				gateDrift++
			}
			reportRootProgress(i, len(paths), path, elapsed)
		})
	if err := ctx.Err(); err != nil {
		renderError(os.Stderr, oops.Wrapf(err, "check interrupted before every root was read"))
		return 1
	}
	if failed > 0 {
		return 1
	}
	// A root that failed left its patterns out, so the block is only judged when
	// every root was read (generate rewrites it under the same condition).
	for _, sd := range sink.Drift(logger.Std()) {
		d := sd.Drift
		rep.add(d.Kind, displayDriftAbs(filepath.Join(sd.TopLevel, filepath.FromSlash(d.Path)), d.Path), d.Note)
		total++
	}
	if total == 0 && gateDrift > 0 {
		rep.writeJSON(statusDrift, len(paths), totalBlocked)
		return exitDrift
	}
	return finishDrift(rep, total, totalBlocked, len(paths), fix)
}

func finishDrift(rep *driftReport, differing, blocked, roots int, fix string) int {
	status := "ok"
	if differing > 0 {
		status = statusDrift
	}
	rep.writeJSON(status, roots, blocked)
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
	switch blocked {
	case 0:
		return fmt.Sprintf("%d generated file(s) differ from their sources; %s", differing, fix)
	case differing:
		return fmt.Sprintf("%d generated file(s) differ from their sources, %d blocked: generate will not overwrite a file ai-rulez did not write; %s",
			differing, blocked, blockedRemedy)
	default:
		return fmt.Sprintf("%d generated file(s) differ from their sources; %s. %d blocked: generate will not overwrite a file ai-rulez did not write; %s",
			differing, fix, blocked, blockedRemedy)
	}
}
