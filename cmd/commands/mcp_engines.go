package commands

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
)

// engineOptions wire the engines of the command line into the authoring MCP
// server, so a tool and its command run the same code: the lint of `validate`
// behind validate_config and scan_content, and the policy discovery of
// `validate --show-policy` behind policy_show.
func engineOptions() []mcp.Option {
	return []mcp.Option{
		mcp.WithValidator((&mcpValidator{}).validate),
		mcp.WithPolicyViewer(func(_ context.Context, _ string, cfg *config.Config) (policy.Report, error) {
			return policyReportFor(cfg)
		}),
	}
}

// NewAuthoringMCPServer builds the authoring MCP server `ai-rulez mcp` runs,
// with the command line's engines. opts follow the engines, so they can narrow
// the directories the tools may touch.
func NewAuthoringMCPServer(opts ...mcp.Option) *mcp.Server {
	return mcp.NewServer(Version, append(engineOptions(), opts...)...)
}
