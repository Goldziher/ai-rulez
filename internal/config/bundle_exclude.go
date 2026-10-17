package config

import (
	"bytes"
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// gitDirName is the name of a git metadata directory.
const gitDirName = ".git"

// DefaultBundleExcludes are the names that are never bundled with a skill or
// command, whatever the project's .gitignore says: virtualenvs, bytecode caches,
// dependency trees and VCS metadata are build artifacts, not skill content.
// Projects add to the list with the top-level bundle_exclude config key; an entry
// prefixed with "!" re-includes a path a default excluded (e.g. "!references/venv").
var DefaultBundleExcludes = []string{gitDirName, ".venv*", "venv", "__pycache__", "*.pyc", "node_modules"}

// bundleFilter decides which files below a skill or command root are bundled.
type bundleFilter struct {
	patterns []string
	log      logger.Logger
	// visible is the set of root-relative slash paths git considers part of the
	// project (tracked, or untracked and not ignored). Nil disables the check:
	// the root is not in a git work tree, git is missing, or the root itself is
	// ignored (an author who gitignores the whole content tree still wants it).
	visible map[string]bool
}

// newBundleFilter builds the filter for one skill or command root. marker is the
// item's entry file (SKILL.md, COMMAND.md).
func newBundleFilter(ctx context.Context, git gitutil.Git, log logger.Logger, root, marker string, extra []string) *bundleFilter {
	return newBundleFilterWith(ctx, git, log, root, marker, extra, nil)
}

// newBundleFilterWith is newBundleFilter answering the git question from the
// snapshots when given, so the items of one parent directory share two git calls.
func newBundleFilterWith(ctx context.Context, git gitutil.Git, log logger.Logger, root, marker string, extra []string, snaps *bundleSnapshots) *bundleFilter {
	patterns := make([]string, 0, len(DefaultBundleExcludes)+len(extra))
	patterns = append(patterns, DefaultBundleExcludes...)
	for _, p := range extra {
		if p = strings.TrimSpace(p); p != "" {
			patterns = append(patterns, p)
		}
	}
	log = logger.Or(log)
	var visible map[string]bool
	if snaps != nil {
		visible = snaps.visible(ctx, git, log, root, marker)
	} else {
		visible = gitVisibleFiles(ctx, git, log, root, marker)
	}
	return &bundleFilter{patterns: patterns, log: log, visible: visible}
}

// bundleSnapshots answers gitVisibleFiles for many sibling items from one
// `git ls-files` and one batched `git check-ignore` per parent directory,
// instead of two git processes per item. The answers are those the per-item
// calls would give: an item that is itself a repository (or a symlink) is asked
// on its own, because git run inside it sees another work tree.
type bundleSnapshots struct {
	mu sync.Mutex
	by map[snapshotKey]*parentSnapshot

	// anchor, when set, is a directory (the configuration directory) whose one
	// `git ls-files` listing answers every parent below it. A repository with a
	// large index pays about a second per listing, and a project with dozens of
	// domains asked once per domain's skills and commands directory.
	anchor string
	wide   *wideListing
	// indexIgnores answers "does git ignore this item's entry file" from the
	// anchor's listing instead of one `git check-ignore` per parent directory.
	// It is set for a scan that shares one snapshot across many single-item
	// loads (see WithBundleSnapshots), where a process per parent costs seconds
	// on a large repository.
	indexIgnores bool
}

// wideListing is one `git ls-files` run at the anchor: the slash paths, relative
// to the anchor, that git does not ignore. ok is false when git could not list.
type wideListing struct {
	paths []string
	ok    bool
	// set holds paths for membership tests; markers holds, per entry file name,
	// the listed files of that name an ignore rule matches. Both are filled on
	// demand, only when indexIgnores is set.
	set     map[string]bool
	markers map[string]map[string]bool
}

type snapshotKey struct{ parent, marker string }

// parentSnapshot is what git says about the children of one directory.
type parentSnapshot struct {
	// ignored holds the children whose entry file git ignores.
	ignored map[string]bool
	// files maps a child to the child-relative slash paths git considers part
	// of the project. Nil when git could not list the directory.
	files map[string]map[string]bool
	// probed are the children the ignore batch covered.
	probed map[string]bool
}

func (b *bundleSnapshots) visible(ctx context.Context, git gitutil.Git, log logger.Logger, root, marker string) map[string]bool {
	root = filepath.Clean(root)
	if standsAlone(root) {
		return gitVisibleFiles(ctx, git, log, root, marker)
	}
	parent, name := filepath.Dir(root), filepath.Base(root)
	snap := b.snapshot(ctx, git, log, parent, marker)
	if !snap.probed[name] {
		return gitVisibleFiles(ctx, git, log, root, marker)
	}
	if snap.ignored[name] || len(snap.files[name]) == 0 {
		return nil
	}
	return snap.files[name]
}

// standsAlone reports whether git run inside root may see a different work tree
// than git run in its parent: root is a symlink or holds its own .git.
func standsAlone(root string) bool {
	if info, err := os.Lstat(root); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	_, err := os.Lstat(filepath.Join(root, gitDirName))
	return err == nil
}

func (b *bundleSnapshots) snapshot(ctx context.Context, git gitutil.Git, log logger.Logger, parent, marker string) *parentSnapshot {
	key := snapshotKey{parent, marker}
	b.mu.Lock()
	defer b.mu.Unlock()
	if snap, ok := b.by[key]; ok {
		return snap
	}
	listed := b.filesBelow(ctx, git, log, parent)
	snap := b.takeSnapshot(ctx, git, log, parent, marker, listed)
	if b.by == nil {
		b.by = make(map[snapshotKey]*parentSnapshot)
	}
	b.by[key] = snap
	return snap
}

// filesBelow answers `git ls-files` for parent from the anchor's single listing:
// the git-visible slash paths below parent, relative to it. It returns nil when no
// anchor applies (none set, parent outside it, or a nested repository between
// them, whose files the anchor's git does not list) or git could not list.
func (b *bundleSnapshots) filesBelow(ctx context.Context, git gitutil.Git, log logger.Logger, parent string) []string { //nolint:gocyclo // one guarded walk from anchor check to listing; splitting it scatters the fallbacks
	if b.anchor == "" {
		return nil
	}
	rel, err := filepath.Rel(b.anchor, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	for dir := parent; dir != b.anchor && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, gitDirName)); err == nil {
			return nil
		}
	}
	if b.wide == nil {
		b.wide = b.loadWide(ctx, git, log)
	}
	if !b.wide.ok {
		return nil
	}
	prefix := ""
	if rel != "." {
		prefix = filepath.ToSlash(rel) + "/"
	}
	below := []string{}
	for _, p := range b.wide.paths {
		if strings.HasPrefix(p, prefix) {
			below = append(below, strings.TrimPrefix(p, prefix))
		}
	}
	return below
}

// loadWide reads the anchor's listing once.
func (b *bundleSnapshots) loadWide(ctx context.Context, git gitutil.Git, log logger.Logger) *wideListing {
	w := &wideListing{}
	files, ok, err := git.ListFilesContext(ctx, b.anchor)
	if err != nil || !ok {
		if err != nil {
			log.Debug("git ls-files unavailable, bundling without .gitignore", "path", b.anchor, "error", err)
		}
		return w
	}
	w.ok = true
	w.paths = make([]string, 0, len(files))
	for _, p := range files {
		w.paths = append(w.paths, filepath.ToSlash(p))
	}
	return w
}

// markerIgnored answers `git check-ignore --no-index` for the entry file of the
// child name of parent from the anchor's listing. The tracked entry files of the
// whole repository are asked in one batched call; a file the listing lacks is left
// to git (it may be ignored, or only spelled in another case). ok is false when
// the listing cannot answer.
func (b *bundleSnapshots) markerIgnored(ctx context.Context, git gitutil.Git, parent, name, marker string) (ignored, ok bool) {
	if !b.indexIgnores || b.wide == nil || !b.wide.ok {
		return false, false
	}
	rel, err := filepath.Rel(b.anchor, filepath.Join(parent, name, marker))
	if err != nil {
		return false, false
	}
	rel = filepath.ToSlash(rel)
	if b.wide.set == nil {
		b.wide.set = make(map[string]bool, len(b.wide.paths))
		for _, p := range b.wide.paths {
			b.wide.set[p] = true
		}
	}
	if b.wide.set[rel] {
		tracked, ok := b.trackedIgnoredMarkers(ctx, git, marker)
		if !ok {
			return false, false
		}
		return tracked[rel], true
	}
	// Not listed: nothing to ignore when the file is absent. A file that is there
	// may be unlisted only because the listing spells its name in another case
	// (a case-insensitive filesystem), so git is asked about it.
	if _, err = os.Lstat(filepath.Join(parent, name, marker)); err == nil {
		return false, false
	}
	return false, true
}

// trackedIgnoredMarkers returns which listed files named marker an ignore rule
// matches, asked of git once for all of them.
func (b *bundleSnapshots) trackedIgnoredMarkers(ctx context.Context, git gitutil.Git, marker string) (map[string]bool, bool) {
	if got, done := b.wide.markers[marker]; done {
		return got, got != nil
	}
	if b.wide.markers == nil {
		b.wide.markers = map[string]map[string]bool{}
	}
	var stdin bytes.Buffer
	for _, p := range b.wide.paths {
		if p == marker || strings.HasSuffix(p, "/"+marker) {
			stdin.WriteString(p)
			stdin.WriteByte(0)
		}
	}
	got := map[string]bool{}
	if stdin.Len() > 0 {
		res := git.ExecStdin(ctx, b.anchor, stdin.Bytes(), 0, "check-ignore", "--no-index", "--stdin", "-z")
		switch {
		case res.Status == runner.StatusOK:
			for _, p := range bytes.Split(res.Stdout, []byte{0}) {
				if len(p) > 0 {
					got[filepath.ToSlash(string(p))] = true
				}
			}
		case res.Status == runner.StatusExit && res.ExitCode == 1: // none ignored
		default:
			got = nil
		}
	}
	b.wide.markers[marker] = got
	return got, got != nil
}

func (b *bundleSnapshots) takeSnapshot(ctx context.Context, git gitutil.Git, log logger.Logger, parent, marker string, listed []string) *parentSnapshot { //nolint:gocyclo // one probe-then-list sequence; splitting it scatters the fallbacks
	snap := &parentSnapshot{ignored: map[string]bool{}, files: map[string]map[string]bool{}, probed: map[string]bool{}}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return snap
	}
	var stdin bytes.Buffer
	probed := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		snap.probed[e.Name()] = true
		probed = true
		if listed != nil {
			if ignored, ok := b.markerIgnored(ctx, git, parent, e.Name(), marker); ok {
				if ignored {
					snap.ignored[e.Name()] = true
				}
				continue
			}
		}
		stdin.WriteString(e.Name() + "/" + marker)
		stdin.WriteByte(0)
	}
	if !probed {
		return snap
	}
	// Exit 0/1: some/none of the entry files are ignored (the whole item is);
	// 128: not a repo. Neither is an answer by itself, as in the per-item case.
	if stdin.Len() > 0 {
		res := git.ExecStdin(ctx, parent, stdin.Bytes(), 0, "check-ignore", "--no-index", "--stdin", "-z")
		if res.Status == runner.StatusOK {
			for _, p := range bytes.Split(res.Stdout, []byte{0}) {
				if name, _, ok := strings.Cut(filepath.ToSlash(string(p)), "/"); ok {
					snap.ignored[name] = true
				}
			}
		}
		if listed == nil && res.ExitCode == gitFatalExit {
			// Not a repository: ls-files would fail the same way, so skip its process.
			log.Debug("git ls-files unavailable, bundling without .gitignore", "path", parent, "error", gitutil.ResultErr(res))
			return snap
		}
	}
	if listed == nil {
		res := git.Exec(ctx, parent, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
		if err := gitutil.ResultErr(res); err != nil {
			if res.Status != runner.StatusUnavailable {
				log.Debug("git ls-files unavailable, bundling without .gitignore", "path", parent, "error", err)
			}
			return snap
		}
		for _, p := range bytes.Split(res.Stdout, []byte{0}) {
			listed = append(listed, filepath.ToSlash(string(p)))
		}
	}
	for _, p := range listed {
		name, rel, ok := strings.Cut(p, "/")
		if !ok || rel == "" {
			continue
		}
		if snap.files[name] == nil {
			snap.files[name] = make(map[string]bool)
		}
		snap.files[name][rel] = true
	}
	return snap
}

// gitFatalExit is the exit status git uses for a fatal error, such as a
// directory that is not a repository.
const gitFatalExit = 128

// gitVisibleFiles lists the files under root that git would not ignore, or nil
// when that cannot be determined.
func gitVisibleFiles(ctx context.Context, git gitutil.Git, log logger.Logger, root, marker string) map[string]bool {
	// Exit 0: the entry file is ignored, so the whole item is; 128: not a repo;
	// an unavailable git is not an answer either (the ls-files call below fails).
	res := git.Exec(ctx, root, nil, "check-ignore", "-q", "--no-index", "--", marker)
	if res.Status == runner.StatusOK {
		return nil
	}
	if res.ExitCode == gitFatalExit {
		// Not a repository: ls-files would fail the same way, so skip its process.
		log.Debug("git ls-files unavailable, bundling without .gitignore", "path", root, "error", gitutil.ResultErr(res))
		return nil
	}
	res = git.Exec(ctx, root, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
	if err := gitutil.ResultErr(res); err != nil {
		if res.Status != runner.StatusUnavailable {
			log.Debug("git ls-files unavailable, bundling without .gitignore", "path", root, "error", err)
		}
		return nil
	}
	out := res.Stdout
	visible := make(map[string]bool)
	for _, name := range bytes.Split(out, []byte{0}) {
		if len(name) > 0 {
			visible[filepath.ToSlash(string(name))] = true
		}
	}
	if len(visible) == 0 {
		return nil
	}
	return visible
}

// excluded reports whether the root-relative slash path rel (a file, or a
// directory when isDir) is left out of the bundle by a pattern. A pattern
// without a slash matches any path segment; one with a slash matches the whole
// relative path or any leading directory of it.
func (f *bundleFilter) excluded(rel string) bool {
	segments := strings.Split(rel, "/")
	skip := false
	matched := ""
	// Later patterns win, so a "!pattern" entry re-includes what a default or an
	// earlier pattern excluded.
	for _, pattern := range f.patterns {
		negate := strings.HasPrefix(pattern, "!")
		raw := strings.TrimPrefix(pattern, "!")
		if matchesPattern(raw, segments) {
			skip = !negate
			matched = pattern
		}
	}
	if skip {
		f.log.Debug("Skipped bundle path matching an exclude pattern", "path", rel, "pattern", matched)
	}
	return skip
}

// matchesPattern reports whether pattern matches a segment (slash-less pattern)
// or the whole path / a leading directory of it (pattern with a slash).
func matchesPattern(pattern string, segments []string) bool {
	pattern = strings.Trim(pattern, "/")
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		return slices.ContainsFunc(segments, func(seg string) bool { return globMatch(pattern, seg) })
	}
	for i := range segments {
		if globMatch(pattern, strings.Join(segments[:i+1], "/")) {
			return true
		}
	}
	return false
}

// keepFile reports whether the file at root-relative slash path rel is bundled.
func (f *bundleFilter) keepFile(rel string) bool {
	if f.excluded(rel) {
		return false
	}
	return f.visible == nil || f.visible[rel]
}

// rel is the root-relative slash path of p.
func (f *bundleFilter) rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// globMatch reports whether name matches the glob; a malformed pattern matches nothing.
func globMatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
