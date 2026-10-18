package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// resetMCPFlags returns every flag of MCPCmd to its default now, so state left by
// other tests does not leak in, and again after the test that parses arguments.
func resetMCPFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		MCPCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil) //nolint:errcheck // resetting a test flag
			} else {
				_ = f.Value.Set(f.DefValue) //nolint:errcheck // resetting a test flag
			}
			f.Changed = false
		})
	}
	reset()
	t.Cleanup(reset)
}

// closedStdio replaces stdin with an empty file and stdout with a scratch file
// for the test, so a server started by runMCP sees the client disconnect at once.
func closedStdio(t *testing.T) {
	t.Helper()
	in, err := os.Open(os.DevNull)
	require.NoError(t, err)
	out, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	require.NoError(t, err)
	prevIn, prevOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, out
	t.Cleanup(func() {
		os.Stdin, os.Stdout = prevIn, prevOut
		_ = in.Close()  //nolint:errcheck // test cleanup
		_ = out.Close() //nolint:errcheck // test cleanup
	})
}

func TestRunMCP_ServeOnlyFlagsFollowTheServeSkillsKey(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErr    bool
		wantErrMsg string
	}{
		{"a serve-only flag works on a bare mcp when the key is true", []string{"--allow", "deploy*"}, false, ""},
		{"--serve-skills=false turns the key off, so the serve-only flag is refused", []string{"--serve-skills=false", "--allow", "deploy*"}, true,
			"--allow requires --serve-skills (or [mcp] serve_skills = true)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testutil.IsolateHome(t)
			root := t.TempDir()
			writeSettingsFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
				"version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[mcp]\nserve_skills = true\nno_watch = true\n")
			t.Chdir(root)
			resetMCPFlags(t)
			closedStdio(t)
			require.NoError(t, MCPCmd.Flags().Parse(tt.args))

			// Act
			err := runMCP(MCPCmd)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
		})
	}
}
