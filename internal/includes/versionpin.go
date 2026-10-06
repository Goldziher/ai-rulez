package includes

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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
)

// tagInfo is the tag a version constraint resolved to.
type tagInfo struct{ tag, tagObject string }

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

// gitRunner runs git for tag listing the way an include fetch does.
func gitRunner(ctx context.Context, args ...string) (string, error) {
	cmd := gitCmd(ctx, "", args...)
	cmd.Env = gitEnvFor(ctx)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", oops.With("output", RedactURL(strings.TrimSpace(errOut.String()))).Wrapf(err, "git %s", args[0])
	}
	return out.String(), nil
}

// ListRemoteTags lists the tags of a git source (token injected for https).
func ListRemoteTags(ctx context.Context, repoURL, token string) ([]tagresolve.RawTag, error) {
	if err := checkRemoteArgs(repoURL, ""); err != nil {
		return nil, err
	}
	if err := requireGit(ctx); err != nil {
		return nil, err
	}
	tags, err := tagresolve.ListTags(ctx, gitRunner, injectToken(repoURL, token))
	if err != nil {
		return nil, oops.With("url", RedactURL(repoURL)).Wrap(err)
	}
	return tags, nil
}

// offline reports whether the run may not use the network.
func offline(ctx context.Context) bool { return SkipFetch || config.OfflineIncludes(ctx) }

// versionRef returns what to fetch for want w, resolving its version constraint
// when it has one. A source with a plain ref keeps the old behaviour: the locked
// commit when pinned, else the ref. A constraint is resolved only here, on the
// paths that may move a pin (`lock` for a source the lock does not cover or an
// `update` that advances it); a covered pin is used as it is.
func versionRef(ctx context.Context, lock *lockfile.File, w lockfile.Want, p *pin, repoURL, token, baseDir string) (string, error) {
	if w.Constraint == "" {
		return p.effectiveRef(w.Ref), nil
	}
	if p != nil {
		recordTag(baseDir, w.Kind, w.Name, tagInfo{p.entry.Tag, p.entry.TagObject})
		return p.entry.Commit, nil
	}
	ref, info, err := resolveConstraint(ctx, lock, w, repoURL, token)
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

// resolveConstraint picks the commit of a constraint source that has no usable pin.
func resolveConstraint(ctx context.Context, lock *lockfile.File, w lockfile.Want, repoURL, token string) (string, tagInfo, error) {
	entry := lock.Find(w.Kind, w.Name)
	covered := entry.Covers(w)
	advance := Advance != nil && Advance(w.Kind, w.Name)
	keep := refreshing(w.Kind, w.Name) && covered && !advance
	if offline(ctx) {
		if keep {
			return entry.Commit, tagInfo{entry.Tag, entry.TagObject}, nil
		}
		return "", tagInfo{}, oops.Hint("Run `ai-rulez lock` with network access").
			Errorf("version constraint %q of %s %q cannot be resolved offline", w.Constraint, w.Kind, w.Name)
	}
	tags, err := ListRemoteTags(ctx, repoURL, token)
	if err != nil {
		return "", tagInfo{}, err
	}
	if keep {
		return keepPin(entry, tags, w)
	}
	sel, err := tagresolve.Select(tags, TagSpec(w))
	if err != nil {
		return "", tagInfo{}, err //nolint:wrapcheck // carries the rule code
	}
	for _, n := range sel.Notes {
		logger.Warn("Ambiguous version tags", "source", w.Name, "note", n)
	}
	if err := refuseDowngrade(entry, sel.Chosen, w); err != nil {
		return "", tagInfo{}, err
	}
	c := sel.Chosen.Tag
	return c.Commit, tagInfo{c.Name, c.TagObject()}, nil
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
func keepPin(entry *lockfile.Entry, tags []tagresolve.RawTag, w lockfile.Want) (string, tagInfo, error) {
	status, now := tagresolve.Check(tags, entry.Tag, entry.Commit)
	switch status {
	case tagresolve.StatusMoved:
		if !AcceptMovedTag {
			return "", tagInfo{}, tagresolve.MovedError(entry.Tag, entry.Commit, now.Commit)
		}
		logger.Warn("Accepted a moved tag", "source", w.Name, "tag", entry.Tag, "was", shortSHA(entry.Commit), "now", shortSHA(now.Commit))
		return now.Commit, tagInfo{now.Name, now.TagObject()}, nil
	case tagresolve.StatusMissing:
		logger.Warn(fmt.Sprintf("%s tag %q of %s %q no longer exists on the remote; keeping the pinned commit", tagresolve.CodeLockedTagMissed, entry.Tag, w.Kind, w.Name))
	case tagresolve.StatusOK:
	}
	return entry.Commit, tagInfo{entry.Tag, entry.TagObject}, nil
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
		urls[lockfile.KindInclude+"\x00"+cfg.Includes[i].Name] = cfg.Includes[i].Source
	}
	for i := range cfg.InstalledSkills {
		urls[lockfile.KindSkill+"\x00"+cfg.InstalledSkills[i].Name] = cfg.InstalledSkills[i].Source
	}
	for _, w := range Lockable(cfg) {
		if w.Constraint != "" {
			out = append(out, VersionSource{Want: w, URL: urls[w.Kind+"\x00"+w.Name]})
		}
	}
	return out
}
