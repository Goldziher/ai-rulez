package lint

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func versionEntry(t *testing.T, rng, printed string) string {
	t.Helper()
	exe := filepath.ToSlash(fakeScannerExe(t, ""))
	return fakeScannerCommand(t, "--msg", "scanned") + "version = " + strconv.Quote(rng) + "\n" +
		"version_command = " + tomlArray(fakeScannerArgv(exe, "--print-version", printed)...) + "\n"
}

func TestVersionCommandChecksTheScannerNotTheLauncher(t *testing.T) {
	tests := []struct {
		name      string
		rng       string
		printed   string
		wantRan   bool
		wantUnavl string
	}{
		{"inside the range", ">=2.0.0, <3", "2.5.0", true, ""},
		{"below the range", ">=2.0.0, <3", "1.9.0", false, "version 1.9.0, which does not satisfy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: command[0] answers --version with the Go test flags, not a version.
			p := fakeProject(t, isolationNone, versionEntry(t, tt.rng, tt.printed))

			// Act
			findings := p.run(Options{})

			// Assert
			ran := scannerFindings(findings)
			if tt.wantRan {
				require.Len(t, ran, 1, dump(findings))
				assert.Empty(t, ofCode(findings, CodeScannerUnavailable), dump(findings))
				return
			}
			assert.Empty(t, ran, dump(findings))
			unavailable := ofCode(findings, CodeScannerUnavailable)
			require.Len(t, unavailable, 1, dump(findings))
			assert.Contains(t, unavailable[0].Message, tt.wantUnavl)
		})
	}
}

func TestVersionCommandNeedsAVersionRange(t *testing.T) {
	// Arrange
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--msg", "x")+"version_command = [\"tool\", \"--version\"]\n")

	// Act
	findings := p.run(Options{})

	// Assert
	invalid := ofCode(findings, CodeScannerConfigInvalid)
	require.Len(t, invalid, 1, dump(findings))
	assert.Contains(t, invalid[0].Message, "version_command needs version")
	assert.Empty(t, scannerFindings(findings))
}

func TestCheckScannerVersionAppliesTheRunsRangeCheck(t *testing.T) {
	exe := fakeScannerExe(t, "")
	tests := []struct {
		name string
		info ScannerInfo
		want string
	}{
		{"no range", ScannerInfo{Path: exe}, ""},
		{"satisfied", ScannerInfo{Path: exe, Version: ">=2", VersionCommand: fakeScannerArgv(exe, "--print-version", "2.1.0")}, ""},
		{"below", ScannerInfo{Path: exe, Version: ">=2", VersionCommand: fakeScannerArgv(exe, "--print-version", "1.0.0")}, "does not satisfy"},
		{"no version in the output", ScannerInfo{Path: exe, Version: ">=2", VersionCommand: fakeScannerArgv(exe, "--print-version", "dev")}, "has no version"},
		{"version command missing", ScannerInfo{Path: exe, Version: ">=2", VersionCommand: []string{"ai-rulez-no-such-tool"}}, "version_command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			v := CheckScannerVersion(context.Background(), tt.info)

			// Assert
			if tt.want == "" {
				assert.Empty(t, v.Problem)
				return
			}
			assert.Contains(t, v.Problem, tt.want)
		})
	}
}

func TestInspectScannersCarriesTheVersionCommand(t *testing.T) {
	// Arrange
	p := fakeProject(t, isolationNone, versionEntry(t, ">=2", "2.0.0"))
	cfg := loadNoRemote(t, p.root)

	// Act
	infos := InspectScanners(cfg, p.root)

	// Assert
	require.Len(t, infos, 1)
	assert.Equal(t, ">=2", infos[0].Version)
	assert.Len(t, infos[0].VersionCommand, 4)
	assert.Equal(t, p.root, infos[0].Dir)
}

func TestVersionProbeTimeoutIsConfigurableAndReported(t *testing.T) {
	exe := filepath.ToSlash(fakeScannerExe(t, ""))
	slow := tomlArray(fakeScannerArgv(exe, "--sleep", "2s", "--print-version", "1.0.0")...)
	tests := []struct {
		name    string
		keys    string
		wantRan bool
		wantMsg string
		wantBad string
	}{
		{"short timeout is reported as a timeout", "version_timeout = \"300ms\"\n", false, "version probe timed out after 300ms", ""},
		{"long timeout lets a slow probe pass", "version_timeout = \"30s\"\n", true, "", ""},
		{"invalid value", "version_timeout = \"soon\"\n", false, "", "version_timeout"},
		{"needs version_command or version", "", true, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			body := fakeScannerCommand(t, "--msg", "scanned") + "version = \">=1, <2\"\nversion_command = " + slow + "\n" + tt.keys
			if tt.name == "needs version_command or version" {
				body = fakeScannerCommand(t, "--msg", "scanned") + "version_timeout = \"5s\"\n"
				tt.wantRan, tt.wantBad = false, "version_timeout needs version"
			}
			p := fakeProject(t, isolationNone, body)

			// Act
			findings := p.run(Options{})

			// Assert
			if tt.wantBad != "" {
				invalid := ofCode(findings, CodeScannerConfigInvalid)
				require.Len(t, invalid, 1, dump(findings))
				assert.Contains(t, invalid[0].Message, tt.wantBad)
				return
			}
			if tt.wantRan {
				require.Len(t, scannerFindings(findings), 1, dump(findings))
				return
			}
			unavailable := ofCode(findings, CodeScannerUnavailable)
			require.Len(t, unavailable, 1, dump(findings))
			assert.Contains(t, unavailable[0].Message, tt.wantMsg)
			assert.NotContains(t, unavailable[0].Message, "has no version")
		})
	}
}
