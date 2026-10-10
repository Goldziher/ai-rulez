package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddIncludeAndInstallSkillRefuseCredentialedURLs checks the MCP tools use
// the same refusal as the CLI: a source with userinfo is not written and the
// handler returns an error result naming the environment variable to use.
func TestAddIncludeAndInstallSkillRefuseCredentialedURLs(t *testing.T) {
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	credentialed := "https://user:" + secret + "@github.com/o/r.git"
	tests := []struct {
		name string
		call func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)
	}{
		{"add_include", AddIncludeHandler},
		{"install_skill", InstallSkillHandler},
	}
	for _, tt := range tests {
		t.Run("refuses/"+tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeMinimalConfig(t, dir)

			res, err := tt.call(context.Background(), newRequestWithArgs(map[string]any{
				"working_directory": dir,
				"name":              "source",
				"source":            credentialed,
			}))

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.True(t, res.IsError, "want an error result")
			assert.Contains(t, textOf(t, res), "embeds a credential")
			assert.Contains(t, textOf(t, res), "AI_RULEZ_GIT_TOKEN")
			assert.NotContains(t, textOf(t, res), secret, "the refusal echoes the credential")
			assertNoSecretUnder(t, dir, secret)
		})
		t.Run("accepts/"+tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeMinimalConfig(t, dir)

			res, err := tt.call(context.Background(), newRequestWithArgs(map[string]any{
				"working_directory": dir,
				"name":              "source",
				"source":            "https://github.com/o/r.git",
			}))

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.IsError, textOf(t, res))
		})
	}
}

func assertNoSecretUnder(t *testing.T, dir, secret string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // a path the test just created
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(data), secret) {
			t.Errorf("credential written to %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}
