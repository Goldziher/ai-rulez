package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modeProject generates a plugin bundle whose skill core-s carries an
// executable scripts/analyze.py.
func modeProject(t *testing.T) string {
	t.Helper()
	dir := newDomainsProject(t, "")
	script := filepath.Join(dir, ".ai-rulez", "skills", "core-s", "scripts", "analyze.py")
	writeDomainsFile(t, script, "#!/usr/bin/env python3\nprint('hi')\n")
	require.NoError(t, os.Chmod(script, 0o755))
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	return dir
}

// TestVerifyPlugin_ReportsALostExecutableBit covers #403: a bundled script that
// lost its executable bit has the recorded content, so only its mode differs.
func TestVerifyPlugin_ReportsALostExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	const rel = "skills/core-s/scripts/analyze.py"
	tests := []struct {
		name     string
		prepare  func(t *testing.T, dir string)
		wantMode bool
	}{
		{"bundle as generated", func(*testing.T, string) {}, false},
		{"executable bit lost", func(t *testing.T, dir string) {
			require.NoError(t, os.Chmod(filepath.Join(dir, filepath.FromSlash(rel)), 0o644))
		}, true},
		{"older sidecar without modes still compares with the sources", func(t *testing.T, dir string) {
			stripSidecarModes(t, dir)
			require.NoError(t, os.Chmod(filepath.Join(dir, filepath.FromSlash(rel)), 0o644))
		}, true},
		{"older sidecar and an untouched mode", func(t *testing.T, dir string) { stripSidecarModes(t, dir) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := modeProject(t)
			require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
			tt.prepare(t, dir)

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			if !tt.wantMode {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrPluginDrift)
			assert.Contains(t, PluginDriftOf(err), PluginDrift{Path: rel, Kind: PluginDriftMode})
		})
	}
}

// stripSidecarModes rewrites the sidecar the way a release that recorded no
// modes wrote it.
func stripSidecarModes(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, ".ai-rulez-generated.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	legacy := regexp.MustCompile(`,\n\s*"mode": "[0-7]+"`).ReplaceAll(data, nil)
	require.NotEqual(t, string(data), string(legacy), "the sidecar records modes")
	require.NoError(t, os.WriteFile(path, legacy, 0o644))
}
