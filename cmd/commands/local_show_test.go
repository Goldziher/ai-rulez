package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetLocalFlags restores the package-level flag state after a test.
func resetLocalFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		localShowJSON, localShowReveal, localSetString, localSetStdin = false, false, false, false
		localSetCmd.SetIn(nil)
	})
}

// leakyOverlay holds values that must never be printed by default. It is written
// by hand because `local set` would refuse some of these keys.
const leakyOverlay = `name = "mine"

[[mcp_servers]]
name = "gh"
transport = "http"
args = ["--token", "tok123abc"]
url = "https://user:pw@example.com/mcp"
auth = "authsecret"
dsn = "dsnsecret"

[[includes]]
name = "inc"
source = "https://x-access-token:ghp_abc@github.com/o/r.git"

[[foo]]
token = "unnamedsecret"
`

var leakySecrets = []string{"tok123abc", "pw@", "ghp_abc", "authsecret", "dsnsecret", "unnamedsecret"}

func TestLocalShow_DefaultDenyAndReveal(t *testing.T) {
	tests := []struct {
		name    string
		json    bool
		reveal  bool
		visible []string
		hidden  []string
	}{
		{"text default", false, false, []string{"mcp_servers.gh.args", "foo.[0].token", "name: ", "mine", "http", "<redacted>"}, leakySecrets},
		{"json default", true, false, []string{"mcp_servers.gh.args", "foo.[0].token", "<redacted>"}, leakySecrets},
		{"text reveal", false, true, leakySecrets, nil},
		{"json reveal", true, true, leakySecrets, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetLocalFlags(t)
			dir := localProject(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(leakyOverlay), 0o600))
			localShowJSON, localShowReveal = tt.json, tt.reveal

			// Act
			out := captureStdout(t, func() { localShowCmd.Run(localShowCmd, nil) })

			// Assert
			for _, want := range tt.visible {
				assert.Contains(t, out, want)
			}
			for _, secret := range tt.hidden {
				assert.NotContains(t, out, secret)
			}
		})
	}
}

func TestLocalSet_StdinAndStringFlags(t *testing.T) {
	// Arrange
	resetLocalFlags(t)
	dir := localProject(t)

	// Act
	localSetStdin = true
	localSetCmd.SetIn(strings.NewReader("123_456\n"))
	localSetCmd.Run(localSetCmd, []string{"mcp_servers.gh.env.GH_TOKEN"})
	localSetStdin = false
	localSetString = true
	localSetCmd.Run(localSetCmd, []string{"description", "true"})

	// Assert
	local, err := os.ReadFile(filepath.Join(dir, "config.local.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(local), "GH_TOKEN = '123_456'")
	assert.Contains(t, string(local), "description = 'true'")
}

func TestLocalShow_PresetsShowTheMergedResult(t *testing.T) {
	tests := []struct {
		name  string
		local string
		want  string
	}{
		{"adds a preset", `presets = ["devin"]`, `["claude","devin"]`},
		{"drops a preset", `presets = ["!claude", "codex"]`, `["codex"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetLocalFlags(t)
			dir := localProject(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(tt.local+"\n"), 0o600))

			// Act
			out := captureStdout(t, func() { localShowCmd.Run(localShowCmd, nil) })

			// Assert
			assert.Contains(t, out, "-> "+tt.want)
			assert.Contains(t, out, "merged, not a replacement")
		})
	}
}
