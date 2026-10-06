package includes

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/samber/oops"
)

// LockMode says how resolution treats ai-rulez.lock.
type LockMode int

const (
	// LockAuto uses the lock when one covers a source and fetches an uncovered
	// source as before. Without a lock file nothing changes.
	LockAuto LockMode = iota
	// LockRequire (generate --locked) fails when the lock is missing or does not
	// cover a configured remote source.
	LockRequire
	// LockFrozen (generate --frozen) is LockRequire that never touches the
	// network; the caller also sets SkipFetch.
	LockFrozen
	// LockRefresh (ai-rulez lock) re-resolves the sources selected by
	// RefreshFilter from the remote, ignoring their pins, so new pins can be recorded.
	LockRefresh
)

// Lock policy for this process, set from the CLI before any config is loaded.
var (
	Mode LockMode
	// RefreshFilter selects what LockRefresh re-resolves; nil selects everything.
	RefreshFilter func(kind, name string) bool
	// RequireWhenEnforced makes an enforced lock ([lock] enforce, on by default
	// when ai-rulez.lock exists) behave like LockRequire: a remote source the
	// lock does not cover is a violation instead of an unpinned fetch. `generate`
	// sets it; commands that only read or report leave it off.
	RequireWhenEnforced bool
)

// observed is what a fetch actually resolved to, recorded so `ai-rulez lock`
// can write it down.
type observed struct{ commit, digest string }

var (
	observedMu sync.Mutex
	observedBy = map[string]observed{}
)

func observedKey(baseDir, kind, name string) string { return baseDir + "\x00" + kind + "\x00" + name }

func record(baseDir, kind, name string, o observed) {
	observedMu.Lock()
	observedBy[observedKey(baseDir, kind, name)] = o
	observedMu.Unlock()
}

// ResetObserved forgets the recorded resolutions.
func ResetObserved() {
	observedMu.Lock()
	observedBy = map[string]observed{}
	observedMu.Unlock()
	resetTags()
}

// refreshing reports whether the source is being re-resolved by `ai-rulez lock`.
func refreshing(kind, name string) bool {
	return Mode == LockRefresh && (RefreshFilter == nil || RefreshFilter(kind, name))
}

// Lockable lists the configured remote sources a lock should cover: git
// includes and git installed skills. Local paths live in the repository and need
// no pin.
func Lockable(cfg *config.Config) []lockfile.Want {
	var wants []lockfile.Want
	for i := range cfg.Includes {
		inc := &cfg.Includes[i]
		if DetectSourceType(inc.Source) == SourceTypeGit {
			wants = append(wants, withVersion(lockfile.Want{Kind: lockfile.KindInclude, Name: inc.Name, Source: lockSource(cfg.BaseDir, inc.Source), Path: inc.Path, Ref: inc.Ref}, inc.VersionSpec()))
		}
	}
	for i := range cfg.InstalledSkills {
		sk := &cfg.InstalledSkills[i]
		if DetectSourceType(sk.Source) == SourceTypeGit {
			wants = append(wants, withVersion(lockfile.Want{Kind: lockfile.KindSkill, Name: sk.Name, Source: lockSource(cfg.BaseDir, sk.Source), Path: sk.GetPath(), Ref: sk.Ref}, sk.VersionSpec()))
		}
	}
	return wants
}

// lockSource is the source as the lock records it: credentials redacted, and a
// file:// URL written relative to the project (file://./vendor/x, file://../x)
// so the lock does not carry a machine-specific absolute path.
func lockSource(baseDir, source string) string {
	redacted := RedactURL(source)
	prefix := ""
	rest := redacted
	if after, ok := strings.CutPrefix(rest, "git+"); ok {
		prefix, rest = "git+", after
	}
	path, ok := strings.CutPrefix(rest, "file://")
	if !ok || !filepath.IsAbs(path) || baseDir == "" {
		return redacted
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return redacted
	}
	rel, err := filepath.Rel(base, filepath.Clean(path))
	if err != nil {
		return redacted
	}
	rel = filepath.ToSlash(rel)
	if rel != "." && !strings.HasPrefix(rel, "../") {
		rel = "./" + rel
	}
	return prefix + "file://" + rel
}

// withVersion turns a want into a version-constraint want: the constraint takes
// the place of the ref, which is what the lock records as the requested ref.
func WithVersion(w lockfile.Want, v config.VersionSpec) lockfile.Want { return withVersion(w, v) }

func withVersion(w lockfile.Want, v config.VersionSpec) lockfile.Want {
	if !v.Active() {
		return w
	}
	w.Ref, w.Constraint, w.TagPrefix, w.IncludePrerelease = v.Constraint, v.Constraint, v.TagPrefix, v.IncludePrerelease
	return w
}

// strictLock reports whether a source that cannot be resolved must fail the run
// instead of being skipped with a warning: generate --locked, --frozen, or an
// enforced lock under `generate`.
func strictLock(cfg *config.Config) bool {
	return Mode == LockRequire || Mode == LockFrozen || (RequireWhenEnforced && cfg.LockEnforced())
}

// pin is the resolved lock entry a source must match.
type pin struct {
	entry lockfile.Entry
	kind  string
	name  string
}

// pinFor decides how one git source is fetched. It returns the pin to enforce
// (nil to fetch unpinned) or a lock violation.
func pinFor(cfg *config.Config, lock *lockfile.File, w lockfile.Want) (*pin, error) {
	if refreshing(w.Kind, w.Name) {
		return nil, nil
	}
	entry := lock.Find(w.Kind, w.Name)
	switch {
	case entry.Covers(w):
		if lockfile.IsFullSHA(w.Ref) && w.Ref != entry.Commit {
			return nil, violation(w, "ref is pinned to %s but the lock records commit %s; run `ai-rulez lock`", w.Ref, entry.Commit)
		}
		return &pin{entry: *entry, kind: w.Kind, name: w.Name}, nil
	case strictLock(cfg):
		if lock == nil {
			return nil, violation(w, "%s not found in %s; run `ai-rulez lock` and commit it", lockfile.FileName, cfg.ConfigDir)
		}
		if entry == nil {
			return nil, violation(w, "not covered by %s; run `ai-rulez lock`", lockfile.FileName)
		}
		return nil, violation(w, "%s is stale (source, path or ref changed); run `ai-rulez lock`", lockfile.FileName)
	default:
		return nil, nil
	}
}

func violation(w lockfile.Want, format string, args ...any) error {
	return violationWith(nil, w, format, args...)
}

// errDigestMismatch tags a violation whose cause is the fetched files, which a
// fresh fetch of the pinned commit may repair (a damaged cache) or confirm (a
// remote that serves different bytes for the same commit).
var errDigestMismatch = errors.New("content digest mismatch")

func violationWith(cause error, w lockfile.Want, format string, args ...any) error {
	msg := fmt.Sprintf("%s %q: %s", w.Kind, w.Name, fmt.Sprintf(format, args...))
	if cause != nil {
		return oops.Wrapf(errors.Join(config.ErrLockViolation, cause), "%s", msg)
	}
	return oops.Wrapf(config.ErrLockViolation, "%s", msg)
}

// retryable reports whether a failed pin check is worth one fresh fetch: the
// digest differed and the network may be used.
func retryable(ctx context.Context, err error) bool {
	return errors.Is(err, errDigestMismatch) && !SkipFetch && !config.OfflineIncludes(ctx)
}

// effectiveRef is the ref to fetch: the locked commit when pinned.
func (p *pin) effectiveRef(requested string) string {
	if p != nil && p.entry.Commit != "" {
		return p.entry.Commit
	}
	return requested
}

// check records what a fetch resolved to and, for a pinned source, fails when
// the commit or the content digest differs from the lock.
func (p *pin) check(baseDir, kind, name, commit, digest string) error {
	record(baseDir, kind, name, observed{commit: commit, digest: digest})
	if p == nil {
		return nil
	}
	w := lockfile.Want{Kind: kind, Name: name}
	if p.entry.Commit != "" && commit != p.entry.Commit {
		return violation(w, "fetched commit %s but the lock records %s; the cache is stale or the remote cannot serve the pinned commit (run `ai-rulez lock` to accept a new one)", commit, p.entry.Commit)
	}
	if p.entry.Digest != "" && digest != p.entry.Digest {
		return violationWith(errDigestMismatch, w, "content digest %s does not match the lock's %s; the fetched files changed (run `ai-rulez lock` only after reviewing the change)", digest, p.entry.Digest)
	}
	return nil
}

// BuildLock turns the recorded resolutions of cfg's sources into a lock file.
// Entries not selected by RefreshFilter keep their current pin. The returned
// problems name every source that could not be locked.
func BuildLock(cfg *config.Config, current *lockfile.File) (lock *lockfile.File, problems []string) {
	out := &lockfile.File{Version: lockfile.Version}
	for _, w := range Lockable(cfg) {
		if !refreshing(w.Kind, w.Name) {
			if e := current.Find(w.Kind, w.Name); e.Covers(w) {
				out.Set(w.Kind, *e)
				continue
			}
		}
		observedMu.Lock()
		o, ok := observedBy[observedKey(cfg.BaseDir, w.Kind, w.Name)]
		observedMu.Unlock()
		if !ok || o.commit == "" || o.digest == "" {
			if msg := recordedProblem(cfg.BaseDir, w.Kind, w.Name); msg != "" {
				problems = append(problems, fmt.Sprintf("%s %q: %s", w.Kind, w.Name, msg))
			} else {
				problems = append(problems, fmt.Sprintf("%s %q could not be resolved; see the warnings above", w.Kind, w.Name))
			}
			continue
		}
		entry := lockfile.Entry{Name: w.Name, Source: w.Source, Path: w.Path, Ref: w.Ref, Commit: o.commit, Digest: o.digest}
		if w.Constraint != "" {
			t, _ := recordedTag(cfg.BaseDir, w.Kind, w.Name)
			entry.Tag, entry.TagObject = t.tag, t.tagObject
		}
		out.Set(w.Kind, entry)
	}
	sort.Strings(problems)
	return out, problems
}

// Problem is one way the lock disagrees with the configuration or the cache.
type Problem struct {
	Kind, Name, Message string
}

func (p Problem) String() string { return fmt.Sprintf("%s %s: %s", p.Kind, p.Name, p.Message) }

// CheckLock compares the lock with the configuration and, for sources whose
// content is already cached, with the cached files. It never uses the network.
// cached counts the sources whose digest was verified.
func CheckLock(cfg *config.Config, lock *lockfile.File) (problems []Problem, cached int) {
	wants := Lockable(cfg)
	if len(wants) == 0 && lock == nil {
		return nil, 0
	}
	if lock == nil {
		for _, w := range wants {
			problems = append(problems, Problem{w.Kind, w.Name, "not pinned: " + lockfile.FileName + " does not exist"})
		}
		return problems, 0
	}
	configured := map[string]bool{}
	for _, w := range wants {
		configured[w.Kind+"\x00"+w.Name] = true
		entry := lock.Find(w.Kind, w.Name)
		switch {
		case entry == nil:
			problems = append(problems, Problem{w.Kind, w.Name, "not covered by the lock"})
			continue
		case !entry.Covers(w):
			problems = append(problems, Problem{w.Kind, w.Name, "lock is stale: source, path or ref changed since it was written"})
			continue
		}
		digest, commit, ok, err := cachedState(cfg, w)
		if err != nil {
			problems = append(problems, Problem{w.Kind, w.Name, "cannot digest the cached content: " + err.Error()})
			continue
		}
		if !ok {
			continue
		}
		cached++
		switch {
		case commit != "" && entry.Commit != "" && commit != entry.Commit:
			problems = append(problems, Problem{w.Kind, w.Name, fmt.Sprintf("cache is at commit %s, lock records %s", commit, entry.Commit)})
		case digest != entry.Digest:
			problems = append(problems, Problem{w.Kind, w.Name, fmt.Sprintf("cached content digest %s does not match the lock's %s", digest, entry.Digest)})
		}
	}
	problems = append(problems, unconfiguredEntries(lock, configured)...)
	sort.Slice(problems, func(i, j int) bool { return problems[i].String() < problems[j].String() })
	return problems, cached
}

// cachedTree locates the cached tree of a source without fetching: the directory
// that is digested, the cache directory holding its bookkeeping and the tree kind.
func cachedTree(cfg *config.Config, w lockfile.Want) (dir, cacheDir, treeKind string) {
	treeKind = contentlock.KindInstalledSkill
	switch w.Kind {
	case lockfile.KindInclude:
		for i := range cfg.Includes {
			if cfg.Includes[i].Name != w.Name {
				continue
			}
			newSource := NewGitSource
			treeKind = contentlock.KindInclude
			if cfg.Includes[i].Format == config.IncludeFormatOKF {
				newSource, treeKind = NewOKFGitSource, contentlock.KindOKFInclude
			}
			src, err := newSource(w.Name, cfg.Includes[i].Source, cfg.Includes[i].Path, cfg.Includes[i].Ref, cfg.BaseDir, nil, "")
			if err != nil {
				return "", "", treeKind
			}
			dir, cacheDir = src.findAIRulezDir(), src.cacheDir
		}
	case lockfile.KindSkill:
		for i := range cfg.InstalledSkills {
			if cfg.InstalledSkills[i].Name != w.Name {
				continue
			}
			sk := &cfg.InstalledSkills[i]
			src, err := NewSkillGitSource(w.Name, sk.Source, sk.GetPath(), sk.Ref, "")
			if err != nil {
				return "", "", treeKind
			}
			dir, cacheDir = src.findSkillDir(), src.cacheDir
		}
	}
	return dir, cacheDir, treeKind
}

// cachedCommit is the commit the cache at cacheDir was fetched at ("" unknown).
func cachedCommit(cacheDir string) string {
	if meta, err := readCacheMeta(cacheDir); err == nil && meta != nil {
		return meta.RemoteHEADSHA
	}
	return ""
}

// cachedState reads the cached tree of a source without fetching. ok is false
// when nothing is cached; a tree that is cached but cannot be digested is an
// error, not "not cached".
func cachedState(cfg *config.Config, w lockfile.Want) (digest, commit string, ok bool, err error) {
	dir, cacheDir, treeKind := cachedTree(cfg, w)
	if dir == "" {
		return "", "", false, nil
	}
	d, err := contentlock.DigestDir(treeKind, dir)
	if err != nil {
		return "", "", false, oops.With("dir", dir).Wrapf(err, "digest cached %s %s", w.Kind, w.Name)
	}
	return d, cachedCommit(cacheDir), true, nil
}

// NotCached lists the remote sources whose content is not in the local cache.
func NotCached(cfg *config.Config) []lockfile.Want {
	var out []lockfile.Want
	for _, w := range Lockable(cfg) {
		if dir, _, _ := cachedTree(cfg, w); dir == "" {
			out = append(out, w)
		}
	}
	return out
}

// FormatProblems renders problems one per line.
func FormatProblems(ps []Problem) string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = "  " + p.String()
	}
	return strings.Join(lines, "\n")
}

// loadLockFor reads the project's lock, but only when it has remote sources. A
// lock file that cannot be read is a violation: pins must never be skipped silently.
func loadLockFor(cfg *config.Config) (*lockfile.File, error) {
	if len(Lockable(cfg)) == 0 || cfg.ConfigDir == "" {
		return nil, nil
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, oops.Wrapf(errors.Join(config.ErrLockViolation, err), "read %s", lockfile.FileName)
	}
	return lock, nil
}

// Unpinned lists the remote sources that follow a moving ref and are not
// covered by ai-rulez.lock. A full commit SHA counts as pinned. An unreadable
// lock leaves every moving source unpinned.
func Unpinned(cfg *config.Config) []lockfile.Want {
	wants := Lockable(cfg)
	if len(wants) == 0 {
		return nil
	}
	lock, _ := lockfile.Load(cfg.ConfigDir) //nolint:errcheck // an unreadable lock pins nothing
	var out []lockfile.Want
	for _, w := range wants {
		if lockfile.IsFullSHA(w.Ref) || lock.Find(w.Kind, w.Name).Covers(w) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// unconfiguredEntries reports lock entries whose source is no longer configured.
func unconfiguredEntries(lock *lockfile.File, configured map[string]bool) []Problem {
	var out []Problem
	for _, list := range []struct {
		kind    string
		entries []lockfile.Entry
	}{{lockfile.KindInclude, lock.Include}, {lockfile.KindSkill, lock.Skill}} {
		for _, e := range list.entries {
			if !configured[list.kind+"\x00"+e.Name] {
				out = append(out, Problem{list.kind, e.Name, "in the lock but no longer configured"})
			}
		}
	}
	return out
}
