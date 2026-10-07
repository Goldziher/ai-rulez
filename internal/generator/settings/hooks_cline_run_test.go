package settings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

// TestClineHookScript_FindsTheProjectRootWhateverCDPATHSays runs the rendered
// wrapper with sh: a CDPATH that matches the relative script directory makes a
// plain `cd` print the path it chose, which would end up in $root twice.
func TestClineHookScript_FindsTheProjectRootWhateverCDPATHSays(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Cline hook wrappers are POSIX sh scripts")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}
	tests := []struct {
		name   string
		cdpath string
	}{
		{"no CDPATH", ""},
		{"CDPATH matching the relative script directory", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			cfg := &config.Config{BaseDir: dir, Hooks: []config.HookGroup{
				{Event: "PreToolUse", Hooks: []config.HookAction{{Script: "scripts/probe.sh"}}},
			}}
			scripts := settings.ClineHookScripts(cfg)
			require.Len(t, scripts, 1)
			hook := filepath.Join(".clinerules", "hooks", scripts[0].Name)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(hook)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, hook), []byte(scripts[0].Body), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", "probe.sh"),
				[]byte("#!/bin/sh\necho probe-ran\n"), 0o755))
			cmd := exec.Command(sh, hook)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "CDPATH="+tt.cdpath)
			cmd.Stdin = strings.NewReader("{}")

			// Act
			out, err := cmd.CombinedOutput()

			// Assert
			require.NoError(t, err, string(out))
			assert.Equal(t, "probe-ran\n", string(out))
		})
	}
}
