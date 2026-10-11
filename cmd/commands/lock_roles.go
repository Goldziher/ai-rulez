package commands

import (
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// verifyLockedRoleOutputs is the role half of `generate --locked --role r`: when
// the lock pins the outputs of role (or the role declares pin = true), the
// rendering must still match. A role without a pin behaves as before. role may be
// a comma-separated composition, pinned under its canonical name. The role is
// rendered in memory, never written. It returns the differences, one line each.
func verifyLockedRoleOutputs(cfg *config.Config, lock *lockfile.File, role string) ([]string, error) {
	names := config.SplitRoleNames(role)
	if len(names) == 0 {
		return nil, nil
	}
	key := config.CanonicalRoleNames(names)
	_, locked := lock.RoleOutput(key)
	declared := false
	for i := range cfg.Roles {
		for _, name := range names {
			declared = declared || (cfg.Roles[i].Name == name && cfg.Roles[i].Pin)
		}
	}
	if !locked && !declared {
		return nil, nil
	}
	outputs, err := generator.LockRoleOutputs(cfg, key)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	changes, err := contentlock.CompareRoles(lock, map[string][]contentlock.Output{key: outputs}, []string{key})
	if err != nil {
		return nil, oops.Wrap(err)
	}
	lines := make([]string, 0, len(changes))
	for i := range changes {
		lines = append(lines, changes[i].Line())
	}
	return lines, nil
}

// lockRoleNames is the role --role names: the one whose outputs `lock` also pins,
// and the only one --check and --diff compare. A comma-separated value is the
// canonical composition name.
func lockRoleNames() []string {
	names := config.SplitRoleNames(lockServeRole)
	if len(names) == 0 {
		return nil
	}
	return []string{config.CanonicalRoleNames(names)}
}
