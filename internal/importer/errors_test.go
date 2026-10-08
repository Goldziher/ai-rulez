package importer

import (
	"errors"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlanErrors_AreOopsErrors pins RV-CLI-10 for the importer: the errors a
// plan fails with carry oops context (and a hint where there is a remedy), the
// way the rest of the CLI reports errors, with their text unchanged.
func TestPlanErrors_AreOopsErrors(t *testing.T) {
	tests := []struct {
		name     string
		importer Format
		files    map[string]string
		message  string
		hint     bool
	}{
		{"invalid skills lock", skillsLockImporter{}, map[string]string{"skills-lock.json": "{"}, "skills-lock.json is not valid JSON (AR9F0)", false},
		{"unknown skills lock version", skillsLockImporter{}, map[string]string{"skills-lock.json": `{"version":9}`}, "rerun with --best-effort", true},
		{"invalid rulesync config", rulesyncImporter{}, map[string]string{"rulesync.jsonc": "{", ".rulesync/rules/a.md": "x\n"}, "rulesync.jsonc is not valid JSONC (AR9F0)", false},
		{"no okf bundle", okfImporter{}, map[string]string{"notes.txt": "x\n"}, "no OKF bundle found", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := tt.importer.Plan(mapFS(tt.files), Options{})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.message)
			o, ok := oops.AsOops(err)
			require.True(t, ok, "%T is not an oops error", err)
			if tt.hint {
				assert.NotEmpty(t, o.Hint())
			}
		})
	}
}

func TestReaderSkipReasons_KeepTheirText(t *testing.T) {
	// Arrange
	r := newReader(mapFS(map[string]string{"big.md": string(make([]byte, maxFileBytes+1))}))

	// Act
	_, escapeErr := r.read("../x")
	_, bigErr := r.read("big.md")

	// Assert
	assert.Equal(t, "path escapes the source directory", skipReason(escapeErr))
	assert.Equal(t, "file is larger than the 2 MiB limit", skipReason(bigErr))
	assert.ErrorIs(t, bigErr, errSkipped)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestReportWriteText_ReturnsTheWriteError(t *testing.T) {
	// Arrange
	r := &Report{Importer: "native", Source: "."}

	// Act
	err := r.WriteText(failingWriter{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}
