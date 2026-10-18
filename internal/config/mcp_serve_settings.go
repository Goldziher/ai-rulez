package config

import (
	"path"
	"strings"

	"github.com/samber/oops"
)

// validateServeSettings checks the [mcp] allow, deny and tools lists: no blank
// entry, and each glob is well formed. Whether a tool name exists is checked
// when the server starts, since the tool set belongs to the server.
func (m *MCPConfig) validateServeSettings() error {
	for _, list := range []struct {
		key   string
		items []string
		glob  bool
	}{
		{"allow", m.Allow, true}, {"deny", m.Deny, true}, {"tools", m.Tools, false},
	} {
		for _, item := range list.items {
			if strings.TrimSpace(item) == "" {
				return oops.With("field", "mcp."+list.key).
					Hint("Remove the empty entry").
					Errorf("mcp.%s must not contain a blank entry", list.key)
			}
			if !list.glob {
				continue
			}
			if _, err := path.Match(strings.TrimSpace(item), ""); err != nil {
				return oops.With("field", "mcp."+list.key).
					Hint("Use a path.Match glob such as \"review-*\"").
					Errorf("mcp.%s entry %q is not a valid glob", list.key, item)
			}
		}
	}
	return nil
}
