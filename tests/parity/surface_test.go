package parity_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/parity"
)

// cobraBuiltins are commands Cobra adds to the tree on its own.
var cobraBuiltins = map[string]bool{"help": true, "completion": true}

// command is one node of the real Cobra tree.
type command struct {
	path     string
	cmd      *cobra.Command
	runnable bool
}

// commandTree walks the real command tree, root excluded, and returns every command by path.
func commandTree() map[string]command {
	out := map[string]command{}
	var walk func(c *cobra.Command, prefix string)
	walk = func(c *cobra.Command, prefix string) {
		for _, sub := range c.Commands() {
			if cobraBuiltins[sub.Name()] && prefix == "" {
				continue
			}
			path := strings.TrimSpace(prefix + " " + sub.Name())
			out[path] = command{path: path, cmd: sub, runnable: sub.Runnable()}
			walk(sub, path)
		}
	}
	walk(commands.RootCmd, "")
	return out
}

// tool is a tool as a client lists it.
type tool struct {
	name   string
	tool   *sdkmcp.Tool
	schema *jsonschema.Schema
}

// listTools lists the tools of an MCP server through a real in-memory client.
func listTools(t *testing.T, srv *sdkmcp.Server) map[string]tool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "parity", Version: "1"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	out := map[string]tool{}
	for item, err := range session.Tools(ctx, nil) {
		require.NoError(t, err)
		raw, err := json.Marshal(item.InputSchema)
		require.NoError(t, err)
		var schema jsonschema.Schema
		require.NoError(t, json.Unmarshal(raw, &schema))
		out[item.Name] = tool{name: item.Name, tool: item, schema: &schema}
	}
	return out
}

// surfaces are the tools of both MCP servers.
type surfaces map[parity.Server]map[string]tool

func realSurfaces(t *testing.T) surfaces {
	t.Helper()
	cat, err := mcp.BuildCatalog("default", "claude", nil, mcp.SkillFilter{})
	require.NoError(t, err)
	return surfaces{
		parity.Authoring: listTools(t, commands.NewAuthoringMCPServer(mcp.WithAnyDirectory()).GetMCPServer()),
		parity.Skills:    listTools(t, mcp.NewSkillServerWith("test", cat, mcp.ServeOptions{}).GetMCPServer()),
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
