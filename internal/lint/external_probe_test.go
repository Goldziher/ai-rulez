package lint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

func noSandbox(t *testing.T) {
	t.Helper()
	prev := scannerSandbox
	scannerSandbox = sandbox.New("windows", func(string) (string, error) { return "", os.ErrNotExist })
	t.Cleanup(func() { scannerSandbox = prev })
}

func TestVersionProbeIsRefusedWhenIsolationIsRequiredAndUnavailable(t *testing.T) {
	// Arrange: --version leaves a marker, so a probe that ran is visible.
	marker := filepath.Join(t.TempDir(), "probed")
	bin := fakeBin(t, "probe-scan", `if [ "$1" = "--version" ]; then echo ran > `+marker+`; echo "probe-scan 1.2.3"; exit 0; fi
echo '{}'
`)
	noSandbox(t)
	p := policyProject(t, "\n[lint.scanner_policy]\nisolation = \"require\"\n\n[[lint.external]]\nname = \"probe\"\ncommand = [\""+bin+"\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\nversion = \">=1.0.0\"\n")

	// Act
	findings := p.run(Options{})

	// Assert
	assert.NoFileExists(t, marker, "isolation = \"require\" must not start the scanner unconfined to ask its version")
	failed := ofCode(findings, CodeScannerRunFailed)
	require.Len(t, failed, 1, dump(findings))
	assert.Contains(t, failed[0].Message, "isolation")
}

func TestProbeScannerVersionHonoursIsolation(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "probed")
	bin := fakeBin(t, "doctor-scan", `echo ran > `+marker+`; echo "doctor-scan 2.0.1"`)
	noSandbox(t)
	tests := []struct {
		name      string
		isolation string
		wantRan   bool
		wantErr   bool
	}{
		{"require without a backend refuses", "require", false, true},
		{"auto without a backend probes unconfined", "auto", true, false},
		{"none probes", "none", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			require.NoError(t, os.RemoveAll(marker))
			// Act
			version, err := ProbeScannerVersion(context.Background(), ScannerInfo{Path: bin, Isolation: tt.isolation})
			// Assert
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.wantRan, fileExists(marker))
			if tt.wantRan {
				assert.Equal(t, "doctor-scan 2.0.1", version)
			}
		})
	}
}

func TestProbeScannerVersionIsConfinedByTheRealSandbox(t *testing.T) {
	if err := scannerSandbox.Check(context.Background()); err != nil {
		t.Skipf("no usable process isolation: %v", err)
	}
	if scannerSandbox.Backend() == sandbox.BackendUnshare {
		t.Skip("unshare confines the network only")
	}
	// Arrange: a --version that also tries to write outside the probe directory.
	outside := filepath.Join(t.TempDir(), "escaped")
	bin := fakeBin(t, "escape-scan", `echo x > `+outside+` 2>/dev/null; echo "escape-scan 9.9.9"`)

	// Act
	version, err := ProbeScannerVersion(context.Background(), ScannerInfo{Path: bin, Isolation: "require"})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "escape-scan 9.9.9", version)
	assert.NoFileExists(t, outside, "the version probe must run inside the sandbox")
}
