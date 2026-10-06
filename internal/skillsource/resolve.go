package skillsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// Options controls how a source is resolved.
type Options struct {
	// CacheDir is the cache root; empty means ~/.cache/ai-rulez/skill-sources.
	CacheDir string
	// Lock supplies the pins to honor (may be nil).
	Lock *lockfile.File
	// Offline never uses the network: a source is read from the lock's pinned
	// commit, or from the ref recorded by an earlier online resolution, and must
	// already be cached.
	Offline bool
	// Frozen is Offline that additionally requires ai-rulez.lock to cover the
	// source: nothing is resolved that the lock does not already pin.
	Frozen bool
	// Refresh ignores the lock's pin and resolves the ref again (`ai-rulez lock`).
	Refresh bool
	// ProjectRoot is the project a local source must stay inside unless the
	// source allows outside paths (see Spec.AllowOutside). It is also the base of
	// a relative local path of such a source.
	ProjectRoot string
	// MaxCloneBytes is the clone size limit of a git source that sets none; 0
	// selects the AI_RULEZ_MAX_CLONE_BYTES environment variable, then the default.
	MaxCloneBytes int64
}

// Resolved is a source fetched (or found in the cache) and ready to read.
type Resolved struct {
	Spec Spec
	// Dir is the directory holding the skill directories.
	Dir string
	// Commit is the commit the tree is at; empty for a local directory.
	Commit string
	// Digest is the lockfile tree digest of Dir.
	Digest string
	// Pinned reports that the source cannot move under the user: a full commit SHA, or a ref covered by the lock.
	Pinned bool
	// Locked reports that the lock covered the source and the tree matched it.
	Locked bool
	// RefKind is "tag", "branch", "head", "commit" or "local".
	RefKind string
	// Skills are the discovered skills after include/exclude selection, by name.
	Skills []Skill
}

// Want is the lock description of a source.
func (s Spec) Want() lockfile.Want {
	return lockfile.Want{Kind: lockfile.KindSource, Name: s.Name, Source: s.Redacted(), Path: s.Path, Ref: s.Ref}
}

// Entry is the lock entry recording what the source resolved to.
func (r *Resolved) Entry() lockfile.Entry {
	w := r.Spec.Want()
	return lockfile.Entry{Name: w.Name, Source: w.Source, Path: w.Path, Ref: w.Ref, Commit: r.Commit, Digest: r.Digest}
}

// Resolve fetches (or finds in the cache) the source and lists its skills.
func Resolve(ctx context.Context, spec Spec, opts Options) (*Resolved, error) {
	if !spec.IsGit() {
		return resolveLocal(spec, opts)
	}
	return resolveGit(ctx, spec, opts)
}

func errLock(spec Spec, format string, args ...any) error {
	return oops.Wrapf(config.ErrLockViolation, "skill source %q: %s", spec.Name, fmt.Sprintf(format, args...))
}

func resolveLocal(spec Spec, opts Options) (*Resolved, error) {
	base := spec.URL
	if !spec.AllowOutside && opts.ProjectRoot != "" && !filepath.IsAbs(base) {
		base = filepath.Join(opts.ProjectRoot, base)
	}
	root := base
	if spec.Path != "" {
		root = filepath.Join(root, filepath.FromSlash(spec.Path))
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve local skill source %q", spec.Name)
	}
	// A symlinked root is the user's own choice of directory: resolve it and read
	// (and digest) the real directory. Links below the root are never followed.
	if resolved, linkErr := filepath.EvalSymlinks(root); linkErr == nil {
		root = resolved
	}
	if err := checkInsideProject(spec, opts, root); err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, oops.With("path", root).Errorf("skill source %q: %s is not a directory", spec.Name, root)
	}
	digest, err := contentlock.DigestDir(contentlock.KindSkillSource, root)
	if err != nil {
		return nil, err //nolint:wrapcheck // already wrapped
	}
	res := &Resolved{Spec: spec, Dir: root, Digest: digest, RefKind: "local"}
	entry := opts.Lock.Find(lockfile.KindSource, spec.Name)
	if entry != nil && !opts.Refresh {
		if entry.Digest != "" && entry.Digest != digest {
			return nil, errLock(spec, "content digest %s does not match the lock's %s; run `ai-rulez lock` only after reviewing the change", digest, entry.Digest)
		}
		res.Locked = true
	}
	res.Pinned = res.Locked
	res.Skills, err = Discover(spec, root)
	return res, err
}

// checkInsideProject refuses a local source that resolves outside the project.
// A committed config can name any path (`/home/victim/.claude/skills`, `../..`),
// and its files would then be served to the agent as trusted skills; only a path
// the user types on the command line (--source) or writes in their own user
// config may leave the project. Both paths are compared after symlinks are
// resolved, so a link inside the project does not smuggle an outside directory in.
func checkInsideProject(spec Spec, opts Options, root string) error {
	if spec.AllowOutside {
		return nil
	}
	project := opts.ProjectRoot
	if project == "" {
		return oops.Hint("Pass the directory with --source on the command line, or declare it in your user config").
			Errorf("skill source %q: a local source declared in a project config needs a project root to be checked against", spec.Name)
	}
	if abs, err := filepath.Abs(project); err == nil {
		project = abs
	}
	if resolved, err := filepath.EvalSymlinks(project); err == nil {
		project = resolved
	}
	rel, err := filepath.Rel(project, root)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return oops.With("path", root).With("project", project).
			Hint("Pass the directory with --source on the command line, or declare it in your user config").
			Errorf("skill source %q: local path %s is outside the project %s; a local source declared in the project config must stay inside the project (pass the directory with --source, or declare it in your user config)", spec.Name, root, project)
	}
	return nil
}

func resolveGit(ctx context.Context, spec Spec, opts Options) (*Resolved, error) {
	url := gitURL(spec.URL)
	cacheRoot, err := cacheRoot(opts.CacheDir)
	if err != nil {
		return nil, err
	}
	repoDir := filepath.Join(cacheRoot, urlKey(url))
	offline := opts.Offline || opts.Frozen || config.OfflineIncludes(ctx)

	entry := opts.Lock.Find(lockfile.KindSource, spec.Name)
	covered := entry.Covers(spec.Want()) && !opts.Refresh
	if covered && entry.Commit == "" {
		covered = false
	}
	if covered && !lockCommit.MatchString(entry.Commit) {
		return nil, oops.Wrapf(errors.Join(config.ErrLockViolation, errLockCommit),
			"skill source %q: the lock's commit %q is not a full hexadecimal commit SHA; run `ai-rulez lock`", spec.Name, entry.Commit)
	}
	if opts.Frozen && !covered {
		return nil, errLock(spec, "not covered by %s (or the lock is stale); run `ai-rulez lock`", lockfile.FileName)
	}

	commit, kind, err := pickCommit(ctx, spec, opts, commitSearch{url: url, repoDir: repoDir, entry: entry, covered: covered, offline: offline})
	if err != nil {
		return nil, err
	}

	res, err := materialize(ctx, spec, treeRequest{url: url, repoDir: repoDir, commit: commit, kind: kind, entry: entry, covered: covered, offline: offline, maxClone: spec.maxCloneBytes(opts.MaxCloneBytes)})
	if err != nil {
		return nil, err
	}
	if !res.Pinned {
		logger.Warn("Skill source follows a moving ref and is not pinned by the lock (AR010); run `ai-rulez lock`",
			"source", spec.Name, "ref", refLabel(spec.Ref), "commit", commit)
	}
	return res, nil
}

// treeRequest describes the tree of one commit to make available.
type treeRequest struct {
	url, repoDir, commit, kind string
	entry                      *lockfile.Entry
	covered, offline           bool
	maxClone                   int64
}

// materializer makes the tree of one commit available in the cache.
type materializer struct {
	spec    Spec
	q       treeRequest
	treeDir string
	fetched bool
}

// ensure finds the cached tree or, unless offline, fetches it.
func (m *materializer) ensure(ctx context.Context) error {
	m.treeDir = treeDirFor(m.q.repoDir, m.q.commit, m.spec.Path)
	if _, statErr := os.Stat(m.treeDir); statErr == nil {
		return nil
	}
	if m.q.offline {
		return oops.With("url", m.spec.Redacted()).With("commit", m.q.commit).
			Errorf("skill source %q: commit %s is not cached and the network is off (--frozen/--offline); run `ai-rulez lock` or serve once online", m.spec.Name, m.q.commit)
	}
	m.fetched = false
	return fetchInto(ctx, cloneRequest{url: m.q.url, ref: m.spec.Ref, kind: m.q.kind, commit: m.q.commit, path: m.spec.Path, maxBytes: m.q.maxClone, name: m.spec.Name}, m.treeDir, &m.fetched)
}

// materialize makes the tree of a commit available in the cache (fetching it
// unless offline) and verifies it: against the lock when the lock covers the
// source, against the digest sidecar when the ref is a bare commit SHA.
func materialize(ctx context.Context, spec Spec, q treeRequest) (*Resolved, error) {
	m := &materializer{spec: spec, q: q}
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	res, err := finish(spec, m.treeDir, q.commit, q.kind, q.entry, q.covered)
	if err != nil && errors.Is(err, errDigest) && !m.fetched && !q.offline {
		// A damaged cache looks like tampering; fetch the pinned commit again before failing.
		if rmErr := os.RemoveAll(filepath.Join(q.repoDir, q.commit)); rmErr == nil {
			if err = m.ensure(ctx); err == nil {
				res, err = finish(spec, m.treeDir, q.commit, q.kind, q.entry, q.covered)
			}
		}
	}
	if err != nil || q.covered || !fullSHA.MatchString(spec.Ref) {
		return res, err
	}
	return m.verifyUnlocked(ctx, res)
}

// verifyUnlocked checks the cached tree of an unlocked commit-SHA source against
// the digest recorded when it was stored. The SHA proves which commit was asked
// for, not that the files on disk are still that commit's. A tree that does not
// match, or has no record, is fetched again (online) or refused (offline).
func (m *materializer) verifyUnlocked(ctx context.Context, res *Resolved) (*Resolved, error) {
	if m.fetched {
		storeDigest(m.treeDir, m.q.commit, res.Digest)
		return res, nil
	}
	checkErr := checkDigest(m.treeDir, m.q.commit, res.Digest)
	if checkErr == nil {
		return res, nil
	}
	if m.q.offline {
		return nil, oops.With("url", m.spec.Redacted()).With("commit", m.q.commit).
			Wrapf(errors.Join(config.ErrLockViolation, checkErr), "skill source %q: the cached tree cannot be trusted and the network is off (--frozen/--offline); serve once online to repair the cache, or pin the source with `ai-rulez lock`", m.spec.Name)
	}
	logger.Warn("The cached tree of a skill source is not verified; fetching it again", "source", m.spec.Name, "commit", m.q.commit, "reason", checkErr.Error())
	if err := removeTree(m.treeDir); err != nil {
		return nil, err
	}
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	fresh, err := finish(m.spec, m.treeDir, m.q.commit, m.q.kind, m.q.entry, m.q.covered)
	if err != nil {
		return nil, err
	}
	storeDigest(m.treeDir, m.q.commit, fresh.Digest)
	return fresh, nil
}

// commitSearch is what pickCommit needs besides the spec and the options.
type commitSearch struct {
	url, repoDir string
	entry        *lockfile.Entry
	covered      bool
	offline      bool
}

// pickCommit decides which commit of a git source to use: the lock's, a pinned
// SHA, the one last resolved (offline), or whatever the ref points to now.
func pickCommit(ctx context.Context, spec Spec, opts Options, q commitSearch) (commit, kind string, err error) {
	switch {
	case q.covered:
		if fullSHA.MatchString(spec.Ref) && spec.Ref != q.entry.Commit {
			return "", "", errLock(spec, "ref is pinned to %s but the lock records commit %s; run `ai-rulez lock`", spec.Ref, q.entry.Commit)
		}
		return q.entry.Commit, kindFor(spec.Ref), nil
	case fullSHA.MatchString(spec.Ref):
		return spec.Ref, kindSHA, nil
	case q.offline:
		if commit = readRef(q.repoDir, spec.Ref); commit == "" {
			return "", "", oops.With("url", spec.Redacted()).Errorf("skill source %q: offline and %q was never resolved; run once online (or `ai-rulez lock`) first", spec.Name, spec.Ref)
		}
		return commit, kindFor(spec.Ref), nil
	}
	if commit, kind, err = lsRemote(ctx, q.url, spec.Ref); err != nil {
		return "", "", err
	}
	writeRef(q.repoDir, spec.Ref, commit)
	return commit, kind, nil
}

var errDigest = errors.New("content digest mismatch")

// lockCommit is the only shape a lock's commit may have: it becomes a cache path
// component, so anything else (a path traversal) is a violation, never a path.
var lockCommit = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

var errLockCommit = errors.New("invalid commit in the lock")

func finish(spec Spec, treeDir, commit, kind string, entry *lockfile.Entry, covered bool) (*Resolved, error) {
	dir := treeDir
	if spec.Path != "" {
		dir = filepath.Join(treeDir, filepath.FromSlash(spec.Path))
		if err := rejectSymlinkedPath(treeDir, spec.Path); err != nil {
			return nil, oops.With("url", spec.Redacted()).Wrapf(err, "skill source %q", spec.Name)
		}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, oops.With("url", spec.Redacted()).Errorf("skill source %q: path %q does not exist at commit %s", spec.Name, spec.Path, commit)
	}
	digest, err := contentlock.DigestDir(contentlock.KindSkillSource, dir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already wrapped
	}
	if covered && entry.Digest != "" && entry.Digest != digest {
		return nil, oops.Wrapf(errors.Join(config.ErrLockViolation, errDigest),
			"skill source %q: content digest %s does not match the lock's %s; the fetched files changed (run `ai-rulez lock` only after reviewing the change)", spec.Name, digest, entry.Digest)
	}
	res := &Resolved{
		Spec: spec, Dir: dir, Commit: commit, Digest: digest, RefKind: kind,
		Locked: covered, Pinned: covered || fullSHA.MatchString(spec.Ref),
	}
	res.Skills, err = Discover(spec, dir)
	return res, err
}

// rejectSymlinkedPath fails when any component of rel below root is a symlink: a
// fetched repository could otherwise point its skills path at a file tree outside
// the checkout, whose content neither the digest nor the scan would cover.
func rejectSymlinkedPath(root, rel string) error {
	cur := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return nil //nolint:nilerr // a missing path is reported by the caller
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return oops.Errorf("path %q goes through a symlink (%s); symlinks in a fetched repository are not followed", rel, part)
		}
	}
	return nil
}

func fetchInto(ctx context.Context, req cloneRequest, treeDir string, fetched *bool) error {
	commitDir := filepath.Dir(treeDir)
	if err := os.MkdirAll(commitDir, 0o700); err != nil {
		return oops.Wrapf(err, "create skill source cache")
	}
	// A private name per fetch: two servers fetching the same commit do not share a checkout.
	tmp, err := os.MkdirTemp(commitDir, "tree-*.partial")
	if err != nil {
		return oops.Wrapf(err, "create a checkout directory in the skill source cache")
	}
	if err := fetchCommit(ctx, req, tmp); err != nil {
		_ = os.RemoveAll(tmp)    //nolint:errcheck // best-effort cleanup
		_ = os.Remove(commitDir) //nolint:errcheck // best-effort: only removes the directory when nothing else is in it
		return err
	}
	if err := os.Rename(tmp, treeDir); err != nil {
		_ = os.RemoveAll(tmp) //nolint:errcheck // best-effort cleanup
		if _, statErr := os.Stat(treeDir); statErr == nil {
			return nil // another process stored the same commit first; its tree is verified like ours
		}
		return oops.Wrapf(err, "store skill source in the cache")
	}
	*fetched = true
	return nil
}

func kindFor(ref string) string {
	switch {
	case ref == "" || ref == refHEAD:
		return kindHead
	case fullSHA.MatchString(ref):
		return kindSHA
	}
	return kindTag // the exact kind is unknown without the network; both clone the same way
}

func refLabel(ref string) string {
	if ref == "" {
		return "the default branch (HEAD)"
	}
	return ref
}

func cacheRoot(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.CacheDir("skill-sources") //nolint:wrapcheck // already contextual
}

// cacheTree is the directory holding the tree of a commit of url (for the
// source path, "" for the whole repository) below the cache root.
func cacheTree(root, url, commit, srcPath string) string {
	return treeDirFor(filepath.Join(root, urlKey(gitURL(url))), commit, srcPath)
}

func urlKey(url string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(url, "/")))
	return hex.EncodeToString(sum[:])[:16]
}

// refs.json remembers what each ref last resolved to, for offline use.
func readRef(repoDir, ref string) string {
	data, err := os.ReadFile(filepath.Join(repoDir, "refs.json"))
	if err != nil {
		return ""
	}
	m := map[string]string{}
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return m[refLabel(ref)]
}

func writeRef(repoDir, ref, commit string) {
	path := filepath.Join(repoDir, "refs.json")
	m := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &m) //nolint:errcheck // a damaged index is rewritten
	}
	m[refLabel(ref)] = commit
	data, err := json.Marshal(m)
	if err != nil || os.MkdirAll(repoDir, 0o700) != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600) //nolint:errcheck // the index is only a convenience for offline runs
}

// Problem is one way the lock disagrees with the configured sources.
type Problem struct{ Name, Message string }

func (p Problem) String() string { return "source " + p.Name + ": " + p.Message }

// Wants lists the sources a lock should cover.
func Wants(sources []config.SkillSourceConfig) []lockfile.Want {
	out := make([]lockfile.Want, 0, len(sources))
	for i := range sources {
		out = append(out, FromConfig(&sources[i]).Want())
	}
	return out
}

// CheckLock compares the lock with the configured sources without the network:
// a source must be covered, and a source already in the cache must match its
// digest. Lock entries of sources that are no longer configured are reported.
func CheckLock(sources []config.SkillSourceConfig, lock *lockfile.File, cacheDir string) []Problem {
	var problems []Problem
	configured := map[string]bool{}
	for i := range sources {
		spec := FromConfig(&sources[i])
		configured[spec.Name] = true
		entry := lock.Find(lockfile.KindSource, spec.Name)
		switch {
		case lock == nil:
			problems = append(problems, Problem{spec.Name, "not pinned: " + lockfile.FileName + " does not exist"})
		case entry == nil:
			problems = append(problems, Problem{spec.Name, "not covered by the lock"})
		case !entry.Covers(spec.Want()):
			problems = append(problems, Problem{spec.Name, "lock is stale: url, path or ref changed since it was written"})
		case spec.IsGit() && !lockCommit.MatchString(entry.Commit):
			problems = append(problems, Problem{spec.Name, fmt.Sprintf("the lock's commit %q is not a full hexadecimal commit SHA", entry.Commit)})
		case spec.IsGit():
			root, _ := cacheRoot(cacheDir) //nolint:errcheck // never fails
			tree := cacheTree(root, spec.URL, entry.Commit, spec.Path)
			if spec.Path != "" {
				tree = filepath.Join(tree, filepath.FromSlash(spec.Path))
			}
			if _, err := os.Stat(tree); err != nil {
				continue
			}
			if d, err := contentlock.DigestDir(contentlock.KindSkillSource, tree); err == nil && d != entry.Digest {
				problems = append(problems, Problem{spec.Name, fmt.Sprintf("cached content digest %s does not match the lock's %s", d, entry.Digest)})
			}
		}
	}
	if lock != nil {
		for _, e := range lock.Source {
			if !configured[e.Name] {
				problems = append(problems, Problem{e.Name, "in the lock but no longer configured"})
			}
		}
	}
	return problems
}
