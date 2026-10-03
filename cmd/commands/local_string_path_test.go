package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// env/headers open a text zone only at their real position under an MCP server.
func TestParseLocalValue_EnvZoneIsPositional(t *testing.T) {
	tests := []struct {
		path string
		in   string
		want any
	}{
		{"mcp_servers.x.env.PIN", "123_456", "123_456"},
		{"mcp_servers.x.headers.X-Id", "42", "42"},
		// a server that happens to be called "env" or "headers" keeps typed fields
		{"mcp_servers.env.args", `["a", "b"]`, []any{"a", "b"}},
		{"mcp_servers.headers.enabled", "false", false},
		{"mcp_servers.env.profiles", `["dev"]`, []any{"dev"}},
		// not under an MCP server at all
		{"profiles.env", `["a"]`, []any{"a"}},
		// named text fields only count at their real position
		{"profiles.default", `["a"]`, []any{"a"}},
		{"defaults.name", `5`, int64(5)},
		{"mcp_servers.x.command", `5`, "5"},
		{"default", `5`, "5"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, parseLocalValue(strings.Split(tt.path, "."), tt.in, false))
		})
	}
}
