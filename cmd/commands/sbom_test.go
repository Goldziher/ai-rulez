package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitSpy puts a `git` wrapper first on PATH that records its arguments, and
// returns the log path.
func gitSpy(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	script := "#!/bin/sh\necho \"$@\" >> '" + logPath + "'\nexec '" + real + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755)) //nolint:gosec // test stub
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestSBOMDoesNotTouchTheNetworkUnlessOnline(t *testing.T) {
	tests := []struct {
		name         string
		online       bool
		wantLsRemote bool
	}{
		{"offline by default", false, false},
		{"online allows ls-remote", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			logPath := gitSpy(t)
			project := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez"), 0o755))
			cfg := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"x\"\nsource = \"https://127.0.0.1:9/x/y.git\"\nref = \"main\"\n"
			require.NoError(t, os.WriteFile(filepath.Join(project, ".ai-rulez", "config.toml"), []byte(cfg), 0o600))
			t.Chdir(project)
			oldFormat, oldOut, oldOnline := sbomFormat, sbomOutput, sbomOnline
			t.Cleanup(func() { sbomFormat, sbomOutput, sbomOnline = oldFormat, oldOut, oldOnline })
			sbomFormat, sbomOutput, sbomOnline = formatCycloneDX, "", tt.online

			// Act
			var out bytes.Buffer
			err := runSBOM(&out)

			// Assert
			require.NoError(t, err)
			assert.Contains(t, out.String(), "ai-rulez:source:include:x")
			logged, _ := os.ReadFile(logPath) //nolint:errcheck // absent when git never ran
			assert.Equal(t, tt.wantLsRemote, strings.Contains(string(logged), "ls-remote"), string(logged))
		})
	}
}
