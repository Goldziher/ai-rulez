package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/contentlock"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
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
	switch {
	case lock == nil && cfg.LockEnforced():
		diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeLock, Change: contentlock.Added,
			Detail: lockfile.FileName + " does not exist but [lock] enforce = true; run `ai-rulez lock`"})
	case lock != nil && diff.NoPins && cfg.LockEnforced():
		diff.Changes = append(diff.Changes, contentlock.Change{Scope: contentlock.ScopeLock, Change: contentlock.Changed,
			Detail: fmt.Sprintf("%s (version %d) has no content pins and [lock] enforce = true; run `ai-rulez lock`", lockfile.FileName, lock.Version)})
	case lock != nil && diff.NoPins:
		diff.Notes = append(diff.Notes, "the lock has no content pins (version 1); run `ai-rulez lock` to pin authored content")
	}
	if remoteSkipped {
		diff.Notes = append(diff.Notes, "remote includes are not in the local cache, so generated outputs were not compared")
	}
	contentlock.SortChanges(diff.Changes)
	diff.InSync = len(diff.Changes) == 0
	return diff, nil
}

// loadForLockCheck loads a configuration for an offline comparison. Includes are
// read from the local cache so the outputs render as generate would; when they
// are not cached the load falls back to skipping them (remoteSkipped).
func loadForLockCheck(path string) (cfg *config.Config, remoteSkipped bool, err error) {
	prev := includes.SkipFetch
	includes.SkipFetch = true
	defer func() { includes.SkipFetch = prev }()
	cfg, err = loadForLock(path, config.WithoutLocal())
	if err == nil {
		return cfg, false, nil
	}
	cfg, retryErr := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
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
	for _, c := range diff.Changes {
		lines = append(lines, c.Line())
	}
	return lines, nil
}

// lockDriftFor returns the AR981 and AR982 findings for strict validation. It
// reports nothing unless a lock exists and [lock] enforce is set.
func lockDriftFor(cfg *config.Config) ([]lint.LockDrift, error) {
	if !cfg.LockEnforced() {
		return nil, nil
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if lock == nil {
		return nil, nil
	}
	configRel := relToBase(cfg, cfg.ConfigDir)
	lockRel := filepath.ToSlash(filepath.Join(configRel, lockfile.FileName))
	if !lock.HasContentPins() {
		return []lint.LockDrift{{Path: lockRel, Message: fmt.Sprintf("%s (version %d) has no content pins and [lock] enforce = true; run `ai-rulez lock`", lockfile.FileName, lock.Version)}}, nil
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
		return nil, err
	}
	var out []lint.LockDrift
	for _, c := range contentlock.Compare(lock, snap).Changes {
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
	return out, nil
}

func relToBase(cfg *config.Config, abs string) string {
	rel, err := filepath.Rel(cfg.BaseDir, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}
