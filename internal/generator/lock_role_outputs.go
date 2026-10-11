package generator

import (
	"context"

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
	return LockRoleOutputsContext(context.Background(), cfg, role)
}

// LockRoleOutputsContext is LockRoleOutputs rendering under ctx. role may be a
// comma-separated composition.
func LockRoleOutputsContext(ctx context.Context, cfg *config.Config, role string) ([]contentlock.Output, error) {
	names := config.SplitRoleNames(role)
	if len(names) == 0 {
		return nil, oops.With("role", role).Errorf("no role to render")
	}
	gen := NewGenerator(cfg)
	gen.SetContext(ctx)
	if err := gen.SetRoles(names); err != nil {
		return nil, oops.With("role", config.CanonicalRoleNames(names)).Wrap(err)
	}
	outputs, err := gen.lockOutputs("", true)
	if err != nil {
		return nil, oops.With("role", config.CanonicalRoleNames(names)).Wrapf(err, "render the outputs of role %q", role)
	}
	return outputs, nil
}
