// Package worktreeprovision provisions a linked git worktree with a checkout's
// AI-Rulez configuration. A fresh linked worktree inherits the tracked
// configuration from its branch, but the checkout-local AI-Rulez inputs stay
// behind: the machine-local overlay, the local content tree and the
// hand-authored `.local`/`.override` instruction and settings files a harness
// reads beside the generated ones.
//
// The service never invents a branch: `create` makes the linked worktree the
// caller names, and `setup` provisions one that already exists (made by git or
// by the caller's host). Local copying is explicit opt-in through CopyLocal
// plus an allowlist; extra files are carried only when the caller names them
// one by one. Generated outputs are regenerated from the destination's own
// committed configuration, never copied, and no file content is ever reported
// across the two checkouts.
package worktreeprovision

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/samber/oops"
)

// Options configure one provisioning run. The commands run it from the source
// checkout, so Source is normally the current directory.
type Options struct {
	// Source is the project root to provision from: the checkout that owns the
	// configuration and the machine-local inputs.
	Source string
	// Destination is the linked worktree to provision. Create makes it first;
	// setup requires it to exist as a registered linked worktree.
	Destination string
	// CopyLocal carries the machine-local inputs the source checkout holds,
	// through the allowlist below. Off by default: consent is explicit.
	CopyLocal bool
	// CopyExtra are extra source-relative files or directories to carry on top
	// of the allowlist, named one by one. Never a bulk copy.
	CopyExtra []string
	// DryRun plans the run and writes nothing, including a generate dry run.
	DryRun bool
	// Out receives the human-readable plan lines; nil discards them.
	Out io.Writer
	// Git runs the git commands; the zero value runs real git.
	Git gitutil.Git
	// Generate regenerates the destination's outputs from its own committed
	// configuration and returns the lines it printed. The caller chooses how:
	// the CLI spawns itself, the MCP server generates in process. With
	// dryRun the function must not write. Nil makes provisioning refuse.
	Generate func(ctx context.Context, projectDir string, dryRun bool) ([]string, error)
}

// CopyEntry is one planned or completed copy: source-relative path,
// destination-relative path and the action taken (copied or identical).
type CopyEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Action      string `json:"action"`
}

// Result says what a provisioning run did or would do. Paths are relative to
// the source and destination checkouts, so no absolute path and no file
// content leaves the machine. The JSON form is schema/worktree-provision.schema.json.
type Result struct {
	Source      string      `json:"source"`
	Destination string      `json:"destination"`
	DryRun      bool        `json:"dry_run,omitempty"`
	Copies      []CopyEntry `json:"copies"`
	Generated   bool        `json:"generated,omitempty"`
	Plan        []string    `json:"plan,omitempty"`
	Conflicts   []CopyEntry `json:"conflicts,omitempty"`
}

// Actions of a planned copy.
const (
	ActionCopy      = "copy"
	ActionIdentical = "identical"
)

// Create provisions a linked worktree that does not exist yet: it makes the
// worktree at Destination on a new branch started from startRef (the source
// checkout's HEAD when empty), then provisions it like Setup. It refuses an
// existing branch or a destination that git cannot create. A dry run writes
// nothing: no branch, no worktree, no registration — it checks what it can
// without mutation and reports what it would carry.
func Create(ctx context.Context, opts *Options, branch, startRef string) (*Result, error) {
	if err := validateBranchName(branch); err != nil {
		return nil, err
	}
	w, err := resolveSource(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return dryRunCreate(ctx, w, opts, branch, startRef)
	}
	if err := addWorktree(ctx, w, branch, startRef); err != nil {
		return nil, err
	}
	if err := checkDestinationWorktree(ctx, w); err != nil {
		return nil, err
	}
	return provision(ctx, w, opts)
}

// dryRunCreate checks a create without mutating anything: the branch must be
// new, the start point a commit, and the destination path free. The planned
// copies are checked against the repository's ignore rules; the generate step
// is not previewed, because the destination checkout does not exist yet.
func dryRunCreate(ctx context.Context, w *worktrees, opts *Options, branch, startRef string) (*Result, error) {
	if startRef == "" {
		startRef = "HEAD"
	}
	if _, err := w.git.CommitOf(ctx, w.top, startRef); err != nil {
		return nil, oops.Hint("pass a branch or commit that exists in this repository").
			Errorf("the start point %q is not a commit of this repository: %v", startRef, err)
	}
	if w.git.BranchExists(ctx, w.top, branch) {
		return nil, oops.Hint("choose another name, or delete the branch first").
			Errorf("the branch %s already exists", branch)
	}
	if _, err := os.Lstat(w.dst); err == nil {
		return nil, oops.Hint("the destination path must be free; git would refuse to create the worktree").
			Errorf("the destination %s already exists", w.dst)
	}
	w.dstState = w.srcState
	w.checkDir = w.top
	files, err := planCopies(ctx, w, opts)
	if err != nil {
		return nil, err
	}
	res := &Result{Source: w.src, Destination: projectAt(w.dst, w.relProject), DryRun: true, Copies: []CopyEntry{}}
	for _, f := range files {
		res.Copies = append(res.Copies, CopyEntry{Source: f.rel, Destination: f.rel, Action: actionCopy})
	}
	res.Plan = []string{"Dry run: the branch " + branch + " and the worktree are not created; " +
		"run without --dry-run to create and provision them. The generate step is not previewed, " +
		"because the destination checkout does not exist yet."}
	writePlan(opts.Out, w, res)
	return res, nil
}

// validateBranchName refuses a branch name git would take as an option, and
// one whose name gitutil cannot verify.
func validateBranchName(branch string) error {
	if strings.TrimSpace(branch) == "" {
		return oops.Hint("name the branch the linked worktree checks out").
			Errorf("no branch")
	}
	if err := gitutil.CheckArg("branch", branch); err != nil {
		return oops.Hint("use a plain branch name: letters, digits, dots, dashes and slashes").
			Errorf("the branch name %q is not valid: %v", branch, err)
	}
	return nil
}

// addWorktree makes the linked worktree: the start point defaults to the
// source checkout's HEAD, the branch must be new, and the path must be free.
func addWorktree(ctx context.Context, w *worktrees, branch, startRef string) error {
	if startRef == "" {
		startRef = "HEAD"
	}
	if _, err := w.git.CommitOf(ctx, w.top, startRef); err != nil {
		return oops.Hint("pass a branch or commit that exists in this repository").
			Errorf("the start point %q is not a commit of this repository: %v", startRef, err)
	}
	if w.git.BranchExists(ctx, w.top, branch) {
		return oops.Hint("choose another name, or delete the branch first").
			Errorf("the branch %s already exists", branch)
	}
	if err := w.git.WorktreeAdd(ctx, w.top, w.dst, branch, startRef); err != nil {
		return oops.Hint("check that the destination path is free and git can write it").
			Errorf("create the linked worktree: %v", err)
	}
	return nil
}

// Setup provisions a linked worktree that already exists: made by git itself
// or by the caller's host, it still shares the source's git common directory.
func Setup(ctx context.Context, opts *Options) (*Result, error) {
	w, err := resolveSource(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := checkDestinationWorktree(ctx, w); err != nil {
		return nil, err
	}
	return provision(ctx, w, opts)
}

// worktrees is a resolved run: the repository and both checkouts, with the
// layouts their configuration resolves to. The state dirs are precomputed
// relative to their project roots, which is the only thing a provisioning run
// needs from a layout: where the overlay and the local content tree live.
type worktrees struct {
	git gitutil.Git
	top string
	// src is the source project; dst is the linked worktree root, dstProj the
	// project inside it (the same repository-relative path src has below top).
	src, dst, dstProj string
	relProject        string
	srcState          StateDirs
	dstState          StateDirs
	// checkDir is the directory the destination ignore rules are checked in:
	// the destination when it exists, the repository top for a dry-run create
	// (whose destination checkout does not exist yet; the branch carries the
	// same committed rules).
	checkDir string
}

// StateDirs is the part of a project's layout provisioning reads: where the
// machine-local state (config.local.toml and local/) lives, relative to the
// project root.
type StateDirs struct {
	StateDirRel string
}

// stateDirsOf resolves the layout at root and returns its state directory
// relative to root.
func stateDirsOf(root string) (StateDirs, error) {
	name := config.ResolveConfigDirName(root)
	if name == "" {
		return StateDirs{}, oops.Errorf("no configuration")
	}
	layout := config.LayoutOf(filepath.Join(root, name))
	return StateDirs{StateDirRel: relTo(root, layout.StateDir)}, nil
}

// compatibleLayouts reports whether the two checkouts address their
// machine-local state the same way, so a copied overlay lands where the
// destination's own generate run reads it.
func compatibleLayouts(w *worktrees) bool {
	return w.srcState == w.dstState
}

// relStateDir is the state directory of root's layout, relative to root, for
// the refusal text.
func relStateDir(root string, state StateDirs) string {
	if state.StateDirRel == "" {
		return "(none)"
	}
	return state.StateDirRel
}

// resolveSource validates the run's frame: the source checkout, its
// repository, and both paths, without requiring the destination to be a
// registered worktree yet (create makes it first).
func resolveSource(ctx context.Context, opts *Options) (*worktrees, error) {
	if opts == nil {
		return nil, oops.Errorf("no options")
	}
	if strings.TrimSpace(opts.Destination) == "" {
		return nil, oops.Hint("name the linked worktree to provision").
			Errorf("no destination")
	}
	dst, err := filepath.Abs(opts.Destination)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve the destination")
	}
	src, err := config.DiscoverProjectRoot(ctx, opts.Source)
	if err != nil {
		return nil, oops.Hint("run this from the source checkout that holds the configuration").
			Errorf("find the source project: %v", err)
	}
	if sameDir(dst, src) {
		return nil, oops.Hint("provision another checkout, not the source itself").
			Errorf("the destination is the source checkout")
	}
	g := opts.Git
	top := g.TopLevelContext(ctx, src)
	if top == "" {
		return nil, oops.Hint("linked worktrees live in a git repository; run this inside one").
			Errorf("%s is not inside a git repository", src)
	}
	// A project may sit below the repository top (a monorepo); the
	// destination's project is the worktree's copy of that path.
	relProject := gitutil.RepoRelative(top, src)
	if relProject == "." {
		relProject = ""
	}
	if relProject != "" && strings.Contains(relProject, "..") {
		return nil, oops.Hint("the source project must live inside the repository that owns the linked worktrees").
			Errorf("the source project %s is not inside the repository %s", src, top)
	}
	srcState, err := stateDirsOf(src)
	if err != nil {
		return nil, oops.Hint("run this from a checkout whose configuration is committed, or point --source at it").
			Errorf("the source %s holds no AI-Rulez configuration", src)
	}
	return &worktrees{git: g, top: top, src: src, dst: dst, dstProj: projectAt(dst, relProject),
		relProject: relProject, srcState: srcState}, nil
}

// checkDestinationWorktree validates the destination once it exists: it must
// be a registered linked worktree of the source's repository, share its git
// common directory, and hold a configuration of its own.
func checkDestinationWorktree(ctx context.Context, w *worktrees) error {
	infos, err := w.git.WorktreesContext(ctx, w.top)
	if err != nil {
		return oops.Wrapf(err, "list the registered worktrees")
	}
	registered := false
	for _, info := range infos {
		if sameDir(info.Path, w.dst) {
			registered = true
			break
		}
	}
	if !registered {
		return oops.Hint("create it with `ai-rulez worktree create`, or let git or your host create it first").
			Errorf("%s is not a registered linked worktree of this repository", w.dst)
	}
	srcCommon, err := w.git.CommonDirContext(ctx, w.src)
	if err != nil {
		return oops.Wrapf(err, "resolve the source common dir")
	}
	dstCommon, err := w.git.CommonDirContext(ctx, w.dst)
	if err != nil {
		return oops.Hint("is the destination a checkout of the same repository?").
			Errorf("resolve the destination common dir: %v", err)
	}
	if !sameDir(srcCommon, dstCommon) {
		return oops.Hint("a provisioning run may only touch a linked worktree of the same repository").
			Errorf("the destination belongs to another repository: common dir %s, expected %s", dstCommon, srcCommon)
	}
	if _, err := os.Lstat(w.dst); err != nil {
		return oops.Hint("git registered the worktree but its directory is missing: remove it and create it again").
			Errorf("read the destination: %v", err)
	}
	dstState, err := stateDirsOf(w.dstProj)
	if err != nil {
		return oops.Hint("a worktree inherits the tracked configuration from its branch; commit the configuration first").
			Errorf("the destination holds no AI-Rulez configuration at %s", projectAt(w.dst, w.relProject))
	}
	w.dstState = dstState
	return nil
}

// projectAt is the project directory of the linked worktree: the worktree
// root itself, or the worktree's copy of the source project's path below the
// repository top.
func projectAt(dst, relProject string) string {
	if relProject == "" {
		return dst
	}
	return filepath.Join(dst, filepath.FromSlash(relProject))
}

// sameDir reports whether two paths are the same directory once resolved.
func sameDir(a, b string) bool {
	return gitutil.Resolve(a) == gitutil.Resolve(b)
}
