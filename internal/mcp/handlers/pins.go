package handlers

import (
	"context"
	"encoding/json"
	"slices"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// The pin tools (lock_outdated, update_plan, update, lock_diff) mirror
// `lock --outdated`, `update --dry-run`, `update` and `lock --diff`. The first
// three read the remotes' tags, so they use the network; lock_diff is offline.
// The engine behind the first three is injected by the host (the command line):
// it lives in internal/pinupdate, which imports internal/mcp and so cannot be
// imported here. The consents that override a defense (--accept-findings,
// --accept-moved-tag, --allow-downgrade) are not tool arguments: a refusal is
// returned to the caller, and a person decides on the command line.

// PinKinds are the entry kinds lock_outdated, update_plan and update can be
// limited to.
func PinKinds() []string {
	return []string{lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource, lockfile.KindMarketplace, lockfile.KindPlugin}
}

// PinCall is what a pin tool hands the engine.
type PinCall struct {
	// Load loads the project the call addresses under a lock policy.
	Load func(ctx context.Context, policy config.LockPolicy, opts ...config.LoadOption) (*config.Config, error)
	// Dir is the working directory of the call.
	Dir string
	// Kind limits the run to one entry kind; Names to these sources.
	Kind  string
	Names []string
	// DryRun plans the update and writes nothing.
	DryRun bool
}

// PinEngine runs the lock-moving operations of the command line.
type PinEngine interface {
	// Outdated returns the document of `lock --outdated --format json`.
	Outdated(ctx context.Context, call PinCall) (doc any, err error)
	// Update returns the document of `update --format json`. refused is true
	// when a source was refused (AR730, AR731, AR732, or scan findings): the
	// document says which, and nothing was written.
	Update(ctx context.Context, call PinCall) (doc any, refused bool, err error)
}

const (
	pinNoEngine = "the lock engine is not available in this server"
	// updateNextStep is what a successful update leaves to do, as `update` prints it.
	updateNextStep = "run `ai-rulez generate`, then `ai-rulez lock` (it refreshes the output pins and the served-skill pins, which stay stale until then)"
)

func pinCall(ctx context.Context, request *ToolRequest) (context.Context, PinCall, error) {
	kind := request.GetString("kind", "")
	if kind != "" && !slices.Contains(PinKinds(), kind) {
		return ctx, PinCall{}, oops.Errorf("unknown kind %q (use include, skill, source, marketplace or plugin)", kind)
	}
	base := workingDir(request)
	// lock reports an include it cannot resolve as a problem of its own.
	ctx = config.WithUnresolvedIncludesTolerated(ctx)
	call := PinCall{
		Dir:   base,
		Kind:  kind,
		Names: request.GetStringSlice("names", nil),
		Load: func(ctx context.Context, p config.LockPolicy, opts ...config.LoadOption) (*config.Config, error) {
			return loadProjectConfigWith(ctx, request, base, append(append([]config.LoadOption(nil), opts...), config.WithLockPolicy(p))...)
		},
	}
	return ctx, call, nil
}

// LockOutdatedHandler returns which sources have newer tags than their pins
// (`lock --outdated --format json`). It reads the remotes' tags and writes nothing.
func LockOutdatedHandler(engine PinEngine) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		if engine == nil {
			return ToolError(oops.New(pinNoEngine))
		}
		ctx, call, err := pinCall(ctx, request)
		if err != nil {
			return ToolError(err)
		}
		doc, err := engine.Outdated(ctx, call)
		if err != nil {
			return ToolError(err)
		}
		return governanceResult(doc, false)
	}
}

// UpdatePlanHandler returns what `update` would change (`update --dry-run
// --format json`). It reads the remotes and refreshes the local cache, and
// writes neither the lock nor the configuration. A refused source is an error
// result carrying the document.
func UpdatePlanHandler(engine PinEngine) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return updateHandler(engine, true)
}

// UpdateHandler moves the pins of sources that use a version constraint
// (`update [name...]`), after the pre-pin security scan. A refusal (AR730,
// AR731, AR732, or scan findings) writes nothing and is an error result
// carrying the document.
func UpdateHandler(engine PinEngine) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return updateHandler(engine, false)
}

func updateHandler(engine PinEngine, dryRun bool) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		if engine == nil {
			return ToolError(oops.New(pinNoEngine))
		}
		ctx, call, err := pinCall(ctx, request)
		if err != nil {
			return ToolError(err)
		}
		call.DryRun = dryRun
		doc, refused, err := engine.Update(ctx, call)
		if err != nil {
			return ToolError(err)
		}
		if refused {
			return refusedResult(doc)
		}
		res, err := governanceResult(doc, false)
		if err == nil && !dryRun && !res.IsError {
			res.Content = append(res.Content, &sdkmcp.TextContent{Text: updateNextStep})
		}
		return res, err
	}
}

// refusedResult is the error result of an update that refused a source: the
// text is the update document (also returned as structuredContent), followed by
// the sentence the command prints.
func refusedResult(doc any) (*sdkmcp.CallToolResult, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return ToolError(oops.Wrapf(err, "encode json"))
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: string(data)},
			&sdkmcp.TextContent{Text: "refused: a source was blocked (AR730, AR731, AR732) or the security scan of a new tree has error findings; nothing was written. Review it, then decide on the command line (--accept-moved-tag, --accept-findings)"},
		},
		IsError: true,
	}, nil
}

// lockViews reads the serve-view selectors lock_status and lock_diff share.
func lockViews(request *ToolRequest) (view LockView, views []LockView, only []string) {
	view = LockView{Role: request.GetString("role", ""), Profile: request.GetString("profile", ""), Targets: request.GetString("targets", ""),
		IncludeStatic: request.GetBool("include_static", false), Sources: request.GetStringSlice("sources", nil)}
	if view.Role != "" || view.Profile != "" || view.Targets != "" || view.IncludeStatic || len(view.Sources) > 0 {
		views = []LockView{view}
	}
	if view.Role != "" {
		only = []string{view.Role}
	}
	return view, views, only
}

// LockDiffHandler returns how the sources and outputs differ from ai-rulez.lock
// (`lock --diff --format json`) without fetching anything. Unlike lock_status it
// is a review aid: it lists per-file digests of a changed role, and a project with
// no lock file is reported as every pin added rather than as an error.
func LockDiffHandler(version string, dynamic DynamicLockChanges) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		view, views, only := lockViews(request)
		cfg, skipped, _, err := loadOffline(ctx, request, true)
		if err != nil {
			return ToolError(err)
		}
		lock, err := lockfile.Load(cfg.ConfigDir)
		if err != nil {
			return ToolError(err)
		}
		profile := view.Profile
		if profile == "" && lock != nil {
			profile = lock.Profile
		}
		var changes govview.DynamicChanges
		if dynamic != nil {
			changes = func(cfg *config.Config, lock *lockfile.File) []contentlock.Change {
				return dynamic(ctx, cfg, lock, views)
			}
		}
		diff, err := govview.LockDiffRolesContext(ctx, cfg, lock, profile, skipped, version, changes, govview.RoleSelection{Only: only, Files: true})
		if err != nil {
			return ToolError(err)
		}
		return governanceResult(diff, false)
	}
}
