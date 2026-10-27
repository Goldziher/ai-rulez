package commands

import (
	"context"
	"errors"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/pinupdate"
)

// pinEngine runs lock --outdated and update for the lock_outdated, update_plan
// and update tools: the same internal/pinupdate code as the commands, so the
// pre-pin scan and the refusals are the same. It holds no state and reads no flag;
// the consents that override a refusal are not available to a tool call.
type pinEngine struct{}

func (pinEngine) env(call handlers.PinCall) pinupdate.Env {
	return pinupdate.Env{
		Version:  Version,
		Load:     call.Load,
		GitToken: GetGitToken(),
		NewForge: func(cfg *config.Config, offline bool) forge.Client { return newForgeClient(cfg, offline) },
		Cwd:      call.Dir,
	}
}

func (e pinEngine) Outdated(ctx context.Context, call handlers.PinCall) (any, error) {
	report, err := pinupdate.Outdated(ctx, e.env(call), call.Kind, call.Names)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return report, nil
}

func (e pinEngine) Update(ctx context.Context, call handlers.PinCall) (doc any, refused bool, err error) {
	report, err := pinupdate.Update(ctx, e.env(call), pinupdate.Options{DryRun: call.DryRun}, call.Kind, call.Names)
	if errors.Is(err, pinupdate.ErrRefused) {
		return report, true, nil
	}
	if err != nil {
		return nil, false, err //nolint:wrapcheck // already contextual
	}
	return report, false, nil
}
