package verifiers

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
	"github.com/samber/oops"
)

// Mode selects which files a run considers changed.
type Mode string

// The modes: every file (the default), the changes since a revision's merge
// base (--since), or what is staged (--staged).
const (
	ModeAll    Mode = "all"
	ModeSince  Mode = "since"
	ModeStaged Mode = "staged"
)

// scopeData is what scoped verifiers see: the files of the repository and
// which of them changed (with the lines they gained).
type scopeData struct {
	mode    Mode
	rev     string
	changed map[string]gitutil.Change
	tree    []string
	treeSet map[string]bool
}

func newScope(mode Mode, rev string, changes []gitutil.Change, tree []string) *scopeData {
	sd := &scopeData{mode: mode, rev: rev, changed: map[string]gitutil.Change{}, tree: tree, treeSet: map[string]bool{}}
	for _, c := range changes {
		sd.changed[c.Path] = c
	}
	for _, f := range tree {
		sd.treeSet[f] = true
	}
	return sd
}

// describe names the mode for reports.
func (s *scopeData) describe() string {
	switch s.mode {
	case ModeSince:
		return "since " + s.rev
	case ModeStaged:
		return "staged"
	}
	return "all"
}

// prepareScope resolves the file set once per run. A base revision that does
// not resolve or share history with HEAD is an error, never an empty scope.
func (e *Env) prepareScope(ctx context.Context) error {
	if e.scope != nil {
		return nil
	}
	tree, err := e.treeFiles(ctx)
	if err != nil {
		return err
	}
	var (
		mode    = ModeAll
		rev     string
		changes []gitutil.Change
	)
	switch {
	case e.opts.Since != "":
		mode, rev = ModeSince, e.opts.Since
		if changes, err = gitutil.ChangesSince(e.Root, rev); err != nil {
			return oops.Wrapf(err, "resolve changed files")
		}
	case e.opts.Staged:
		mode = ModeStaged
		if changes, err = gitutil.StagedChanges(e.Root); err != nil {
			return oops.Wrapf(err, "resolve staged files")
		}
	default:
		for _, f := range tree {
			changes = append(changes, gitutil.Change{Path: f, Status: 'A', AllAdded: true})
		}
	}
	e.scope = newScope(mode, rev, changes, tree)
	return nil
}

// treeFiles lists the regular files of the project: tracked plus untracked
// non-ignored files inside a repository, a directory walk elsewhere. Symlinks
// are never listed.
func (e *Env) treeFiles(ctx context.Context) ([]string, error) {
	listed, ok, err := gitutil.ListFiles(e.Root)
	if err != nil {
		return nil, oops.Wrapf(err, "list repository files")
	}
	if !ok {
		return e.listFiles(ctx)
	}
	out := make([]string, 0, len(listed))
	for _, f := range listed {
		if err := ctx.Err(); err != nil {
			return nil, oops.Wrapf(err, "verifier canceled")
		}
		if inSkippedDir(f) {
			continue
		}
		fi, err := os.Lstat(filepath.Join(e.Root, filepath.FromSlash(f)))
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		out = append(out, f)
	}
	sort.Strings(out)
	return out, nil
}

func inSkippedDir(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, p := range parts[:len(parts)-1] {
		if skipDirs[p] {
			return true
		}
	}
	return false
}

// scopedFiles returns the changed, still existing files the spec's
// when_changed selects. applicable is false when when_changed is set and
// selects nothing.
func (e *Env) scopedFiles(sp *Spec) (files []string, applicable bool, err error) {
	if len(sp.WhenChanged) == 0 {
		return nil, true, nil
	}
	inc, err := compileGlobs(sp.WhenChanged)
	if err != nil {
		return nil, false, err
	}
	excl, err := compileGlobs(sp.Exclude)
	if err != nil {
		return nil, false, err
	}
	for p, c := range e.scope.changed {
		if c.Status == 'D' || !e.scope.treeSet[p] {
			continue
		}
		if matchesAny(inc, p) && !matchesAny(excl, p) {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	return files, len(files) > 0, nil
}

// deadScope reports whether no file of the repository matches when_changed.
func (e *Env) deadScope(sp *Spec) bool {
	if len(sp.WhenChanged) == 0 {
		return false
	}
	inc, err := compileGlobs(sp.WhenChanged)
	if err != nil {
		return false
	}
	for _, f := range e.scope.tree {
		if matchesAny(inc, f) {
			return false
		}
	}
	return true
}

func compileGlobs(patterns []string) ([]vspec.Glob, error) {
	out := make([]vspec.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := vspec.CompileGlob(p)
		if err != nil {
			return nil, oops.Wrapf(err, "invalid glob %q", p)
		}
		out = append(out, g)
	}
	return out, nil
}

func matchesAny(globs []vspec.Glob, p string) bool {
	for _, g := range globs {
		if g.Match(p) {
			return true
		}
	}
	return false
}
