package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// lockSnapshot computes the content pins of cfg: the authored items, and the
// generated outputs when [lock] pins them. sourcesOnly skips rendering.
func lockSnapshot(cfg *config.Config, profileName string, sourcesOnly bool) (*contentlock.Snapshot, error) {
	opts := contentlock.Options{
		Scope:          cfg.LockScope(),
		IncludeOutputs: cfg.LockIncludeOutputs(),
		ToolVersion:    Version,
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

// lockDiff compares the lock with the working tree: remote pins (offline, against
// the cache), authored sources and generated outputs. remoteSkipped reports that
// includes could not be loaded from the cache, so the outputs were not compared.
func lockDiff(cfg *config.Config, lock *lockfile.File, profileName string, remoteSkipped bool) (*contentlock.Diff, error) {
	snap, err := lockSnapshot(cfg, profileName, remoteSkipped)
	if err != nil {
		return nil, err
	}
	var diff *contentlock.Diff
	if lock == nil {
		diff = &contentlock.Diff{SchemaVersion: contentlock.DiffSchemaVersion, Changes: []contentlock.Change{}, ToolVersion: Version}
	} else {
		diff = contentlock.Compare(lock, snap)
	}
	problems, _ := includes.CheckLock(cfg, lock)
	for _, p := range problems {
		diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeRemote, Change: contentlock.Changed, Kind: p.Kind, ID: p.Name, Detail: p.Message})
	}
	diff.Changes = append(diff.Changes, dynamicLockChanges(cfg, lock)...)
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

// loadForLockCheck loads a configuration for an offline comparison. Includes are
// read from the local cache so the outputs render as generate would; when they
// are not cached (and only then: any other load error is returned as it is) the
// load falls back to skipping them (remoteSkipped).
func loadForLockCheck(path string) (cfg *config.Config, remoteSkipped bool, err error) {
	prev := includes.SkipFetch
	includes.SkipFetch = true
	defer func() { includes.SkipFetch = prev }()
	return loadWithCacheFallback(func(opts ...config.LoadOption) (*config.Config, error) {
		return loadForLock(path, append([]config.LoadOption{config.WithoutLocal()}, opts...)...)
	})
}

// loadWithCacheFallback loads with remote includes, and retries without them only
// when the first error says an include is not in the cache. Every other error (a
// lock violation, a parse error) is returned unchanged.
func loadWithCacheFallback(load func(opts ...config.LoadOption) (*config.Config, error)) (cfg *config.Config, remoteSkipped bool, err error) {
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

// verifyLockedSources is the content half of `generate --locked`: when the lock
// pins authored content, every source must still match it. The lock is read, never
// written. It returns the differing sources, one line each.
func verifyLockedSources(cfg *config.Config) ([]string, error) {
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if lock == nil || !lock.HasContentPins() {
		if cfg.LockEnforced() {
			return []string{lockfile.FileName + " has no content pins and [lock] enforce = true; run `ai-rulez lock`"}, nil
		}
		if lock != nil {
			logger.Warn("The lock has no content pins, so authored content is not verified; run `ai-rulez lock` to pin it", "lock", lockfile.FileName)
		}
		return nil, nil
	}
	// The lock pins the shared sources: the machine-local overlay is not part of it.
	shared := cfg
	if cfg.LocalOverlay != nil || cfg.LocalContent != nil {
		path := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
		if _, statErr := os.Stat(path); statErr == nil {
			if reloaded, loadErr := config.LoadConfigFromFile(context.Background(), path, config.WithoutLocal()); loadErr == nil {
				shared = reloaded
			}
		}
	}
	snap, err := lockSnapshot(shared, lock.Profile, true)
	if err != nil {
		return nil, err
	}
	diff := contentlock.Compare(lock, snap)
	var lines []string
	for i := range diff.Changes {
		lines = append(lines, diff.Changes[i].Line())
	}
	return lines, nil
}

// lockDriftFor returns the AR981 and AR982 findings for strict validation. It
// reports nothing unless [lock] enforce is set. Under enforce a lock that cannot
// be read or compared (corrupt, newer than this ai-rulez, a source that cannot be
// snapshotted) is itself a finding: enforcement never fails open.
func lockDriftFor(cfg *config.Config) []lint.LockDrift {
	if !cfg.LockEnforced() {
		return nil
	}
	configRel := relToBase(cfg, cfg.ConfigDir)
	lockRel := filepath.ToSlash(filepath.Join(configRel, lockfile.FileName))
	unverifiable := func(err error) []lint.LockDrift {
		return []lint.LockDrift{{Path: lockRel, Message: fmt.Sprintf("cannot verify %s and [lock] enforce = true: %v", lockfile.FileName, err)}}
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return unverifiable(err)
	}
	if lock == nil {
		return nil
	}
	if !lock.HasContentPins() {
		return []lint.LockDrift{{Path: lockRel, Message: fmt.Sprintf("%s (version %d) has no content pins and [lock] enforce = true; run `ai-rulez lock`", lockfile.FileName, lock.Version)}}
	}
	shared := cfg
	if cfg.LocalOverlay != nil || cfg.LocalContent != nil {
		path := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
		if reloaded, loadErr := config.LoadConfigFromFile(context.Background(), path, config.WithoutLocal()); loadErr == nil {
			shared = reloaded
		}
	}
	snap, err := lockSnapshot(shared, lock.Profile, false)
	if err != nil {
		return unverifiable(err)
	}
	var out []lint.LockDrift
	changes := contentlock.Compare(lock, snap).Changes
	for i := range changes {
		c := &changes[i]
		switch c.Scope {
		case contentlock.ScopeOutput:
			out = append(out, lint.LockDrift{Output: true, Path: c.Path, Message: "generated " + c.Line() + " since " + lockfile.FileName + " was written"})
		case contentlock.ScopeSource:
			p := lockRel
			if c.Path != "" {
				p = filepath.ToSlash(filepath.Join(configRel, c.Path))
			}
			out = append(out, lint.LockDrift{Path: p, Message: c.Line() + " since " + lockfile.FileName + " was written"})
		case contentlock.ScopeLock:
			out = append(out, lint.LockDrift{Path: lockRel, Message: c.Line()})
		}
	}
	return out
}

func relToBase(cfg *config.Config, abs string) string {
	rel, err := filepath.Rel(cfg.BaseDir, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}
