package lockfile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanRecordsRoundTripAndSortDeterministically(t *testing.T) {
	dir := t.TempDir()
	f := &File{Version: Version}
	f.SetScan(Scan{Scanner: "zeta", Tree: "sha256:bb", Findings: 1, MaxSeverity: "error", Result: ScanFail})
	f.SetScan(Scan{Scanner: "alpha", Version: "alpha 1.2.3", Tree: "sha256:aa", Result: ScanPass})

	require.NoError(t, Save(dir, f))
	first, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	got, err := Load(dir)
	require.NoError(t, err)

	require.Len(t, got.Scan, 2)
	assert.Equal(t, "alpha", got.Scan[0].Scanner, "sorted by scanner")
	assert.Equal(t, Scan{Scanner: "alpha", Version: "alpha 1.2.3", Tree: "sha256:aa", Result: ScanPass}, got.Scan[0])
	assert.Equal(t, ScanFail, got.FindScan("ZETA").Result, "lookup ignores case")
	assert.Nil(t, got.FindScan("missing"))
	assert.Contains(t, string(first), "[[scan]]")

	// Saving what was loaded reproduces the file byte for byte.
	require.NoError(t, Save(dir, got))
	second, err := os.ReadFile(Path(dir))
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

func TestSetScanReplacesTheRecordOfTheSameScanner(t *testing.T) {
	f := &File{}
	f.SetScan(Scan{Scanner: "a", Tree: "sha256:1", Result: ScanPass})
	f.SetScan(Scan{Scanner: "a", Tree: "sha256:2", Result: ScanFail})

	require.Len(t, f.Scan, 1)
	assert.Equal(t, "sha256:2", f.Scan[0].Tree)
}

func TestScanRecordsDoNotChangeTheFormatVersionOrContentPins(t *testing.T) {
	f := &File{Version: Version, Scan: []Scan{{Scanner: "a", Tree: "sha256:1", Result: ScanPass}}}

	assert.Equal(t, Version, f.FormatVersion())
	assert.False(t, f.HasContentPins(), "a scan record is not a content pin")
}
