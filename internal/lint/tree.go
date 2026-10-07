package lint

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
)

// Tree is the set of files the checks resolve references against: the git
// index when the root is inside a repository, else a walk of the directory.
type Tree struct {
	Top string // repo root (git toplevel) or the base dir outside git
	Git bool
	// Explicit is set when the top was chosen with --repo-root rather than found.
	Explicit bool
	files    map[string]uint32
	dirs     map[string]struct{}
	topNames map[string]struct{}
}

// LoadTree indexes the tracked files below the repository containing base.
func LoadTree(base string) (*Tree, error) {
	return LoadTreeAt(base, "")
}

// LoadTreeAt is LoadTree with an explicit repository root. A non-empty root is
// used as the tree top whether or not base lies inside it, so a configuration
// checked out away from its repository (a scratch copy, a CI artifact) resolves
// repo-relative paths and globs against the real tree.
func LoadTreeAt(base, root string) (*Tree, error) {
	return LoadTreeWith(gitutil.Git{}, base, root)
}

// LoadTreeWith is LoadTreeAt with the git questions answered through git, so a
// caller can inject the runner that starts the process.
func LoadTreeWith(git gitutil.Git, base, root string) (*Tree, error) {
	return LoadTreeContext(context.Background(), git, base, root)
}

// LoadTreeContext is LoadTreeWith with the git probes bounded by ctx.
func LoadTreeContext(ctx context.Context, git gitutil.Git, base, root string) (*Tree, error) {
	base = gitutil.Resolve(base)
	t := &Tree{files: map[string]uint32{}, dirs: map[string]struct{}{}, topNames: map[string]struct{}{}}
	if root != "" {
		root = gitutil.Resolve(root)
		t.Explicit = true
		files, ok, err := git.TrackedFilesContext(ctx, root)
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		t.Top, t.Git = root, ok
		if ok {
			t.files = files
		} else {
			t.walk()
		}
		t.index()
		return t, nil
	}
	if top := git.TopLevelContext(ctx, base); top != "" {
		files, ok, err := git.TrackedFilesContext(ctx, top)
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		if ok {
			t.Top, t.Git, t.files = gitutil.Resolve(top), true, files
		}
	}
	if !t.Git {
		t.Top = base
		t.walk()
	}
	t.index()
	return t, nil
}

// index derives the top-level names and directory set from the file list.
func (t *Tree) index() {
	for f := range t.files {
		if i := strings.IndexByte(f, '/'); i >= 0 {
			t.topNames[f[:i]] = struct{}{}
		} else {
			t.topNames[f] = struct{}{}
		}
		for d := filepath.ToSlash(filepath.Dir(f)); d != "." && d != "/"; d = filepath.ToSlash(filepath.Dir(d)) {
			if _, seen := t.dirs[d]; seen {
				break
			}
			t.dirs[d] = struct{}{}
		}
	}
}

func (t *Tree) walk() {
	_ = filepath.WalkDir(t.Top, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil {
			return nil //nolint:nilerr // best effort
		}
		if d.IsDir() {
			if p != t.Top && walkutil.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(t.Top, p)
		if rerr != nil {
			return nil //nolint:nilerr // best effort
		}
		mode := uint32(0o100644)
		if info, ierr := d.Info(); ierr == nil && info.Mode()&0o111 != 0 {
			mode = 0o100755
		}
		t.files[filepath.ToSlash(rel)] = mode
		return nil
	})
}

// Rel returns abs as a slash path relative to the tree top, or "" when outside.
func (t *Tree) Rel(abs string) string {
	rel, err := filepath.Rel(t.Top, gitutil.Resolve(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Paths lists every indexed file.
func (t *Tree) Paths() []string {
	out := make([]string, 0, len(t.files))
	for f := range t.files {
		out = append(out, f)
	}
	return out
}

// IsTopLevel reports whether name is a top-level file or directory of the repo.
func (t *Tree) IsTopLevel(name string) bool { _, ok := t.topNames[name]; return ok }

// Exists reports whether rel (slash path from the top) is a tracked file or
// directory, or is present on disk (so a new, not yet added file resolves).
func (t *Tree) Exists(rel string) bool {
	rel = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(rel)), "/")
	if rel == "." || rel == "" {
		return true
	}
	if _, ok := t.files[rel]; ok {
		return true
	}
	if _, ok := t.dirs[rel]; ok {
		return true
	}
	_, err := os.Stat(filepath.Join(t.Top, filepath.FromSlash(rel)))
	return err == nil
}

// Executable reports whether rel is executable: by git mode when tracked, else
// by the file-system mode. known is false when the file cannot be found.
func (t *Tree) Executable(rel string) (exec, known bool) {
	if mode, ok := t.files[rel]; ok {
		return mode&0o111 != 0, true
	}
	info, err := os.Stat(filepath.Join(t.Top, filepath.FromSlash(rel)))
	if err != nil || info.IsDir() {
		return false, false
	}
	return info.Mode()&0o111 != 0, true
}

// matchAny reports whether the glob matches at least one tracked file, taking
// paths relative to the repo top or, for a nested root, to the root's own
// directory (baseRel, slash path from the top; empty for the top itself).
func (t *Tree) matchAny(g globMatcher, baseRel string) bool {
	prefix := ""
	if baseRel != "" {
		prefix = baseRel + "/"
	}
	for f := range t.files {
		if g.match(f) {
			return true
		}
		if prefix != "" && strings.HasPrefix(f, prefix) && g.match(f[len(prefix):]) {
			return true
		}
	}
	return false
}

// Loader memoizes trees so linting many roots of one repository reads the git
// index once.
type Loader struct {
	// Root overrides the repository root for every tree (see LoadTreeAt).
	Root  string
	cache map[string]*Tree
}

// Load is LoadContext without a caller's context.
func (l *Loader) Load(base string) (*Tree, error) {
	return l.LoadContext(context.Background(), base)
}

// LoadContext returns the tree for the repository containing base, with the git
// probes bounded by ctx.
func (l *Loader) LoadContext(ctx context.Context, base string) (*Tree, error) {
	if l.cache == nil {
		l.cache = map[string]*Tree{}
	}
	key := gitutil.Git{}.TopLevelContext(ctx, gitutil.Resolve(base))
	if l.Root != "" {
		key = "root:" + gitutil.Resolve(l.Root)
	} else if key == "" {
		key = "fs:" + gitutil.Resolve(base)
	}
	if t, ok := l.cache[key]; ok {
		return t, nil
	}
	t, err := LoadTreeContext(ctx, gitutil.Git{}, base, l.Root)
	if err != nil {
		return nil, err
	}
	l.cache[key] = t
	return t, nil
}
