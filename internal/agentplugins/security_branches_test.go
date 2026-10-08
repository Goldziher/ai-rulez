package agentplugins

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The symlink and containment gate: every state a link or a bundled path can
// be in is reported, and none is read past the plugin root.
func TestValidateReportsEveryLinkAndContainmentState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(m fstest.MapFS)
		want   []string
	}{
		{name: "dangling resource link", mutate: func(m fstest.MapFS) { m["skills/summarize/scripts/gone.sh"] = symlink("../../../nowhere.sh") },
			want: []string{"unreadable@skills/summarize/scripts/gone.sh"}},
		{name: "resource link to a directory", mutate: func(m fstest.MapFS) {
			m["shared/dir/x"] = &fstest.MapFile{Data: []byte("x")}
			m["skills/summarize/scripts/dir"] = symlink("../../../shared/dir")
		}, want: []string{"unreadable@skills/summarize/scripts/dir"}},
		{name: "resource link loop", mutate: func(m fstest.MapFS) { m["skills/summarize/scripts/loop.sh"] = symlink("loop.sh") },
			want: []string{"unreadable@skills/summarize/scripts/loop.sh"}},
		{name: "bundled command is a directory", mutate: func(m fstest.MapFS) {
			delete(m, "bin/validator")
			m["bin/validator/x"] = &fstest.MapFile{Data: []byte("x")}
		}, want: []string{"mcp-server-invalid@mcp.json#/mcpServers/validator"}},
		{name: "bundled command link loop", mutate: func(m fstest.MapFS) { m["bin/validator"] = symlink("validator") },
			want: []string{"unreadable@bin/validator", "mcp-server-invalid@mcp.json#/mcpServers/validator"}},
		{name: "cwd link escapes the root", mutate: func(m fstest.MapFS) {
			m["work"] = symlink("../../outside")
			m["mcp.json"].Data = []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"v":{"type":"stdio","command":"./bin/validator","cwd":"./work"}}}`)
		}, want: []string{"mcp-server-invalid@mcp.json#/mcpServers/v", "path-escape@work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := builtFS(t, Spec100)
			tt.mutate(m)

			// Act
			res := Validate(m)

			// Assert
			assert.False(t, res.Rejected)
			assert.Equal(t, tt.want, codes(res.Findings), "%+v", res.Findings)
		})
	}
}

func TestWithin(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{"", true}, {".", true}, {"a/b", true}, {"a/../b", true},
		{"..", false}, {"../a", false}, {"a/../../b", false},
		{"/etc", false}, {`a\b`, false}, {"a\x00b", false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			assert.Equal(t, tt.want, within(tt.rel))
		})
	}
}

func TestValidLabel(t *testing.T) {
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	tests := []struct {
		label string
		want  bool
	}{
		{"example", true}, {"a-b", true}, {"a1", true}, {"x", true},
		{"", false}, {"-a", false}, {"a-", false}, {"A", false}, {"a_b", false}, {"a.b", false},
		{string(long[:63]), true}, {string(long), false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			assert.Equal(t, tt.want, validLabel(tt.label))
		})
	}
}

func TestHasErrors(t *testing.T) {
	require.False(t, HasErrors(nil))
	assert.False(t, HasErrors([]Finding{{Severity: SeverityWarning}}))
	assert.True(t, HasErrors([]Finding{{Severity: SeverityWarning}, {Severity: SeverityError}}))
}
