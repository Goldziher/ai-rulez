package includes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/samber/oops"
)

// Policy of a refresh over sources that ask for a version range. They are set by
// `ai-rulez update` and are zero for `ai-rulez lock`, which keeps a pin that
// still satisfies its constraint.
var (
	// Advance selects the sources a refresh moves to the newest allowed tag.
	// nil moves none: a pin that still satisfies its constraint is kept.
	Advance func(kind, name string) bool
	// AllowDowngrade lets Advance select a tag with lower precedence than the pin.
	AllowDowngrade bool
	// AcceptMovedTag re-pins a tag that now points to another commit instead of
	// failing with AR732.
	AcceptMovedTag bool
	// ReleaseGate returns the minimum release age gate for a source, or nil for
	// none. `lock` and `update` set it; it is only consulted when a range is
	// resolved to a new tag (a pin that still satisfies its constraint is kept
	// without a lookup, its age having been decided when it was pinned).
	ReleaseGate func(w lockfile.Want) *tagresolve.AgeGate
)

// tagInfo is the tag a version constraint resolved to.
type tagInfo struct {
	tag, tagObject string
	// released and releasedFrom are the recorded release time (RFC 3339) and its source.
	released, releasedFrom string
}

var (
	tagsMu   sync.Mutex
	tagsBy   = map[string]tagInfo{}
	problems = map[string]string{}
)

func recordTag(baseDir, kind, name string, t tagInfo) {
	tagsMu.Lock()
	tagsBy[observedKey(baseDir, kind, name)] = t
	tagsMu.Unlock()
}

func recordProblem(baseDir, kind, name, msg string) {
	tagsMu.Lock()
	problems[observedKey(baseDir, kind, name)] = msg
	tagsMu.Unlock()
}

func resetTags() {
	tagsMu.Lock()
	tagsBy, problems = map[string]tagInfo{}, map[string]string{}
	tagsMu.Unlock()
}

func recordedTag(baseDir, kind, name string) (tagInfo, bool) {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	t, ok := tagsBy[observedKey(baseDir, kind, name)]
	return t, ok
}

func recordedProblem(baseDir, kind, name string) string {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	return problems[observedKey(baseDir, kind, name)]
}

// TagSpec converts a want's constraint to the resolver's spec.
func TagSpec(w lockfile.Want) tagresolve.Spec {
	return tagresolve.Spec{Constraint: w.Constraint, TagPrefix: w.TagPrefix, IncludePrerelease: w.IncludePrerelease}
}

// runGit runs git for tag listing the way an include fetch does, with env.
func runGit(ctx context.Context, env []string, args ...string) (string, error) {
	res := gitRun(ctx, "", env, args...)
	if err := gitutil.ResultErr(res); err != nil {
		return "", oops.With("output", RedactURL(strings.TrimSpace(string(res.Stderr)))).Wrapf(err, "git %s", args[0])
	}
	return string(res.Stdout), nil
}

// ListRemoteTags lists the tags of a git source (the token goes in a scoped header, never the URL).
func ListRemoteTags(ctx context.Context, repoURL, token string) ([]tagresolve.RawTag, error) {
	if err := checkRemoteArgs(repoURL, ""); err != nil {
		return nil, err
	}
	if err := requireGit(ctx); err != nil {
		return nil, err
	}
	runner := func(ctx context.Context, args ...string) (string, error) {
		return runGit(ctx, withAuth(ctx, gitEnvFor(ctx), repoURL, token), args...)
	}
	tags, err := tagresolve.ListTags(ctx, runner, repoURL)
	if err != nil {
		return nil, oops.With("url", RedactURL(repoURL)).Wrap(err)
	}
	return tags, nil
}

// offline reports whether the run may not use the network.
func offline(ctx context.Context) bool { return config.OfflineIncludes(ctx) }

// versionRef returns what to fetch for want w, resolving its version constraint
// when it has one. A source with a plain ref keeps the old behavior: the locked
// commit when pinned, else the ref. A constraint is resolved only here, on the
// paths that may move a pin (`lock` for a source the lock does not cover or an
// `update` that advances it); a covered pin is used as it is.
func versionRef(ctx context.Context, cfg *config.Config, lock *lockfile.File, w lockfile.Want, p *pin, repoURL, token, baseDir string) (string, error) {
	if w.Constraint == "" {
		return p.effectiveRef(w.Ref), nil
	}
	if p != nil {
		recordTag(baseDir, w.Kind, w.Name, tagInfo{p.entry.Tag, p.entry.TagObject, p.entry.Released, p.entry.ReleasedFrom})
		return p.entry.Commit, nil
	}
	ref, info, err := resolveConstraint(ctx, refreshing(cfg, w.Kind, w.Name), lock, w, repoURL, token)
	if err != nil {
		recordProblem(baseDir, w.Kind, w.Name, err.Error())
		return "", violationOrPlain(w, err)
	}
	recordTag(baseDir, w.Kind, w.Name, info)
	return ref, nil
}

func violationOrPlain(w lockfile.Want, err error) error {
	return oops.Wrapf(err, "%s %q", w.Kind, w.Name)
}

// RunMode says how a resolution run may behave.
type RunMode struct {
	// Refresh: the run may move the pin (`lock`, `update`).
	Refresh bool
	// Offline: the network may not be used.
	Offline bool
}

// Resolution is the commit and tag a version constraint resolved to.
type Resolution struct {
	Commit, Tag, TagObject string
	// Released and ReleasedFrom are the release time of Tag (RFC 3339) and its
	// source, when a min_release_age looked it up or the pin already had it.
	Released, ReleasedFrom string
	// Held lists the newer tags min_release_age held back (AR733).
	Held []tagresolve.Held
}

// resolveConstraint picks the commit of a constraint source that has no usable pin.
func resolveConstraint(ctx context.Context, refresh bool, lock *lockfile.File, w lockfile.Want, repoURL, token string) (string, tagInfo, error) {
	res, err := ResolveVersion(ctx, lock, w, RunMode{Refresh: refresh, Offline: offline(ctx)}, func(ctx context.Context) ([]tagresolve.RawTag, error) {
		return ListRemoteTags(ctx, repoURL, token)
	})
	return res.Commit, tagInfo{res.Tag, res.TagObject, res.Released, res.ReleasedFrom}, err
}

// ResolveVersion resolves w's constraint against the tags list returns. A
// Refresh run may move the pin (`lock`, `update`): a pin that still satisfies
// the constraint is then kept, unless Advance selects the source, after
// checking that its tag was not moved. Offline, only a kept pin resolves.
func ResolveVersion(ctx context.Context, lock *lockfile.File, w lockfile.Want, run RunMode, list func(context.Context) ([]tagresolve.RawTag, error)) (Resolution, error) {
	entry := lock.Find(w.Kind, w.Name)
	advance := Advance != nil && Advance(w.Kind, w.Name)
	keep := run.Refresh && entry.Covers(w) && !advance
	if run.Offline {
		if keep {
			return keptResolution(entry), nil
		}
		return Resolution{}, oops.Hint("Run `ai-rulez lock` with network access").
			Errorf("version constraint %q of %s %q cannot be resolved offline", w.Constraint, w.Kind, w.Name)
	}
	tags, err := list(ctx)
	if err != nil {
		return Resolution{}, err
	}
	if keep {
		commit, t, err := keepPin(ctx, entry, tags, w)
		return Resolution{Commit: commit, Tag: t.tag, TagObject: t.tagObject, Released: t.released, ReleasedFrom: t.releasedFrom}, err
	}
	spec := TagSpec(w)
	if entry != nil {
		spec.Pinned = entry.Tag
	}
	var gate *tagresolve.AgeGate
	if ReleaseGate != nil {
		gate = ReleaseGate(w)
	}
	sel, err := tagresolve.SelectGated(ctx, tags, spec, gate)
	if err != nil {
		return Resolution{}, err //nolint:wrapcheck // carries the rule code
	}
	for _, n := range sel.Notes {
		logger.FromContext(ctx).Warn("Ambiguous version tags", "source", w.Name, "note", n)
	}
	for _, h := range sel.Held {
		logger.FromContext(ctx).Info(h.String(), "source", w.Name, "min_release_age", gate.Min.String())
	}
	if err := refuseDowngrade(entry, sel.Chosen, w); err != nil {
		return Resolution{}, err
	}
	c := sel.Chosen.Tag
	res := Resolution{Commit: c.Commit, Tag: c.Name, TagObject: c.TagObject(), Held: sel.Held}
	switch {
	case sel.Release != nil:
		res.Released, res.ReleasedFrom = sel.Release.At.UTC().Format(time.RFC3339), sel.Release.From
	case entry != nil && entry.Tag == c.Name && entry.Commit == c.Commit:
		// The pinned tag was chosen again (exempt from the gate): its record stays.
		res.Released, res.ReleasedFrom = entry.Released, entry.ReleasedFrom
	}
	return res, nil
}

// keptResolution is the resolution of a pin that is kept as it is.
func keptResolution(e *lockfile.Entry) Resolution {
	return Resolution{Commit: e.Commit, Tag: e.Tag, TagObject: e.TagObject, Released: e.Released, ReleasedFrom: e.ReleasedFrom}
}

// refuseDowngrade stops an update that would select a tag below the pinned one:
// a truncated tag list (an attacker, a mirror) must not roll a pin back.
func refuseDowngrade(entry *lockfile.Entry, chosen tagresolve.Candidate, w lockfile.Want) error {
	if entry == nil || entry.Tag == "" || AllowDowngrade || Advance == nil {
		return nil
	}
	cur, ok := semver.ParseTag(entry.Tag, w.TagPrefix)
	if !ok || chosen.Version.Compare(cur) >= 0 {
		return nil
	}
	return oops.Hint("Pass --allow-downgrade if the lower tag is intended").
		Errorf("refusing to move %s %q from %s down to %s: a lower version can come from a truncated tag list", w.Kind, w.Name, entry.Tag, chosen.Tag.Name)
}

// keepPin keeps the pinned commit after checking that its tag was not moved.
func keepPin(ctx context.Context, entry *lockfile.Entry, tags []tagresolve.RawTag, w lockfile.Want) (string, tagInfo, error) {
	status, now := tagresolve.Check(tags, entry.Tag, entry.Commit)
	switch status {
	case tagresolve.StatusMoved:
		if !AcceptMovedTag {
			return "", tagInfo{}, tagresolve.MovedError(entry.Tag, entry.Commit, now.Commit)
		}
		logger.FromContext(ctx).Warn("Accepted a moved tag", "source", w.Name, "tag", entry.Tag, "was", shortSHA(entry.Commit), "now", shortSHA(now.Commit))
		return now.Commit, tagInfo{tag: now.Name, tagObject: now.TagObject()}, nil
	case tagresolve.StatusMissing:
		logger.FromContext(ctx).Warn(fmt.Sprintf("%s tag %q of %s %q no longer exists on the remote; keeping the pinned commit", tagresolve.CodeLockedTagMissed, entry.Tag, w.Kind, w.Name))
	case tagresolve.StatusOK:
	}
	return entry.Commit, tagInfo{entry.Tag, entry.TagObject, entry.Released, entry.ReleasedFrom}, nil
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// VersionSource is a configured git source that asks for a version range.
type VersionSource struct {
	Want lockfile.Want
	// URL is the source as configured (credentials are never written anywhere).
	URL string
}

// VersionSources lists the includes and installed skills of cfg that use a version constraint.
func VersionSources(cfg *config.Config) []VersionSource {
	var out []VersionSource
	urls := map[string]string{}
	for i := range cfg.Includes {
		urls[lockfile.KindInclude+"\x00"+cfg.Includes[i].Name] = stripGitPlus(cfg.Includes[i].Source)
	}
	for i := range cfg.InstalledSkills {
		urls[lockfile.KindSkill+"\x00"+cfg.InstalledSkills[i].Name] = stripGitPlus(cfg.InstalledSkills[i].Source)
	}
	wants := Lockable(cfg)
	for i := range wants {
		w := &wants[i]
		if w.Constraint != "" {
			out = append(out, VersionSource{Want: *w, URL: urls[w.Kind+"\x00"+w.Name]})
		}
	}
	return out
}

// CachedTreeDir is the directory of the cached tree of a locked source ("" when
// nothing is cached): what `update` compares before and after a refresh.
func CachedTreeDir(cfg *config.Config, w lockfile.Want) string {
	dir, _, _ := cachedTree(cfg, w)
	return dir
}

// FileHashes returns path -> content hash of every regular file below dir, with
// the git metadata and the cache bookkeeping file left out. Paths are relative
// to dir and "/"-separated.
func FileHashes(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == cacheMetaFile || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // a file of the cache directory
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "hash the cached tree")
	}
	return out, nil
}
