package govview

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// DynamicChanges reports the source and served pins that disagree with the
// configuration and the local cache. It is injected because the check needs the
// skills server, which the packages below internal/mcp cannot import.
type DynamicChanges func(cfg *config.Config, lock *lockfile.File) []contentlock.Change

// Snapshot computes the content pins of cfg: the authored items, and the
// generated outputs when [lock] pins them. sourcesOnly skips rendering.
func Snapshot(cfg *config.Config, profileName string, sourcesOnly bool, toolVersion string) (*contentlock.Snapshot, error) {
	opts := contentlock.Options{
		Scope:          cfg.LockScope(),
		IncludeOutputs: cfg.LockIncludeOutputs(),
		ToolVersion:    toolVersion,
		Profile:        profileName,
		SourcesOnly:    sourcesOnly,
	}
	if opts.IncludeOutputs && !sourcesOnly {
		outputs, err := generator.NewGenerator(cfg).LockOutputs(profileName)
		if err != nil {
			return nil, oops.Wrapf(err, "render the outputs to pin")
		}
		opts.Outputs = outputs
	}
	snap, err := contentlock.Compute(cfg, opts)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return snap, nil
}

// LockDiff compares the lock with the working tree: remote pins (offline, against
// the cache), authored sources and generated outputs. remoteSkipped reports that
// includes could not be loaded from the cache, so the outputs were not compared.
// dynamic adds the source and served pin changes; nil leaves them out.
func LockDiff(cfg *config.Config, lock *lockfile.File, profileName string, remoteSkipped bool, toolVersion string, dynamic DynamicChanges) (*contentlock.Diff, error) {
	snap, err := Snapshot(cfg, profileName, remoteSkipped, toolVersion)
	if err != nil {
		return nil, err
	}
	var diff *contentlock.Diff
	if lock == nil {
		diff = &contentlock.Diff{SchemaVersion: contentlock.DiffSchemaVersion, Changes: []contentlock.Change{}, ToolVersion: toolVersion}
	} else {
		diff = contentlock.Compare(lock, snap)
	}
	problems, _ := includes.CheckLock(cfg, lock)
	for _, p := range problems {
		diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeRemote, Change: contentlock.Changed, Kind: p.Kind, ID: p.Name, Detail: p.Message})
	}
	if dynamic != nil {
		diff.Changes = append(diff.Changes, dynamic(cfg, lock)...)
	}
	switch {
	case lock == nil && cfg.LockEnforced():
		diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeLock, Change: contentlock.Added,
			Detail: lockfile.FileName + " does not exist but [lock] enforce = true; run `ai-rulez lock`"})
	case lock != nil && diff.NoPins:
		// A lock without content pins cannot tell whether a source changed, so a
		// check never passes on it: that would let a stripped lock switch the
		// content checks off.
		diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeLock, Change: contentlock.Changed,
			Detail: fmt.Sprintf("%s (version %d) has no content pins, so authored content is not verified; run `ai-rulez lock` to pin it", lockfile.FileName, lock.Version)})
	}
	if remoteSkipped {
		const msg = "remote includes are not in the local cache, so generated outputs were not compared"
		if cfg.LockEnforced() {
			diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeLock, Change: contentlock.Changed,
				Detail: msg + " and [lock] enforce = true; run `ai-rulez generate` (or `ai-rulez lock`) to fetch them"})
		} else {
			diff.Notes = append(diff.Notes, msg)
		}
	}
	contentlock.SortChanges(diff.Changes)
	diff.InSync = len(diff.Changes) == 0
	return diff, nil
}

// CheckLock is the comparison behind `lock --check`: it reads the lock next to
// cfg and diffs it with the working tree. profileOverride, when set, replaces the
// profile the lock recorded. It never writes and never uses the network.
func CheckLock(cfg *config.Config, remoteSkipped bool, profileOverride, toolVersion string, dynamic DynamicChanges) (*contentlock.Diff, error) {
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	profile := profileOverride
	if profile == "" && lock != nil {
		profile = lock.Profile
	}
	return LockDiff(cfg, lock, profile, remoteSkipped, toolVersion, dynamic)
}

// LoadWithCacheFallback loads with remote includes, and retries without them only
// when the first error says an include is not in the cache. Every other error (a
// lock violation, a parse error) is returned unchanged.
func LoadWithCacheFallback(load func(opts ...config.LoadOption) (*config.Config, error)) (cfg *config.Config, remoteSkipped bool, err error) {
	cfg, err = load()
	if err == nil {
		return cfg, false, nil
	}
	if !errors.Is(err, includes.ErrNotCached) {
		return nil, false, err
	}
	cfg, retryErr := load(config.WithoutRemote())
	if retryErr != nil {
		return nil, false, err
	}
	return cfg, len(includes.Lockable(cfg)) > 0, nil
}

// Lock status kinds accepted by FilterChanges: the entry kinds of `lock --kind`
// plus "content", the authored items and generated outputs.
const KindContent = "content"

// LockKinds lists the values FilterChanges accepts.
var LockKinds = []string{lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource, lockfile.KindServed, KindContent}

// FilterChanges keeps the changes of one kind (see LockKinds); empty keeps all.
// The diff's in_sync stays that of the whole comparison.
func FilterChanges(diff *contentlock.Diff, kind string) error {
	if kind == "" {
		return nil
	}
	known := false
	for _, k := range LockKinds {
		known = known || k == kind
	}
	if !known {
		return oops.Errorf("unknown kind %q (use %s)", kind, strings.Join(LockKinds, ", "))
	}
	kept := []contentlock.Change{}
	for i := range diff.Changes {
		c := &diff.Changes[i]
		switch {
		case kind == KindContent && (c.Scope == contentlock.ScopeSource || c.Scope == contentlock.ScopeOutput):
			kept = append(kept, *c)
		case kind != KindContent && c.Scope != contentlock.ScopeSource && c.Scope != contentlock.ScopeOutput && (c.Kind == kind || (kind == lockfile.KindServed && c.Scope == contentlock.ScopeServed)):
			kept = append(kept, *c)
		}
	}
	diff.Changes = kept
	return nil
}
