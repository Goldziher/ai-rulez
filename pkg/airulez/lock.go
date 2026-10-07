package airulez

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// LockOptions selects what Lock pins. The zero value pins everything, as
// `ai-rulez lock` does.
//
// Experimental.
type LockOptions struct {
	// Names limit the refresh to these remote includes, installed skills, skill
	// sources or served skills; the content pins are then kept as they are.
	Names []string
	// Kind limits the refresh to one entry kind: "include", "skill", "source"
	// or "served".
	Kind string
	// ContentOnly re-pins the authored content and outputs only, without the
	// network: the remote pins are kept.
	ContentOnly bool
	// Profile is the profile whose outputs are pinned; "" is the one the lock
	// recorded, else the configured default.
	Profile string
	// Roles also pins the rendered outputs of every role.
	Roles bool
	// AcceptFindings pins a remote tree whose security scan has error findings
	// (review them first). Without it such a tree fails Lock with CodeFindings.
	AcceptFindings bool
	// Strict fails Lock with CodeFindings when the security scan refuses a
	// served skill; by default the skill is left unpinned (LockResult.Unpinned)
	// and the rest is pinned.
	Strict bool
	// ToolVersion is the ai-rulez version recorded in the lock; "" records none.
	ToolVersion string
}

// LockFinding is one thing the security scan refused while locking.
//
// Experimental.
type LockFinding struct {
	// Kind is the lock entry kind: "include", "skill", "source" or "served".
	Kind string
	Name string
	// View is the serve view of a served skill ("" for the default view).
	View string
	// Code is the stable AR code of the finding.
	Code     string
	Severity string
	File     string
	Line     int
	Message  string
}

// FindingsError is the cause of a CodeFindings error.
//
// Experimental.
type FindingsError struct {
	Findings []LockFinding
	err      error
}

func (e *FindingsError) Error() string { return e.err.Error() }
func (e *FindingsError) Unwrap() error { return e.err }

// LockResult is what Lock wrote.
//
// Experimental.
type LockResult struct {
	// Path is the lock file, slash-separated and relative to the project root.
	Path string
	// Tree is the digest over every content pin ("" when nothing is pinned).
	Tree string
	// Unpinned are the served skills the security scan refuses, left out of the
	// lock (not with LockOptions.Strict, which fails instead).
	Unpinned []LockFinding
}

// Lock refreshes and writes ai-rulez.lock, as `ai-rulez lock` does and with the
// same gates: the minimum release age of version ranges, the security scan of
// every remote tree about to be pinned and of the served skills, the deny list
// and the carry-over of approvals. It needs a directory workspace (the lock is a
// file of it). Remote sources are re-resolved through Options.Runner when the
// Project was loaded with Options.Remote; release times are then also asked of
// the forge over HTTPS. The Project keeps the configuration it was loaded with:
// Load again to plan against the new lock.
//
// Experimental.
func (p *Project) Lock(ctx context.Context, o LockOptions) (*LockResult, error) {
	if !p.disk {
		return nil, &Error{Code: CodeDiskRequired, Err: oops.Wrapf(ErrDiskRequired, "the lock is a file of the project directory")}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	req := lockrun.Request{Kind: o.Kind, Names: o.Names, ContentOnly: o.ContentOnly, Profile: o.Profile, AllRoles: o.Roles,
		AcceptFindings: o.AcceptFindings, Strict: o.Strict}
	if o.Kind != "" && !lockrun.KnownKind(o.Kind) {
		return nil, &Error{Code: CodeLock, Err: oops.Errorf("unknown kind %q (use include, skill, source or served)", o.Kind)}
	}
	env := lockrun.Env{
		Version:     o.ToolVersion,
		Load:        p.loadUnder,
		LoadOptions: p.opts,
		NewForge: func(cfg *config.Config, offline bool) forge.Client {
			return lockrun.DefaultForge(cfg, offline || !p.remote)
		},
		GitToken: p.token,
		Tree: func(base string) (*lint.Tree, error) { //nolint:contextcheck // internal/lint has no context-taking LoadTree (as in Validate)
			return lint.LoadTreeWith(p.git, base, "")
		},
	}
	res, err := lockrun.Write(runner.WithContext(ctx, p.run), req, env)
	if err != nil {
		var fe *lockrun.FindingsError
		if errors.As(err, &fe) {
			return nil, &Error{Code: CodeFindings, Err: &FindingsError{Findings: lockFindings(fe.Findings), err: err}}
		}
		return nil, &Error{Code: CodeLock, Err: err}
	}
	out := &LockResult{Path: p.rel(res.Path), Tree: res.Lock.Tree}
	for _, r := range res.Unpinned {
		out.Unpinned = append(out.Unpinned, LockFinding{Kind: lockfile.KindServed, Name: r.Name, View: r.View, Code: r.Code, Severity: string(lint.SeverityError), Message: r.Reason})
	}
	return out, nil
}

// loadUnder reloads the project as it was loaded, under policy.
func (p *Project) loadUnder(ctx context.Context, policy config.LockPolicy, opts ...config.LoadOption) (*config.Config, error) {
	all := append(append(append([]config.LoadOption(nil), p.opts...), opts...), config.WithLockPolicy(policy))
	return config.LoadConfig(ctx, ".", all...) //nolint:wrapcheck // classified by the caller
}

// rel is path relative to the project root, slash-separated.
func (p *Project) rel(path string) string {
	if rel, err := filepath.Rel(p.cfg.BaseDir, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func lockFindings(in []lockrun.Finding) []LockFinding {
	out := make([]LockFinding, 0, len(in))
	for i := range in {
		f := &in[i]
		out = append(out, LockFinding{Kind: f.Kind, Name: f.Name, View: f.View, Code: f.Code, Severity: f.Severity, File: f.File, Line: f.Line, Message: f.Message})
	}
	return out
}

// LockCheckOptions selects what LockCheck compares.
//
// Experimental.
type LockCheckOptions struct {
	// Profile is the profile whose outputs are compared; "" is the one the lock recorded.
	Profile string
	// ToolVersion is the ai-rulez version reported as running; "" compares none.
	ToolVersion string
}

// LockStatus is the result of LockCheck.
//
// Experimental.
type LockStatus struct {
	// InSync is true when the lock matches the sources, the outputs and the cache.
	InSync bool
	// Changes describe each difference, one line each.
	Changes []string
	// Notes are differences that do not fail the check (a tool version change).
	Notes []string
}

// LockCheck compares ai-rulez.lock with the sources, the outputs and the local
// cache without the network, as `ai-rulez lock --check` does (the remote tag
// and signature checks of that command are not part of it). Remote content the
// lock pins is read from the cache when the Project was loaded with
// Options.Remote. It needs a directory workspace; a project without a lock (and
// without [lock] enforce) fails with CodeLock.
//
// Experimental.
func (p *Project) LockCheck(ctx context.Context, o LockCheckOptions) (*LockStatus, error) {
	if !p.disk {
		return nil, &Error{Code: CodeDiskRequired, Err: oops.Wrapf(ErrDiskRequired, "the lock is a file of the project directory")}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx = config.WithUnresolvedIncludesTolerated(runner.WithContext(ctx, p.run))
	cfg, remoteSkipped, err := govview.LoadWithCacheFallback(func(opts ...config.LoadOption) (*config.Config, error) {
		return p.loadUnder(ctx, config.LockPolicy{Offline: true}, append([]config.LoadOption{config.WithoutLocal()}, opts...)...)
	})
	if err != nil {
		if errors.Is(err, config.ErrLockViolation) {
			// Cached remote content disagrees with the lock: drift, not a failure to run.
			return &LockStatus{Changes: []string{err.Error()}}, nil
		}
		return nil, &Error{Code: CodeLock, Err: err}
	}
	if lock, loadErr := lockfile.Load(cfg.ConfigDir); loadErr == nil && lock == nil && !cfg.LockEnforced() {
		return nil, &Error{Code: CodeLock, Err: oops.Errorf("no %s in %s: nothing to check", lockfile.FileName, p.rel(cfg.ConfigDir))}
	}
	dynamic := func(cfg *config.Config, lock *lockfile.File) []contentlock.Change {
		if lockrun.ServesNothing(cfg, lock, nil) {
			return nil
		}
		return mcp.DynamicLockChangesWith(config.WithOfflineIncludes(ctx), cfg, lock, o.ToolVersion, p.opts)
	}
	diff, err := govview.CheckLockRoles(ctx, cfg, remoteSkipped, o.Profile, o.ToolVersion, dynamic, govview.RoleSelection{})
	if err != nil {
		return nil, &Error{Code: CodeLock, Err: err}
	}
	status := &LockStatus{InSync: diff.InSync, Notes: diff.Notes}
	for i := range diff.Changes {
		status.Changes = append(status.Changes, diff.Changes[i].Line())
	}
	return status, nil
}
