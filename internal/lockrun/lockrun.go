// Package lockrun writes ai-rulez.lock for one project: it resolves the remote
// sources, pins the authored content and outputs, and runs the gates every
// writer of the lock must run before anything is written: the minimum release
// age of version ranges, the security scan of every newly pinned remote tree
// (AR001-AR009), the security scan of served skills, the deny list (AR717) and
// the carry-over of approvals. `ai-rulez lock` and the pkg/airulez facade are
// thin callers of Write; neither has a copy of these gates.
package lockrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// Exit codes of a lock run (see ExitCode).
const (
	ExitOK = 0
	// ExitFailed: the run could not complete (a tool error).
	ExitFailed = 1
	// ExitFindings: the security scan refused a tree or a served skill; nothing was written.
	ExitFindings = 2
	// ExitUnpinned: the lock was written, but served skills the security scan
	// refuses were left unpinned (without Strict).
	ExitUnpinned = 3
)

// Request is what one lock run pins.
type Request struct {
	// Kind limits the refresh to one entry kind (include, skill, source, served).
	Kind string
	// Names limit the refresh to these remote sources or served skills; the
	// content pins are then kept as they are.
	Names []string
	// ContentOnly re-pins the authored content and outputs only, offline: the
	// remote pins are kept.
	ContentOnly bool
	// Profile is the profile whose outputs are pinned ("" = the one the lock
	// recorded, else the configured default).
	Profile string
	// AllRoles also pins the rendered outputs of every role; Roles names roles
	// whose outputs are pinned too.
	AllRoles bool
	Roles    []string
	// AcceptFindings pins a remote tree whose security scan has error findings.
	AcceptFindings bool
	// Strict fails the run when the security scan refuses a served skill,
	// instead of leaving it unpinned.
	Strict bool
	// Extras are serve views pinned besides the default one and the roles.
	Extras []mcp.ServeSetup
}

// full reports a refresh of everything: no kind and no names.
func (r *Request) full() bool { return len(r.Names) == 0 && r.Kind == "" }

func (r *Request) wanted() map[string]bool {
	wanted := map[string]bool{}
	for _, n := range r.Names {
		wanted[n] = true
	}
	return wanted
}

// Env is what a lock run needs from its caller.
type Env struct {
	// Version is the ai-rulez version recorded in the lock.
	Version string
	// Policy is the base lock policy of the run's loads; the run adds the
	// refresh (or offline) mode and the release age gate.
	Policy config.LockPolicy
	// Load loads the project under policy, with opts.
	Load func(ctx context.Context, policy config.LockPolicy, opts ...config.LoadOption) (*config.Config, error)
	// Collector keeps the warnings of the serve views' loads (nil: their own).
	Collector *diag.Collector
	// LoadOptions are added to the serve views' loads, which re-read the project.
	LoadOptions []config.LoadOption
	// Report receives the findings of the pre-pin scan as text; nil drops them
	// (they are also in the FindingsError and the Result).
	Report io.Writer
	// NewForge builds the forge client of the release age gate (nil: DefaultForge).
	NewForge ForgeFunc
	// GitToken authenticates the git lookups of the release age gate.
	GitToken string
	// Tree indexes the repository for the [[scan]] records (nil: lint.LoadTree).
	Tree func(base string) (*lint.Tree, error)
	// Cwd is the directory scan records show paths relative to.
	Cwd string
}

// Result is what a lock run did. Write returns it even with an error: Unpinned
// and Scans say what the run found before it stopped.
type Result struct {
	// Lock is the written lock; nil when nothing was written.
	Lock *lockfile.File
	// Path is the lock file written.
	Path string
	// Unpinned are the served skills left unpinned because the security scan
	// refuses them (without Strict).
	Unpinned []mcp.Refusal
	// Scans are the pre-pin scans with error findings: refused, or accepted
	// with AcceptFindings.
	Scans []PinScan
}

// ErrScanRefused marks a lock refused only because the security scan refuses
// served skills under Strict.
var ErrScanRefused = errors.New("the security scan refuses served skills (--strict)")

// Finding is one thing the security scan refused.
type Finding struct {
	// Kind is the lock entry kind: include, skill, source or served.
	Kind string
	Name string
	// View is the serve view of a served skill ("" for the default view).
	View string
	// Code is the strict-validation code (AR001..).
	Code     string
	Severity string
	File     string
	Line     int
	Message  string
}

// FindingsError is a run the security scan refused: findings (ExitFindings),
// not a failure to run. Nothing was written.
type FindingsError struct {
	// Sources is how many remote trees the pre-pin scan refused; 0 when the
	// refusal is of served skills under Strict.
	Sources  int
	Findings []Finding
	err      error
}

func (e *FindingsError) Error() string { return e.err.Error() }
func (e *FindingsError) Unwrap() error { return e.err }

// ExitCode classifies the outcome of Write.
func ExitCode(res *Result, err error) int {
	var fe *FindingsError
	switch {
	case errors.As(err, &fe):
		return ExitFindings
	case err != nil:
		return ExitFailed
	case res != nil && len(res.Unpinned) > 0:
		return ExitUnpinned
	}
	return ExitOK
}

// RunPolicy is the lock policy of a run's loads: base, re-resolving the remote
// sources kind and wanted select (remoteRefresh), or offline (a content-only
// run, which keeps the remote pins).
func RunPolicy(base config.LockPolicy, remoteRefresh bool, kind string, wanted map[string]bool) config.LockPolicy {
	p := base
	if remoteRefresh {
		p.Mode = config.LockRefresh
		p.Refresh = func(k, n string) bool {
			return (kind == "" || kind == k) && (len(wanted) == 0 || wanted[n])
		}
	} else {
		p.Offline = true
	}
	return p
}

// Write refreshes and writes the lock of the project env.Load loads.
func Write(ctx context.Context, req Request, env Env) (*Result, error) {
	res := &Result{}
	if env.Report == nil {
		env.Report = io.Discard
	}
	if env.Tree == nil {
		env.Tree = lint.LoadTree
	}
	// lock reports an include it cannot resolve as a problem of its own.
	ctx = config.WithUnresolvedIncludesTolerated(ctx)
	wanted := req.wanted()
	remoteRefresh := !req.ContentOnly
	policy := env.Policy
	if remoteRefresh {
		// min_release_age holds back young tags while ranges resolve.
		policy.ReleaseGate = releaseGate(ctx, env)
		includes.ResetObserved()
	}
	policy = RunPolicy(policy, remoteRefresh, req.Kind, wanted)

	cfg, err := env.Load(ctx, policy, config.WithoutLocal())
	if err != nil {
		return res, err
	}
	// Pin only what generate would accept: a lock for a configuration that
	// fails validation would record content no run can use.
	if err := cfg.Validate(); err != nil {
		return res, err //nolint:wrapcheck // already contextual
	}
	current, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return res, err //nolint:wrapcheck // already contextual
	}
	next, dyn, err := Next(ctx, cfg, current, &req, &env, remoteRefresh)
	res.Unpinned = dyn.Unpinned
	if err != nil {
		return res, err
	}
	if err := PinContent(cfg, current, next, &req, env.Version); err != nil { //nolint:contextcheck // the generator renders without a context (as in govview.CheckLockRoles)
		return res, err
	}
	CarryApprovals(cfg.Log(), current, next, req.full())
	if err := DeniedPinsError(next); err != nil {
		return res, err
	}
	scans, refused := scanNewPins(cfg, current, next, req.AcceptFindings, env.Report)
	res.Scans = scans
	if refused > 0 {
		return res, prePinRefusal(scans, refused)
	}
	PinScans(cfg, current, next, req.full(), env.Tree, env.Cwd)
	if err := lockfile.Save(cfg.ConfigDir, next); err != nil {
		return res, err //nolint:wrapcheck // already contextual
	}
	res.Lock, res.Path = next, lockfile.Path(cfg.ConfigDir)
	return res, nil
}

// releaseGate reads [lock] from a load that fetches nothing (the sources are
// resolved by the real load) and returns the release age gate of the run; nil
// when that load fails, which the real load then reports.
func releaseGate(ctx context.Context, env Env) func(lockfile.Want) *tagresolve.AgeGate {
	cfg, err := env.Load(ctx, env.Policy, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return nil
	}
	return NewAgeGates(cfg, env.NewForge, env.Policy.Offline, env.GitToken).GateFor
}

// prePinRefusal is the error of a run whose pre-pin scan refused remote trees.
func prePinRefusal(scans []PinScan, refused int) error {
	fe := &FindingsError{Sources: refused, err: fmt.Errorf("refused %d source(s): the security scan of the new tree has error findings; review them, then pass --accept-findings. Nothing was written", refused)}
	for _, s := range scans {
		if !s.Summary.Refused {
			continue
		}
		for _, f := range s.Summary.Findings {
			fe.Findings = append(fe.Findings, Finding{Kind: s.Kind, Name: s.Name, Code: f.Code, Severity: f.Severity, File: f.File, Line: f.Line, Message: f.Message})
		}
	}
	return fe
}

// Next builds the remote, source and served entries of the new lock. The
// content pins are added by PinContent. The DynamicResult says which served
// skills were left unpinned.
func Next(ctx context.Context, cfg *config.Config, current *lockfile.File, req *Request, env *Env, remoteRefresh bool) (*lockfile.File, DynamicResult, error) {
	wanted := req.wanted()
	next := &lockfile.File{Version: lockfile.Version}
	if remoteRefresh {
		var problems []string
		next, problems = includes.BuildLock(cfg, current)
		if len(problems) > 0 {
			return nil, DynamicResult{}, oops.With("config", cfg.ConfigDir).Errorf("cannot write %s:\n  %s", lockfile.FileName, strings.Join(problems, "\n  "))
		}
	} else if current != nil {
		next.Include, next.Skill = current.Include, current.Skill
	}

	// Source and served pins: refreshed with the remote pins; a content-only run
	// refreshes only the served digests, and only when that works offline.
	dynamicKind := req.Kind
	if !remoteRefresh {
		dynamicKind = lockfile.KindServed
	}
	dyn := MergeDynamic(ctx, cfg, current, next, DynamicRun{Kind: dynamicKind, Wanted: wanted, Extras: req.Extras, Strict: req.Strict,
		Version: env.Version, Collector: env.Collector, LoadOptions: env.LoadOptions})
	if problems := dyn.Problems; len(problems) > 0 {
		if dyn.ScanOnly() {
			return nil, dyn, servedRefusal(cfg, dyn)
		}
		if remoteRefresh {
			return nil, dyn, oops.With("config", cfg.ConfigDir).Errorf("cannot write %s:\n  %s", lockfile.FileName, strings.Join(problems, "\n  "))
		}
		cfg.Log().Warn("Kept the served pins: they cannot be recomputed offline", "problems", strings.Join(problems, "; "))
	}
	for name := range wanted {
		if !HasName(next, name) {
			return nil, dyn, oops.Errorf("%q is not a remote include, installed skill, skill source or served skill in %s", name, cfg.ConfigDir)
		}
	}
	return next, dyn, nil
}

// servedRefusal is the error of a Strict run whose only problems are served
// skills the security scan refuses.
func servedRefusal(cfg *config.Config, dyn DynamicResult) error {
	fe := &FindingsError{err: oops.With("config", cfg.ConfigDir).Wrapf(ErrScanRefused, "cannot write %s:\n  %s", lockfile.FileName, strings.Join(dyn.Problems, "\n  "))}
	for _, r := range dyn.Refusals {
		fe.Findings = append(fe.Findings, Finding{Kind: lockfile.KindServed, Name: r.Name, View: r.View, Code: r.Code, Severity: string(lint.SeverityError), Message: r.Reason})
	}
	return fe
}

// PinContent adds the content pins to next. A refresh limited to some remote
// sources leaves the content pins alone; otherwise they are recomputed from the
// sources on disk.
func PinContent(cfg *config.Config, current, next *lockfile.File, req *Request, version string) error {
	if req.full() {
		profileName := req.Profile
		if profileName == "" && current != nil {
			profileName = current.Profile
		}
		snap, err := govview.SnapshotRoles(cfg, profileName, false, version, govview.RoleSelection{Write: true, All: req.AllRoles, Lock: current, Only: req.Roles})
		if err != nil {
			return err //nolint:wrapcheck // already contextual
		}
		for _, problem := range snap.Problems {
			cfg.Log().Warn("Cannot pin part of the configuration; lock --check will fail until it is fixed", "problem", problem)
		}
		contentlock.Build(next, snap)
		return nil
	}
	if current == nil {
		return nil
	}
	next.AIRulezVersion, next.Profile = current.AIRulezVersion, current.Profile
	next.Scope, next.OutputsPinned = current.Scope, current.OutputsPinned
	next.Item, next.Output = current.Item, current.Output
	if next.HasContentPins() {
		next.Tree = contentlock.TreeOf(next)
	}
	return nil
}
