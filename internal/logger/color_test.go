package logger

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColorEnabled(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	tests := []struct {
		name    string
		noColor string
		term    string
		w       io.Writer
		want    bool
	}{
		{"buffer keeps color", "", "xterm", &bytes.Buffer{}, true},
		{"NO_COLOR disables", "1", "xterm", &bytes.Buffer{}, false},
		{"TERM=dumb disables", "", "dumb", &bytes.Buffer{}, false},
		{"non-terminal file disables", "", "xterm", file, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)

			// Act
			got := colorEnabled(tt.w)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandlerStripsANSIWithoutColor(t *testing.T) {
	// Arrange
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer

	// Act
	New(&buf, -4).Warn("hello", "path", "a/b", "error", assert.AnError)

	// Assert
	assert.NotContains(t, buf.String(), "\x1b[")
	assert.Contains(t, buf.String(), "WARN  hello")
}
