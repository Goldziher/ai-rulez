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
// rendering must still match. A role without a pin behaves as before. The role is
// rendered in memory, never written. It returns the differences, one line each.
func verifyLockedRoleOutputs(cfg *config.Config, lock *lockfile.File, role string) ([]string, error) {
	if role == "" {
		return nil, nil
	}
	_, locked := lock.RoleOutput(role)
	declared := false
	for i := range cfg.Roles {
		declared = declared || (cfg.Roles[i].Name == role && cfg.Roles[i].Pin)
	}
	if !locked && !declared {
		return nil, nil
	}
	outputs, err := generator.LockRoleOutputs(cfg, role)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	changes, err := contentlock.CompareRoles(lock, map[string][]contentlock.Output{role: outputs}, []string{role})
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
// and the only one --check and --diff compare.
func lockRoleNames() []string {
	if lockServeRole == "" {
		return nil
	}
	return []string{lockServeRole}
}
