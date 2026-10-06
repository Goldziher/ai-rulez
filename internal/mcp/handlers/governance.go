package handlers

import (
	"bytes"
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// The governance tools (list_roles, resolve_role, lock_status, catalog) are
// read-only: they never write a file and never use the network. Remote includes
// resolve from the local cache only (config.WithOfflineIncludes); an include that
// is not cached is skipped and the result says so. Each returns the document the
// matching CLI command prints with --format json (compacted), built by
// internal/govview.

const remoteSkippedNote = "remote includes and installed skills resolve from the local cache only: any that is not cached is left out; run `ai-rulez generate` to fetch it"

// DynamicLockChanges reports the source and served pin changes for lock_status.
// It is injected by internal/mcp, which owns the skills server.
type DynamicLockChanges func(ctx context.Context, cfg *config.Config, lock *lockfile.File) []contentlock.Change

// loadOffline loads the project from the local cache only. remoteSkipped is true
// when an include was not cached and had to be left out.
//
// remote reports that the project declares remote includes or installed skills,
// so the result may lack uncached ones (an offline include that is not cached is
// dropped with a warning rather than failing the load).
func loadOffline(ctx context.Context, request *ToolRequest, shared bool) (cfg *config.Config, remoteSkipped, remote bool, err error) {
	ctx = config.WithOfflineIncludes(ctx)
	base := workingDir(request)
	var extra []config.LoadOption
	if shared {
		extra = append(extra, config.WithoutLocal())
	}
	cfg, remoteSkipped, err = govview.LoadWithCacheFallback(func(opts ...config.LoadOption) (*config.Config, error) {
		return loadProjectConfigWith(ctx, request, base, append(append([]config.LoadOption(nil), extra...), opts...)...)
	})
	if err != nil {
		return nil, false, false, err //nolint:wrapcheck // already contextual
	}
	if err := cfg.Validate(); err != nil {
		return nil, false, false, err //nolint:wrapcheck // already contextual
	}
	return cfg, remoteSkipped, len(includes.Lockable(cfg)) > 0, nil
}

func governanceResult(doc any, remoteSkipped bool) (*sdkmcp.CallToolResult, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return ToolError(oops.Wrapf(err, "encode json"))
	}
	res := &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}}}
	if remoteSkipped {
		res.Content = append(res.Content, &sdkmcp.TextContent{Text: remoteSkippedNote})
	}
	return res, nil
}

// ListRolesHandler returns the roles manifest (`roles list --format json`).
func ListRolesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	cfg, skipped, remote, err := loadOffline(ctx, request, false)
	if err != nil {
		return ToolError(err)
	}
	counter, err := tokens.New("")
	if err != nil {
		return ToolError(err)
	}
	data, err := roles.Build(cfg, counter).Marshal()
	if err != nil {
		return ToolError(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return ToolError(oops.Wrapf(err, "compact json"))
	}
	res := &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: compact.String()}}}
	if skipped || remote {
		res.Content = append(res.Content, &sdkmcp.TextContent{Text: remoteSkippedNote})
	}
	return res, nil
}

// ResolveRoleHandler returns what a person holding a role gets
// (`roles resolve <role> --format json`), with the item list capped at limit.
func ResolveRoleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	name := request.GetString("role", "")
	if name == "" {
		return ToolError(oops.New("role is required"))
	}
	cfg, skipped, remote, err := loadOffline(ctx, request, false)
	if err != nil {
		return ToolError(err)
	}
	counter, err := tokens.New("")
	if err != nil {
		return ToolError(err)
	}
	res, err := govview.ResolveRole(cfg, name, counter)
	if err != nil {
		return ToolError(err)
	}
	return governanceResult(govview.ViewResolution(res, int(request.GetNumber("limit", 0))), skipped || remote)
}

// LockStatusHandler returns the comparison of ai-rulez.lock with the working tree
// (`lock --check --format json`) without fetching anything. kind narrows the
// listed changes (see govview.LockKinds); in_sync always covers the whole lock.
func LockStatusHandler(version string, dynamic DynamicLockChanges) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		kind := request.GetString("kind", "")
		cfg, skipped, _, err := loadOffline(ctx, request, true)
		if err != nil {
			return ToolError(err)
		}
		var changes govview.DynamicChanges
		if dynamic != nil {
			changes = func(cfg *config.Config, lock *lockfile.File) []contentlock.Change { return dynamic(ctx, cfg, lock) }
		}
		diff, err := govview.CheckLock(cfg, skipped, "", version, changes)
		if err != nil {
			return ToolError(err)
		}
		if err := govview.FilterChanges(diff, kind); err != nil {
			return ToolError(err)
		}
		return governanceResult(diff, false)
	}
}

// CatalogHandler returns the project catalog (`catalog --format json`), narrowed
// by kind and role and capped at limit items.
func CatalogHandler(version string) func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		cfg, skipped, remote, err := loadOffline(ctx, request, false)
		if err != nil {
			return ToolError(err)
		}
		counter, err := tokens.New("")
		if err != nil {
			return ToolError(err)
		}
		doc, err := govview.BuildCatalog(cfg, counter, version)
		if err != nil {
			return ToolError(err)
		}
		view, err := govview.ViewCatalog(doc, request.GetString("kind", ""), request.GetString("role", ""), int(request.GetNumber("limit", 0)))
		if err != nil {
			return ToolError(err)
		}
		return governanceResult(view, skipped || remote)
	}
}
