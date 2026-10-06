package generator

import (
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

// LockRoleOutputs renders, in memory, the outputs `generate --role <role>` would
// write and returns the ones a role pin covers (see LockOutputs). Unlike the
// default pins it includes the ai-rulez-rendered part of a merged document, so a
// skill_mode that only lands in .claude/settings.json (skillOverrides) changes the
// digest. Nothing is written, and cfg is not modified: the role is applied to a
// fresh Generator.
func LockRoleOutputs(cfg *config.Config, role string) ([]contentlock.Output, error) {
	gen := NewGenerator(cfg)
	if err := gen.SetRole(role); err != nil {
		return nil, oops.With("role", role).Wrap(err)
	}
	outputs, err := gen.lockOutputs("", true)
	if err != nil {
		return nil, oops.With("role", role).Wrapf(err, "render the outputs of role %q", role)
	}
	return outputs, nil
}
