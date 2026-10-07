package workspace

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

// vcsDirName marks the top of a version-controlled work tree.
const vcsDirName = ".git"

// OS returns a workspace over the directory root of the real file system. A
// relative root is made absolute here, which is the one place a relative path may
// meet the process working directory. Symlinks are followed on read; deciding
// whether one may be followed is the caller's policy (see Resolve).
func OS(root string) (Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, oops.With("path", root).Wrapf(err, "resolve workspace root")
	}
	w := &fsWorkspace{fsys: os.DirFS(abs), root: abs, reach: true}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		w.aliases = []string{resolved}
	}
	return w, nil
}

// Around returns a workspace for the project that contains dir: rooted at the
// nearest ancestor holding a .git entry, or at dir itself when there is none. A
// content symlink may point anywhere in that tree, so a project nested in a
// larger repository can link to its siblings.
func Around(dir string) (Workspace, error) {
	return AroundBelow(dir, "")
}

// AroundBelow is Around that, like the VCS itself, does not look for a repository
// in the directories listed in ceilings (the value of GIT_CEILING_DIRECTORIES:
// paths separated by the OS list separator) or above them.
func AroundBelow(dir, ceilings string) (Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, oops.With("path", dir).Wrapf(err, "resolve workspace root")
	}
	return OS(vcsTop(abs, ceilings))
}

// vcsTop walks up from abs looking for a .git entry; it returns abs when none
// exists, or when a ceiling directory is reached first.
func vcsTop(abs, ceilings string) string {
	ceiling := map[string]bool{}
	for _, c := range filepath.SplitList(ceilings) {
		if c != "" {
			ceiling[filepath.Clean(c)] = true
		}
	}
	for cur := abs; ; {
		if ceiling[cur] {
			return abs
		}
		if _, err := os.Lstat(filepath.Join(cur, vcsDirName)); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		cur = parent
	}
}

// IsDisk reports whether ws reads the real file system: only such a workspace
// can be written to by a generate run, and only its root names a directory that
// exists.
func IsDisk(ws Workspace) bool {
	w, ok := ws.(*fsWorkspace)
	return ok && w.reach
}

// rooted is a Workspace that reports another root.
type rooted struct {
	Workspace
	root string
}

func (r rooted) Root() string { return r.root }

// WithRoot returns ws reporting root as its root. A workspace that is not backed by
// the real file system is given a virtual root this way, so an engine that keeps
// absolute paths in its data model never names a directory that exists on the disk
// of the process; reads of such a path fail as if nothing had been generated there.
func WithRoot(ws Workspace, root string) Workspace {
	return rooted{Workspace: ws, root: filepath.Clean(root)}
}

// CommitOf returns the commit ws reads when it is a snapshot of one (also when
// WithRoot wrapped it), and false for any other workspace.
func CommitOf(ws Workspace) (string, bool) {
	for {
		switch w := ws.(type) {
		case Snapshot:
			return w.Commit(), true
		case rooted:
			ws = w.Workspace
		default:
			return "", false
		}
	}
}
