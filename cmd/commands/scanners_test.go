package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteScannersJSON(t *testing.T) {
	infos := []lint.ScannerInfo{
		{Name: "gitleaks", Command: "gitleaks", Path: "/usr/bin/gitleaks", Egress: "false", Format: "sarif", Timeout: 30 * time.Second},
		{Name: "missing", Command: "nope", Egress: "true"},
	}
	tests := []struct {
		name   string
		doctor bool
	}{
		{"list", false},
		{"doctor adds versions", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer

			// Act
			err := writeScannersJSON(&buf, infos, tt.doctor, map[string]string{"gitleaks": "8.0.0"})

			// Assert
			require.NoError(t, err)
			var got struct {
				Scanners []scannerJSON `json:"scanners"`
			}
			require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
			require.Len(t, got.Scanners, 2)
			assert.True(t, got.Scanners[0].Found)
			assert.True(t, got.Scanners[0].Healthy)
			assert.Equal(t, 30.0, got.Scanners[0].TimeoutSec)
			assert.False(t, got.Scanners[1].Found)
			assert.Equal(t, "not found on PATH", got.Scanners[1].Status)
			assert.NotNil(t, got.Scanners[1].Inputs)
			if tt.doctor {
				assert.Equal(t, "8.0.0", got.Scanners[0].Version)
			} else {
				assert.Empty(t, got.Scanners[0].Version)
			}
		})
	}
}

func TestCheckScannersFormatRejectsUnknown(t *testing.T) {
	old := scannersFormat
	t.Cleanup(func() { scannersFormat = old })
	for format, wantErr := range map[string]bool{"text": false, "json": false, "xml": true} {
		scannersFormat = format
		assert.Equal(t, wantErr, checkScannersFormat(nil, nil) != nil, format)
	}
}

func TestWriteDoctor_StartsTheScannerOnlyWithExternal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script scanner")
	}
	tests := []struct {
		name        string
		probe       bool
		wantStarted bool
		wantText    string
	}{
		{"default does not probe", false, false, "not probed (pass --external)"},
		{"external probes", true, true, "9.9.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a scanner that records that it ran.
			dir := t.TempDir()
			marker := filepath.Join(dir, "ran")
			script := filepath.Join(dir, "scanner")
			require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\necho 9.9.9\n"), 0o755)) //nolint:gosec // test script
			info := lint.ScannerInfo{Name: "s", Command: "scanner", Path: script, Egress: "false", Format: "sarif", Timeout: time.Second, Isolation: "none"}
			var buf bytes.Buffer

			// Act
			writeDoctor(t.Context(), &buf, info, tt.probe)

			// Assert
			_, err := os.Stat(marker)
			assert.Equal(t, tt.wantStarted, err == nil)
			assert.Contains(t, buf.String(), tt.wantText)
		})
	}
}

func TestRunScannersDoctorJSON_DoesNotProbeWithoutExternal(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeScannersJSON(&buf, []lint.ScannerInfo{{Name: "s", Command: "s", Path: "/bin/s"}}, true, nil))

	assert.NotContains(t, buf.String(), `"version"`)
}

func TestWriteScannerListShowsThePresetColumn(t *testing.T) {
	infos := []lint.ScannerInfo{
		{Name: "agnix", Egress: "false", Inputs: []string{"rules"}, Presets: []string{"baseline", "strict"}, Profile: "agnix", FromPreset: true},
		{Name: "mine", Egress: "true", Path: "/bin/mine"},
	}
	var buf bytes.Buffer

	writeScannerList(&buf, infos)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3)
	assert.Regexp(t, `NAME\s+EGRESS\s+INPUTS\s+PRESET\s+STATUS`, lines[0])
	assert.Regexp(t, `agnix\s+false\s+rules\s+baseline,strict\s+not found on PATH`, lines[1])
	assert.Regexp(t, `mine\s+YES\s+-\s+-\s+found; needs --allow-egress=mine`, lines[2])
}

func TestWriteScannersJSONCarriesPolicyFields(t *testing.T) {
	infos := []lint.ScannerInfo{{
		Name: "snyk", Command: "snyk-agent-scan", Egress: "true", Profile: "snyk-agent-scan", Required: true, Version: ">=1",
		DataSent: []string{"skill content"}, Isolation: "auto", Backend: "sandbox-exec",
	}, {Name: "plain", Command: "plain", Egress: "false"}}
	var buf bytes.Buffer

	require.NoError(t, writeScannersJSON(&buf, infos, false, nil))

	var got struct {
		Scanners []scannerJSON `json:"scanners"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "snyk-agent-scan", got.Scanners[0].Profile)
	assert.True(t, got.Scanners[0].Required)
	assert.Equal(t, ">=1", got.Scanners[0].VersionRange)
	assert.Equal(t, []string{"skill content"}, got.Scanners[0].DataSent)
	assert.Equal(t, "auto", got.Scanners[0].Isolation)
	assert.Equal(t, "sandbox-exec", got.Scanners[0].IsolationBackend)
	assert.NotNil(t, got.Scanners[1].Presets, "an empty list, not null")
}

func TestIsolationText(t *testing.T) {
	tests := []struct {
		name string
		in   lint.ScannerInfo
		want string
	}{
		{"none", lint.ScannerInfo{Isolation: "none", Inputs: []string{"rules"}}, "none (isolation"},
		{"in the project root", lint.ScannerInfo{Isolation: "auto"}, "runs in the project root"},
		{"required in the project root", lint.ScannerInfo{Isolation: "require"}, "refused, a scanner without inputs"},
		{"no backend, required", lint.ScannerInfo{Isolation: "require", Inputs: []string{"rules"}}, "refused, this system has no isolation backend"},
		{"no backend, auto", lint.ScannerInfo{Isolation: "auto", Inputs: []string{"rules"}}, "AR9E7"},
		{"confined", lint.ScannerInfo{Isolation: "auto", Inputs: []string{"rules"}, Backend: "bwrap"}, "bwrap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, isolationText(tt.in), tt.want)
		})
	}
}

func TestScannersListSeesThePresetAndProfiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[lint.scanner_policy]\npreset = \"strict\"\nrequired = [\"agnix\"]\n\n[[lint.external]]\nname = \"snyk\"\nprofile = \"snyk-agent-scan\"\n")
	chdir(t, root)
	infos, err := loadScanners(t.Context(), nil)
	require.NoError(t, err)

	byName := map[string]lint.ScannerInfo{}
	for _, s := range infos {
		byName[s.Name] = s
	}
	require.Contains(t, byName, "agnix")
	assert.True(t, byName["agnix"].FromPreset)
	assert.True(t, byName["agnix"].Required)
	assert.Equal(t, []string{"baseline", "strict"}, byName["agnix"].Presets)
	assert.Contains(t, byName, "cisco-skill-scanner")
	assert.NotContains(t, byName, "claude-plugin-validate", "it joins a preset only with [plugin] or [marketplace]")
	assert.Equal(t, "true", byName["snyk"].Egress, "the profile declares egress")
	assert.Equal(t, []string{"skill content (secrets redacted by the vendor)", "MCP server configuration and tool descriptions", "agent application details"}, byName["snyk"].DataSent)
	assert.Equal(t, "snyk-agent-scan", byName["snyk"].Profile)
}
