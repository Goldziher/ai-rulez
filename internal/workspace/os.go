package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
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
// nearest ancestor holding a .git entry when that repository tracks the project,
// else at dir itself. A content symlink may point anywhere in that tree, so a
// project nested in a larger repository can link to its siblings; a project that
// merely sits below an unrelated repository (a dotfiles repository in $HOME) gets
// no such reach.
func Around(dir string) (Workspace, error) {
	return AroundBelow(context.Background(), gitutil.Git{}, dir, "")
}

// AroundBelow is Around that, like the VCS itself, does not look for a repository
// in the directories listed in ceilings (the value of GIT_CEILING_DIRECTORIES:
// paths separated by the OS list separator, symlinks in them resolved as the VCS
// does) or above them. Whether a repository tracks the project is asked
// through git, under ctx.
func AroundBelow(ctx context.Context, git gitutil.Git, dir, ceilings string) (Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, oops.With("path", dir).Wrapf(err, "resolve workspace root")
	}
	return OS(vcsTop(ctx, git, abs, ceilings))
}

// projectMarker is the file whose presence in a repository's index says the
// repository holds the project.
const projectMarker = ".ai-rulez/config.toml"

// vcsTop walks up from abs looking for a .git entry. It returns abs when none
// exists, when a ceiling directory is reached first, or when the repository found
// does not track the project: only a repository that holds the project may widen
// the boundary its symlinks are held to.
func vcsTop(ctx context.Context, git gitutil.Git, abs, ceilings string) string {
	ceiling := map[string]bool{}
	for _, c := range filepath.SplitList(ceilings) {
		if c == "" {
			continue
		}
		ceiling[filepath.Clean(c)] = true
		if realPath, err := filepath.EvalSymlinks(c); err == nil {
			ceiling[realPath] = true
		}
	}
	for cur := abs; ; {
		if ceiling[cur] {
			return abs
		}
		if realPath, err := filepath.EvalSymlinks(cur); err == nil && ceiling[realPath] {
			return abs
		}
		if _, err := os.Lstat(filepath.Join(cur, vcsDirName)); err == nil {
			if cur == abs || tracksProject(ctx, git, cur, abs) {
				return cur
			}
			return abs
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		cur = parent
	}
}

// tracksProject reports whether the repository at repo has the project's config
// in its index. A failure to ask counts as not tracked: the narrower root is the
// safe one.
func tracksProject(ctx context.Context, git gitutil.Git, repo, project string) bool {
	rel, err := filepath.Rel(repo, project)
	if err != nil {
		return false
	}
	marker := filepath.ToSlash(filepath.Join(rel, filepath.FromSlash(projectMarker)))
	res := git.Exec(ctx, repo, nil, "ls-files", "-z", "--", ":(literal)"+marker)
	if gitutil.ResultErr(res) != nil {
		return false // not a repository, or git could not answer
	}
	return slices.Contains(strings.Split(string(res.Stdout), "\x00"), marker)
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
