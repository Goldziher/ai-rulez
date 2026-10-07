package commands

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// approveGit runs git for the approval checks that read history: the commit the
// content is at, whether it is committed, who authored changes to it. Every
// answer comes from the repository that holds the configuration directory.
type approveGit struct {
	ctx context.Context
	top string
	cfg *config.Config
	// items caches the content digests recomputed at a commit.
	items map[string][]lockfile.Item
}

func newApproveGit(ctx context.Context, cfg *config.Config) (*approveGit, error) {
	top := (gitutil.Git{}).TopLevel(cfg.ConfigDir) //nolint:contextcheck // gitutil probes without a context
	if top == "" {
		return nil, oops.Errorf("%s is not inside a git work tree: this needs the commit history", cfg.ConfigDir)
	}
	return &approveGit{ctx: ctx, top: top, cfg: cfg}, nil
}

func (g *approveGit) out(args ...string) (string, error) {
	data, err := gitutil.Command(g.ctx, g.top, args...).Output()
	if err != nil {
		return "", oops.With("args", strings.Join(args, " ")).Wrapf(err, "git %s", args[0])
	}
	return strings.TrimSpace(string(data)), nil
}

// rel is path relative to the top of the work tree, "/"-separated.
func (g *approveGit) rel(abs string) string { return gitutil.RepoRelative(g.top, abs) }

// pinnedAt returns, for subject s, the function that reads the digest of the
// content as it was at a given commit. Authored content is recomputed from the
// files of that commit, so a lock committed there that disagrees with them (or a
// lock edited by hand) cannot vouch for content the reviewer never saw. Content
// that only the lock describes (remote includes, installed skills, role outputs)
// cannot be recomputed offline: the lock at that commit stands in for it.
// A commit that is not in the local clone is an error (fetch the pull request
// head); a commit without the content answers "not pinned".
func (g *approveGit) pinnedAt(s approval.Subject) func(ctx context.Context, sha string) (string, bool, error) {
	return func(ctx context.Context, sha string) (string, bool, error) {
		if err := gitutil.CheckArg("commit", sha); err != nil {
			return "", false, err //nolint:wrapcheck // names the argument
		}
		if _, err := g.out("cat-file", "-e", sha+"^{commit}"); err != nil {
			return "", false, oops.Hint("fetch the pull request head: git fetch origin pull/<number>/head").Errorf("commit %s is not in the local clone", sha)
		}
		if s.Class == approval.ClassLocal {
			return g.recomputedAt(ctx, s, sha)
		}
		return g.lockedAt(ctx, s, sha)
	}
}

// lockedAt reads the digest the lock at sha pins for s.
func (g *approveGit) lockedAt(ctx context.Context, s approval.Subject, sha string) (string, bool, error) {
	rel := g.rel(lockfile.Path(g.cfg.ConfigDir))
	if rel == "" {
		return "", false, oops.Errorf("%s is outside the git work tree", lockfile.Path(g.cfg.ConfigDir))
	}
	data, found, err := workspace.ReadFileAt(ctx, g.top, sha, rel, nil)
	if err != nil {
		return "", false, oops.With("commit", sha).Wrapf(err, "read the lock at the reviewed commit")
	}
	if !found {
		return "", false, nil
	}
	lock, err := lockfile.Parse(data)
	if err != nil {
		return "", false, oops.With("commit", sha).Wrapf(err, "read the lock at the reviewed commit")
	}
	for _, sub := range approval.SubjectsOf(lock, lock.Item) {
		if sub.Key() == s.Key() {
			return sub.Digest, true, nil
		}
	}
	return "", false, nil
}

// recomputedAt digests the authored content of the configuration as committed at
// sha (what `lock` pins, computed from the files, never read from the lock) and
// returns the digest of s. The tree is exported to a temporary directory.
func (g *approveGit) recomputedAt(ctx context.Context, s approval.Subject, sha string) (string, bool, error) {
	items, err := g.itemsAt(ctx, sha)
	if err != nil {
		return "", false, err
	}
	for _, sub := range approval.SubjectsOf(nil, items) {
		if sub.Key() == s.Key() {
			return sub.Digest, true, nil
		}
	}
	return "", false, nil
}

func (g *approveGit) itemsAt(ctx context.Context, sha string) ([]lockfile.Item, error) {
	if items, ok := g.items[sha]; ok {
		return items, nil
	}
	base := gitutil.RepoRelative(g.top, g.cfg.BaseDir)
	name := filepath.Base(g.cfg.ConfigDir)
	if base == "" || name == "" || name == "." {
		return nil, oops.Errorf("the configuration directory %s is outside the git work tree", g.cfg.ConfigDir)
	}
	dest, err := os.MkdirTemp("", "ai-rulez-approve-*")
	if err != nil {
		return nil, oops.Wrapf(err, "create a snapshot directory")
	}
	defer os.RemoveAll(dest) //nolint:errcheck // best-effort cleanup of a temporary directory
	if _, err := govview.ExtractRevision(ctx, g.cfg.BaseDir, sha, path.Join(filepath.ToSlash(base), name), dest); err != nil {
		return nil, oops.With("commit", sha).Wrapf(err, "read the configuration at the reviewed commit")
	}
	cfg, err := loadProjectDir(ctx, filepath.Join(dest, filepath.FromSlash(base)), name, config.WithoutRemote(), config.WithoutLocal())
	if err != nil {
		return nil, oops.With("commit", sha).Wrapf(err, "load the configuration at the reviewed commit")
	}
	profile := ""
	if lock, lerr := lockfile.Load(g.cfg.ConfigDir); lerr == nil && lock != nil {
		profile = lock.Profile
	}
	snap, err := lockSnapshot(cfg, profile, true)
	if err != nil {
		return nil, oops.With("commit", sha).Wrapf(err, "digest the content at the reviewed commit")
	}
	if g.items == nil {
		g.items = map[string][]lockfile.Item{}
	}
	g.items[sha] = snap.Items
	return snap.Items, nil
}

// baseRevision is the revision authors are counted from: the explicit one, else
// the upstream of the current branch, else the remote's default branch.
func (g *approveGit) baseRevision(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if up, err := g.out("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil && up != "" {
		return up, nil
	}
	if def, err := g.out("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && def != "" {
		return def, nil
	}
	return "", oops.Hint("pass --base <revision>: the branch the change will be merged into").
		Errorf("forbid_self_approval needs a revision to count authors from, and this branch has no upstream")
}

// authors lists the author emails of the commits since base that touched paths
// (repository-relative). The range is the merge base of base and HEAD to HEAD,
// so history that landed on base since the branch point does not count.
func (g *approveGit) authors(base string, paths []string) ([]string, error) {
	if err := gitutil.CheckArg("base revision", base); err != nil {
		return nil, err //nolint:wrapcheck // names the argument
	}
	mb, err := (gitutil.Git{}).MergeBase(g.top, base)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	args := []string{"log", "--format=%aE", mb + "..HEAD", "--"}
	args = append(args, paths...)
	out, err := g.out(args...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var emails []string
	for _, line := range strings.Split(out, "\n") {
		if e := strings.ToLower(strings.TrimSpace(line)); e != "" && !seen[e] {
			seen[e] = true
			emails = append(emails, e)
		}
	}
	return emails, nil
}

func joinConfig(configDir, rel string) string {
	return filepath.Join(configDir, filepath.FromSlash(rel))
}

// subjectPaths are the repository-relative paths whose history is the history of
// the subject: its source, else the lock file (remote entries, role outputs).
func (g *approveGit) subjectPaths(s approval.Subject) []string {
	if s.Path != "" {
		if rel := g.rel(joinConfig(g.cfg.ConfigDir, s.Path)); rel != "" {
			return []string{rel}
		}
	}
	if rel := g.rel(lockfile.Path(g.cfg.ConfigDir)); rel != "" {
		return []string{rel}
	}
	return nil
}
