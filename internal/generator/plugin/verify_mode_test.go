package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// writeBundle writes outputs the way generate does (bytes, then the recorded
// mode) plus the provenance sidecar, and returns the bundle directory.
func writeBundle(t *testing.T, outputs []config.OutputFile, baseDir string) {
	t.Helper()
	result, err := AddProvenance(outputs, baseDir)
	require.NoError(t, err)
	for _, out := range result {
		mode := out.Mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(out.Path), 0o755))
		require.NoError(t, os.WriteFile(out.Path, out.RawContent, mode))
		require.NoError(t, os.Chmod(out.Path, mode))
	}
}

func TestAddProvenanceRecordsTheModeOfEachOutput(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	outputs := []config.OutputFile{
		{Path: filepath.Join(baseDir, "hooks", "run.sh"), RawContent: []byte("#!/bin/sh\n"), Mode: 0o755},
		{Path: filepath.Join(baseDir, "plugin.json"), RawContent: []byte("{}\n")},
	}

	// Act
	result, err := AddProvenance(outputs, baseDir)

	// Assert
	require.NoError(t, err)
	var sidecar provenanceDocument
	require.NoError(t, json.Unmarshal(result[len(result)-1].RawContent, &sidecar))
	assert.Equal(t, "0755", sidecar.Outputs["hooks/run.sh"].Mode)
	assert.Equal(t, "0644", sidecar.Outputs["plugin.json"].Mode, "an output without a mode is written 0644")
}

func TestVerifyProvenanceDetectsALostExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	tests := []struct {
		name      string
		recorded  os.FileMode
		onDisk    os.FileMode
		wantDrift bool
	}{
		{"exec bit lost", 0o755, 0o644, true},
		{"exec bit gained", 0o644, 0o755, true},
		{"unchanged executable", 0o755, 0o755, false},
		{"other permission bits are not drift", 0o644, 0o600, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			script := filepath.Join(baseDir, "hooks", "run.sh")
			writeBundle(t, []config.OutputFile{{Path: script, RawContent: []byte("#!/bin/sh\n"), Mode: tc.recorded}}, baseDir)
			require.NoError(t, os.Chmod(script, tc.onDisk))

			// Act
			err := VerifyProvenance(baseDir)

			// Assert
			if !tc.wantDrift {
				require.NoError(t, err)
				return
			}
			var drift *ModeDriftError
			require.True(t, errors.As(err, &drift), "got %v", err)
			assert.Equal(t, "hooks/run.sh", drift.Path)
		})
	}
}

func TestVerifyProvenanceAcceptsASidecarWithoutModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	// Arrange: a sidecar written before modes were recorded.
	baseDir := t.TempDir()
	script := filepath.Join(baseDir, "hooks", "run.sh")
	writeBundle(t, []config.OutputFile{{Path: script, RawContent: []byte("#!/bin/sh\n"), Mode: 0o755}}, baseDir)
	sidecarPath := filepath.Join(baseDir, provenanceFileName)
	data, err := os.ReadFile(sidecarPath)
	require.NoError(t, err)
	var document provenanceDocument
	require.NoError(t, json.Unmarshal(data, &document))
	for path, out := range document.Outputs {
		out.Mode = ""
		document.Outputs[path] = out
	}
	data, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(sidecarPath, data, 0o644))
	require.NoError(t, os.Chmod(script, 0o644))

	// Act
	err = VerifyProvenance(baseDir)

	// Assert
	assert.NoError(t, err, "a sidecar that records no mode cannot report a mode difference")
}
