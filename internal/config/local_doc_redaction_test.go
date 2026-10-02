package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalDoc_SaveErrorNeverEchoesTheRejectedValue(t *testing.T) {
	// Arrange
	_, configDir := overlayProject(t, securityShared)
	d, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Set([]string{"header", "style"}, "SECRETSTYLE"))

	// Act
	err = d.Save(t.Context())

	// Assert
	require.Error(t, err)
	rendered := fmt.Sprintf("%+v", err)
	if o, ok := oops.AsOops(err); ok {
		rendered += fmt.Sprint(o.Context())
	}
	assert.NotContains(t, rendered, "SECRETSTYLE")
	assert.Contains(t, rendered, "detailed, compact, minimal")
}

func TestShowAllowed_MarkersAreEntryLevelOnly(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// real markers
		{"includes.inc.remove", true},
		{"mcp_servers.gh.transport", true},
		{"mcp_servers.gh.enabled", true},
		// the same names as user-chosen keys deeper in the entry
		{"mcp_servers.gh.env.remove", false},
		{"mcp_servers.gh.env.name", false},
		{"mcp_servers.gh.env.enabled", false},
		{"mcp_servers.gh.env.transport", false},
		{"mcp_servers.gh.headers.remove", false},
		{"mcp_servers.gh.headers.transport", false},
		{"mcp_servers.gh.remove.extra", false},
		// free-form strings inside table keys are denied
		{"header.text", false},
		{"defaults.model_by_preset.claude", false},
		{"profiles.dev.extra", false},
		// enum and bool leaves are allowed
		{"header.style", true},
		{"header.hashes", true},
		{"header.timestamp", true},
		{"defaults.effort", true},
		{"defaults.effort_by_preset.claude", true},
		{"rules.mode", true},
		{"rules.mode_by_preset.cursor", true},
		{"profiles.dev", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, showAllowed(strings.Split(tt.path, ".")))
		})
	}
}
