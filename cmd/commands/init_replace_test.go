package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceConfigDir(t *testing.T) {
	tests := []struct {
		name      string
		write     func(configDir string) error
		wantErr   bool
		wantStale bool
		wantNew   bool
	}{
		{"a successful import replaces the directory", func(d string) error {
			return os.MkdirAll(filepath.Join(d, "rules"), 0o755)
		}, false, false, true},
		{"a failed import restores the previous directory", func(d string) error {
			_ = os.MkdirAll(filepath.Join(d, "partial"), 0o755)
			return assert.AnError
		}, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			configDir := filepath.Join(dir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(configDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(configDir, "stale.md"), []byte("old\n"), 0o644))

			// Act
			err := replaceConfigDir(configDir, func() error { return tt.write(configDir) })

			// Assert
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.wantStale, pathExists(filepath.Join(configDir, "stale.md")))
			assert.Equal(t, tt.wantNew, pathExists(filepath.Join(configDir, "rules")))
			assert.False(t, pathExists(filepath.Join(configDir, "partial")))
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			if tt.wantErr {
				assert.Len(t, entries, 1, "a failed import leaves no backup behind")
				return
			}
			require.Len(t, entries, 2, "a successful import keeps the previous directory as a backup")
			assert.FileExists(t, filepath.Join(dir, entries[0].Name(), "stale.md"))
		})
	}
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
