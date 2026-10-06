package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClean_RestoresMergedDocumentWithHeaderComment(t *testing.T) {
	const shared = `version = "4.0"
name = "poolside-clean"
presets = ["poolside"]

[[mcp_servers]]
name = "svc"
command = "svc-bin"
`
	tests := []struct {
		name string
		body string
	}{
		{"one blank line", "# one comment\n\nmodel: big\n"},
		{"two blank lines", "# one comment\n\n\nmodel: big\n"},
		{"crlf", "# one comment\r\n\r\nmodel: big\r\n"},
		{"no blank line", "# one comment\nmodel: big\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, shared)
			path := filepath.Join(p.base, ".poolside", "settings.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o644))
			require.NoError(t, NewGenerator(p.load(t)).Generate(""))
			require.Contains(t, p.read(t, ".poolside/settings.yaml"), "svc")

			// Act
			_, err := NewGenerator(p.load(t)).Clean("", CleanOptions{})

			// Assert
			require.NoError(t, err)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.body, string(got))
		})
	}
}
