package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadOTLPFile_LineCapStopsBeforeBufferingTheRest(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		skipped int
	}{
		{name: "terminated line over the cap", content: strings.Repeat("x", 700_000) + "\n", wantErr: true},
		{name: "unterminated line over the cap", content: strings.Repeat("x", 700_000), wantErr: true},
		{name: "line under the cap is skipped as not OTLP", content: strings.Repeat("x", 400_000) + "\n", skipped: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "big.ndjson")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			// Act
			read, err := readOTLPFile(path, 500_000)

			// Assert
			if tt.wantErr {
				require.ErrorContains(t, err, "exceeds 500000 bytes")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.skipped, read.Skipped)
		})
	}
}
