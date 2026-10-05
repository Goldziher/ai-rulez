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
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
	"github.com/Goldziher/ai-rulez/internal/logger"
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
	// Token authenticates HTTPS fetches.
	Token string
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
	root := spec.URL
	if spec.Path != "" {
		root = filepath.Join(root, filepath.FromSlash(spec.Path))
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve local skill source %q", spec.Name)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, oops.With("path", root).Errorf("skill source %q: %s is not a directory", spec.Name, root)
	}
	digest, err := lockfile.DigestDir(root)
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
	if opts.Frozen && !covered {
		return nil, errLock(spec, "not covered by %s (or the lock is stale); run `ai-rulez lock`", lockfile.FileName)
	}

	var commit, kind string
	switch {
	case covered:
		commit, kind = entry.Commit, kindFor(spec.Ref)
		if fullSHA.MatchString(spec.Ref) && spec.Ref != commit {
			return nil, errLock(spec, "ref is pinned to %s but the lock records commit %s; run `ai-rulez lock`", spec.Ref, commit)
		}
	case fullSHA.MatchString(spec.Ref):
		commit, kind = spec.Ref, kindSHA
	case offline:
		commit = readRef(repoDir, spec.Ref)
		kind = kindFor(spec.Ref)
		if commit == "" {
			return nil, oops.With("url", spec.Redacted()).Errorf("skill source %q: offline and %q was never resolved; run once online (or `ai-rulez lock`) first", spec.Name, spec.Ref)
		}
	default:
		if commit, kind, err = lsRemote(ctx, url, spec.Ref, opts.Token); err != nil {
			return nil, err
		}
		writeRef(repoDir, spec.Ref, commit)
	}

	treeDir := filepath.Join(repoDir, commit, "tree")
	fetched := false
	ensure := func() error {
		if _, statErr := os.Stat(treeDir); statErr == nil {
			return nil
		}
		if offline {
			return oops.With("url", spec.Redacted()).With("commit", commit).
				Errorf("skill source %q: commit %s is not cached and the network is off (--frozen/--offline); run `ai-rulez lock` or serve once online", spec.Name, commit)
		}
		return fetchInto(ctx, url, spec.Ref, kind, commit, opts.Token, treeDir, &fetched)
	}
	if err := ensure(); err != nil {
		return nil, err
	}

	res, err := finish(spec, treeDir, commit, kind, entry, covered)
	if err != nil && errors.Is(err, errDigest) && !fetched && !offline {
		// A damaged cache looks like tampering; fetch the pinned commit again before failing.
		if rmErr := os.RemoveAll(filepath.Join(repoDir, commit)); rmErr == nil {
			if err = ensure(); err == nil {
				res, err = finish(spec, treeDir, commit, kind, entry, covered)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if !res.Pinned {
		logger.Warn("Skill source follows a moving ref and is not pinned by the lock (AR010); run `ai-rulez lock`",
			"source", spec.Name, "ref", refLabel(spec.Ref), "commit", commit)
	}
	return res, nil
}

var errDigest = errors.New("content digest mismatch")

func finish(spec Spec, treeDir, commit, kind string, entry *lockfile.Entry, covered bool) (*Resolved, error) {
	dir := treeDir
	if spec.Path != "" {
		dir = filepath.Join(treeDir, filepath.FromSlash(spec.Path))
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, oops.With("url", spec.Redacted()).Errorf("skill source %q: path %q does not exist at commit %s", spec.Name, spec.Path, commit)
	}
	digest, err := lockfile.DigestDir(dir)
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

func fetchInto(ctx context.Context, url, ref, kind, commit, token, treeDir string, fetched *bool) error {
	if err := os.MkdirAll(filepath.Dir(treeDir), 0o755); err != nil {
		return oops.Wrapf(err, "create skill source cache")
	}
	tmp := treeDir + ".partial"
	_ = os.RemoveAll(tmp) //nolint:errcheck // a stale partial checkout is simply replaced
	if err := fetchCommit(ctx, url, ref, kind, commit, token, tmp); err != nil {
		_ = os.RemoveAll(tmp) //nolint:errcheck // best-effort cleanup
		return err
	}
	if err := os.Rename(tmp, treeDir); err != nil {
		return oops.Wrapf(err, "store skill source in the cache")
	}
	*fetched = true
	return nil
}

func kindFor(ref string) string {
	switch {
	case ref == "" || ref == "HEAD":
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
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".cache", "ai-rulez", "skill-sources"), nil
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
	if err != nil || os.MkdirAll(repoDir, 0o755) != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644) //nolint:errcheck,gosec // the index is only a convenience for offline runs
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
		case spec.IsGit():
			root, _ := cacheRoot(cacheDir) //nolint:errcheck // never fails
			tree := filepath.Join(root, urlKey(gitURL(spec.URL)), entry.Commit, "tree")
			if spec.Path != "" {
				tree = filepath.Join(tree, filepath.FromSlash(spec.Path))
			}
			if _, err := os.Stat(tree); err != nil {
				continue
			}
			if d, err := lockfile.DigestDir(tree); err == nil && d != entry.Digest {
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
