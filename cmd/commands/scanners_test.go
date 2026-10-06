package commands

import (
	"bytes"
	"encoding/json"
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
