package mcp

import (
	"context"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	proj "github.com/Goldziher/ai-rulez/v5/internal/project"
	"github.com/samber/oops"
)

// ProjectSettings is what the project says about how `ai-rulez mcp` runs: its
// [mcp] table, and whether the organization policy requires a read-only server.
type ProjectSettings struct {
	// MCP is the [mcp] table with the layers and the machine-local overlay
	// applied; empty when there is no configuration.
	MCP config.MCPConfig
	// PolicyReadOnly is true when the organization policy requires a read-only
	// server. No flag or key turns it off.
	PolicyReadOnly bool
}

// LoadProjectSettings reads the [mcp] settings of the project that workDir
// belongs to. A directory with no configuration has none and is not an error.
// The load is offline: it reads what is on disk and in the caches, so starting
// a server never waits on the network for settings. Warnings of the load are
// dropped; the server's own load reports them.
func LoadProjectSettings(ctx context.Context, workDir string) (ProjectSettings, error) {
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return ProjectSettings{}, oops.Wrapf(err, "resolve %s", workDir)
	}
	root, err := config.DiscoverProjectRoot(ctx, abs)
	if err != nil {
		// No configuration up the tree: the defaults apply.
		return ProjectSettings{}, nil //nolint:nilerr // a directory without a project has no settings
	}
	cfg, err := proj.Load(ctx, root, config.WithLockPolicy(config.LockPolicy{Offline: true}), config.WithCollector(diag.New(nil)))
	if err != nil {
		return ProjectSettings{}, oops.Wrapf(err, "load the [mcp] settings of %s", root)
	}
	var out ProjectSettings
	if cfg.MCP != nil {
		out.MCP = *cfg.MCP
	}
	if cfg.PolicyOutcome != nil {
		out.PolicyReadOnly = cfg.PolicyOutcome.MCPReadOnly
	}
	return out, nil
}
