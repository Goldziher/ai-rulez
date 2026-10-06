package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

func compactJSON(t *testing.T, raw string) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, json.Compact(&b, []byte(raw)), raw)
	return b.String()
}

func toolText(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	require.False(t, res.IsError, text.Text)
	return text.Text
}

// The MCP governance tools print the document the CLI prints with --format json
// (compacted), for the same project.
func TestMCPGovernanceToolsMatchCLIJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	resetLockViewFlags(t)
	root := rolesCmdProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))
	args := map[string]any{"working_directory": root}
	call := func(h func(context.Context, *handlers.ToolRequest) (*sdkmcp.CallToolResult, error), extra map[string]any) string {
		in := map[string]any{}
		for k, v := range args {
			in[k] = v
		}
		for k, v := range extra {
			in[k] = v
		}
		res, err := h(t.Context(), handlers.NewToolRequest(nil, in))
		require.NoError(t, err)
		return toolText(t, res)
	}
	lockStatus := handlers.LockStatusHandler(Version, func(ctx context.Context, cfg *config.Config, lock *lockfile.File) []contentlock.Change {
		return mcp.DynamicLockChanges(ctx, cfg, lock, Version)
	})
	// Change a source after pinning so lock_status has a drift to report.
	writeFile(t, root+"/.ai-rulez/rules/style.md", "# Style\nchanged\n")

	tests := []struct {
		name string
		cli  func() string
		tool func() string
	}{
		{"list_roles", func() string {
			rolesFormat = formatJSON
			var out bytes.Buffer
			require.NoError(t, runRolesList(&out))
			return out.String()
		}, func() string { return call(handlers.ListRolesHandler, nil) }},
		{"resolve_role", func() string {
			rolesFormat = formatJSON
			var out bytes.Buffer
			require.NoError(t, runRolesResolve(&out, "dev"))
			return out.String()
		}, func() string { return call(handlers.ResolveRoleHandler, map[string]any{"role": "dev"}) }},
		{"catalog", func() string {
			catalogFormat = formatJSON
			var out bytes.Buffer
			require.NoError(t, runCatalog(&out))
			return out.String()
		}, func() string { return call(handlers.CatalogHandler(Version), nil) }},
		{"lock_status", func() string {
			lockCheck, lockFormat = true, formatJSON
			return captureStdout(t, func() { checkLockAt("") })
		}, func() string { return call(lockStatus, nil) }},
	}
	t.Cleanup(func() { rolesFormat, catalogFormat = "", "" })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			want := compactJSON(t, tt.cli())
			got := tt.tool()

			// Assert
			assert.Equal(t, want, got)
			if tt.name == "lock_status" {
				assert.Contains(t, got, `"in_sync":false`, "the edited rule is drift")
			}
		})
	}
}
