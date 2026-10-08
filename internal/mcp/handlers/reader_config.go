package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// WithLoadableConfig makes a reader tool (list_*, read_*) fail the way
// `ai-rulez list` does when the project's configuration does not load: the
// listing is only trustworthy for a configuration that parses, and an empty
// list for a malformed config.toml would read as "nothing is configured".
//
// Like the CLI it loads offline, tolerates an include that cannot be resolved
// (the load warns about it), and ignores the machine-local overlay.
func WithLoadableConfig(next func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)) func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
		loadCtx := config.WithOfflineIncludes(config.WithUnresolvedIncludesTolerated(ctx))
		opts := []config.LoadOption{config.WithoutLocal()}
		if request.GetBool("local", false) {
			opts = append(opts, config.WithoutRemote())
		}
		if _, err := loadProjectConfigWith(loadCtx, request, workingDir(request), opts...); err != nil {
			return ToolError(err)
		}
		return next(ctx, request)
	}
}
