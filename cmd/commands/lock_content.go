package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// lockSnapshot computes the content pins of cfg (see govview.Snapshot).
func lockSnapshot(cfg *config.Config, profileName string, sourcesOnly bool) (*contentlock.Snapshot, error) {
	return govview.Snapshot(cfg, profileName, sourcesOnly, Version)
}

// lockRoleSnapshot is lockSnapshot plus the pins of the selected roles' outputs.
func lockRoleSnapshot(cfg *config.Config, profileName string, sourcesOnly bool, sel govview.RoleSelection) (*contentlock.Snapshot, error) {
	return govview.SnapshotRoles(cfg, profileName, sourcesOnly, Version, sel)
}

// lockDiff compares the lock with the working tree (see govview.LockDiffRoles).
// --diff adds the per-file digests of a changed role.
func lockDiff(cfg *config.Config, lock *lockfile.File, profileName string, remoteSkipped bool) (*contentlock.Diff, error) {
	return govview.LockDiffRoles(cfg, lock, profileName, remoteSkipped, Version, dynamicLockChanges, govview.RoleSelection{Only: lockRoleNames(), Files: true})
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

var loadWithCacheFallback = govview.LoadWithCacheFallback

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
	shared, err := sharedConfig(cfg)
	if err != nil {
		return nil, err
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
	roleLines, err := verifyLockedRoleOutputs(shared, lock, generateRole)
	if err != nil {
		return nil, err
	}
	return append(lines, roleLines...), nil
}

// sharedConfig returns cfg without the machine-local overlay: the lock pins the
// shared sources only. A reload that fails is an error, never a silent fallback
// to cfg, which would compare the local overlay against the shared pins.
func sharedConfig(cfg *config.Config) (*config.Config, error) {
	if cfg.LocalOverlay == nil && cfg.LocalContent == nil {
		return cfg, nil
	}
	path := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
	if _, err := os.Stat(path); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "reload the shared configuration without the local overlay")
	}
	reloaded, err := config.LoadConfigFromFile(context.Background(), path, config.WithoutLocal())
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "reload the shared configuration without the local overlay")
	}
	return reloaded, nil
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
	shared, err := sharedConfig(cfg)
	if err != nil {
		return unverifiable(err)
	}
	snap, err := lockRoleSnapshot(shared, lock.Profile, false, govview.RoleSelection{Enabled: true, Lock: lock})
	if err != nil {
		return unverifiable(err)
	}
	var out []lint.LockDrift
	changes := contentlock.Compare(lock, snap).Changes
	for i := range changes {
		c := &changes[i]
		switch c.Scope {
		case contentlock.ScopeOutput:
			path := c.Path
			if path == "" {
				path = lockRel // a role's outputs are pinned as one digest in the lock
			}
			out = append(out, lint.LockDrift{Output: true, Path: path, Message: "generated " + c.Line() + " since " + lockfile.FileName + " was written"})
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
