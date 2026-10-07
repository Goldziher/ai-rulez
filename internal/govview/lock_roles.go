package govview

import (
	"context"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// RoleSelection chooses the roles whose rendered outputs are pinned or checked.
// The zero value checks nothing about roles.
type RoleSelection struct {
	// Write selects the roles to pin (`lock`): the roles with pin = true, every
	// role when All is set, Only, and the roles Lock already pins. Without Write it selects the roles to
	// check: those with pin = true and those pinned in Lock, narrowed to Only.
	Write bool
	All   bool
	// Lock is the lock being checked, or the current lock `lock` re-pins from.
	Lock *lockfile.File
	// Only names roles to pin in addition (Write) or the roles to check.
	Only []string
	// Files adds the per-file digests of a role to its change (`lock --diff`).
	Files bool
	// Enabled makes a check compare role pins at all.
	Enabled bool
}

// names resolves the selection to role names that exist in cfg, and the subset
// that is pinned only because a command line asked for it (lockfile.OutputPin.Requested).
func (s RoleSelection) names(cfg *config.Config) (names []string, requested map[string]bool, err error) {
	exists := map[string]bool{}
	declared := map[string]bool{}
	pinned := map[string]bool{}
	requested = map[string]bool{}
	for i := range cfg.Roles {
		exists[cfg.Roles[i].Name] = true
		if cfg.Roles[i].Pin {
			declared[cfg.Roles[i].Name] = true
			pinned[cfg.Roles[i].Name] = true
		} else if s.Write && s.All {
			pinned[cfg.Roles[i].Name] = true
			requested[cfg.Roles[i].Name] = true
		}
	}
	// A role pinned with `lock --roles` survives a plain `lock`; one that was pinned
	// by its own pin = true does not outlive that key, so setting it to false
	// unpins the role (the check reports the pin as removed, `lock` drops it). A
	// role that left the config is dropped either way.
	for _, o := range s.Lock.RoleOutputs() {
		if exists[o.Role] && o.Requested {
			pinned[o.Role] = true
			requested[o.Role] = !declared[o.Role]
		}
	}
	if err := s.applyOnly(exists, declared, pinned, requested); err != nil {
		return nil, nil, err
	}
	names = make([]string, 0, len(pinned))
	for name := range pinned {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, requested, nil
}

// applyOnly applies the roles named on the command line: writing pins them
// (requested unless their own pin = true does), checking narrows pinned to them,
// and each must exist (and, when checking, be pinned).
func (s RoleSelection) applyOnly(exists, declared, pinned, requested map[string]bool) error {
	for _, r := range s.Only {
		if !exists[r] {
			return oops.With("role", r).Hint("`ai-rulez roles list` shows the configured roles").Errorf("unknown role %q", r)
		}
		if s.Write {
			pinned[r] = true
			requested[r] = !declared[r]
		} else if !pinned[r] {
			return oops.With("role", r).Hint("pin it with `ai-rulez lock --role "+r+"` or set pin = true on the role").
				Errorf("role %q is not pinned in the lock", r)
		}
	}
	if s.Write || len(s.Only) == 0 {
		return nil
	}
	keep := map[string]bool{}
	for _, r := range s.Only {
		keep[r] = true
	}
	for name := range pinned {
		if !keep[name] {
			delete(pinned, name)
		}
	}
	return nil
}

// SnapshotRoles is Snapshot plus the pins of the selected roles' rendered
// outputs. Every role is rendered in memory: nothing is written.
func SnapshotRoles(cfg *config.Config, profileName string, sourcesOnly bool, toolVersion string, sel RoleSelection) (*contentlock.Snapshot, error) {
	return SnapshotRolesContext(context.Background(), cfg, profileName, sourcesOnly, toolVersion, sel)
}

// SnapshotRolesContext is SnapshotRoles rendering under ctx.
func SnapshotRolesContext(ctx context.Context, cfg *config.Config, profileName string, sourcesOnly bool, toolVersion string, sel RoleSelection) (*contentlock.Snapshot, error) {
	var roleOutputs map[string][]contentlock.Output
	var requested map[string]bool
	check := sel.Enabled && !sel.Write
	if (sel.Write || sel.Enabled) && !sourcesOnly {
		var names []string
		var err error
		names, requested, err = sel.names(cfg)
		if err != nil {
			return nil, err
		}
		roleOutputs = map[string][]contentlock.Output{}
		for _, name := range names {
			outputs, err := generator.LockRoleOutputsContext(ctx, cfg, name)
			if err != nil {
				return nil, oops.Wrapf(err, "render the outputs of role %q to pin", name)
			}
			roleOutputs[name] = outputs
		}
	}
	return snapshot(ctx, cfg, profileName, sourcesOnly, toolVersion, func(o *contentlock.Options) {
		o.RoleOutputs, o.RequestedRoles, o.CheckRoles, o.RoleFiles = roleOutputs, requested, check && !sourcesOnly, sel.Files
		if !sel.Write {
			o.OnlyRoles = sel.Only
		}
	})
}
