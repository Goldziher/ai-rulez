package govview

import (
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
	// role when All is set, and Only. Without Write it selects the roles to
	// check: those with pin = true and those pinned in Lock, narrowed to Only.
	Write bool
	All   bool
	// Lock is the lock being checked.
	Lock *lockfile.File
	// Only names roles to pin in addition (Write) or the roles to check.
	Only []string
	// Files adds the per-file digests of a role to its change (`lock --diff`).
	Files bool
	// Enabled makes a check compare role pins at all.
	Enabled bool
}

// names resolves the selection to role names that exist in cfg.
func (s RoleSelection) names(cfg *config.Config) ([]string, error) {
	exists := map[string]bool{}
	pinned := map[string]bool{}
	for i := range cfg.Roles {
		exists[cfg.Roles[i].Name] = true
		if cfg.Roles[i].Pin || (s.Write && s.All) {
			pinned[cfg.Roles[i].Name] = true
		}
	}
	for _, o := range s.Lock.RoleOutputs() {
		if !s.Write && exists[o.Role] {
			pinned[o.Role] = true
		}
	}
	for _, r := range s.Only {
		if !exists[r] {
			return nil, oops.With("role", r).Hint("`ai-rulez roles list` shows the configured roles").Errorf("unknown role %q", r)
		}
		if s.Write {
			pinned[r] = true
		} else if !pinned[r] {
			return nil, oops.With("role", r).Hint("pin it with `ai-rulez lock --role "+r+"` or set pin = true on the role").
				Errorf("role %q is not pinned in the lock", r)
		}
	}
	if !s.Write && len(s.Only) > 0 {
		keep := map[string]bool{}
		for _, r := range s.Only {
			keep[r] = true
		}
		for name := range pinned {
			if !keep[name] {
				delete(pinned, name)
			}
		}
	}
	names := make([]string, 0, len(pinned))
	for name := range pinned {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// SnapshotRoles is Snapshot plus the pins of the selected roles' rendered
// outputs. Every role is rendered in memory: nothing is written.
func SnapshotRoles(cfg *config.Config, profileName string, sourcesOnly bool, toolVersion string, sel RoleSelection) (*contentlock.Snapshot, error) {
	var roleOutputs map[string][]contentlock.Output
	check := sel.Enabled && !sel.Write
	if (sel.Write || sel.Enabled) && !sourcesOnly {
		names, err := sel.names(cfg)
		if err != nil {
			return nil, err
		}
		roleOutputs = map[string][]contentlock.Output{}
		for _, name := range names {
			outputs, err := generator.LockRoleOutputs(cfg, name)
			if err != nil {
				return nil, oops.Wrapf(err, "render the outputs of role %q to pin", name)
			}
			roleOutputs[name] = outputs
		}
	}
	return snapshot(cfg, profileName, sourcesOnly, toolVersion, func(o *contentlock.Options) {
		o.RoleOutputs, o.CheckRoles, o.RoleFiles = roleOutputs, check && !sourcesOnly, sel.Files
		if !sel.Write {
			o.OnlyRoles = sel.Only
		}
	})
}
