package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const driftMergedExtra = `
[[mcp_servers]]
name = "fs"
command = "npx"
args = ["-y", "fs"]

[[hooks]]
event = "Stop"
[[hooks.hooks]]
command = "echo done"

[permissions]
allow = ["Bash(git status)"]
`

func driftMergedProject(t *testing.T, presets []string) string {
	t.Helper()
	return writeChecksProject(t, presets, map[string]string{"checks/security.md": securityCheck}, driftMergedExtra)
}

func TestCheckDrift_NothingStaleAfterGenerate(t *testing.T) {
	for _, preset := range config.IndividualPresetNames() {
		t.Run(preset, func(t *testing.T) {
			base := driftMergedProject(t, []string{preset})
			generateChecksProject(t, base, "default")

			cfg, err := config.LoadConfig(context.Background(), base)
			require.NoError(t, err)
			drift, err := NewGenerator(cfg).CheckDrift("default")
			require.NoError(t, err)
			assert.Empty(t, drift)
		})
	}
}

func TestCheckDrift_MergedDocumentOwnership(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(body string) string
		stale bool
	}{
		{"unowned key edited", func(b string) string {
			return strings.Replace(b, "{", "{\n  \"userKey\": 1,", 1)
		}, false},
		{"owned key edited", func(b string) string { return strings.Replace(b, "npx", "hacked", 1) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := driftMergedProject(t, []string{"claude"})
			generateChecksProject(t, base, "default")
			path := filepath.Join(base, ".mcp.json")
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte(tt.edit(string(body))), 0o600))

			cfg, err := config.LoadConfig(context.Background(), base)
			require.NoError(t, err)
			drift, err := NewGenerator(cfg).CheckDrift("default")
			require.NoError(t, err)
			if tt.stale {
				assert.Equal(t, []Drift{{Path: ".mcp.json", Kind: DriftStale}}, drift)
			} else {
				assert.Empty(t, drift)
			}
		})
	}
}
