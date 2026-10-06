package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockCheckFormatJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	resetLockViewFlags(t)
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockCheck, lockFormat = true, formatJSON

	tests := []struct {
		name     string
		mutate   func()
		wantCode int
		inSync   bool
	}{
		{"in sync exits 0", func() {}, 0, true},
		{"drift exits 2 and still prints JSON", func() {
			writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nchanged\n")
		}, exitDrift, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			tt.mutate()

			// Act
			var code int
			stdout := captureStdout(t, func() { code = checkLockAt("") })

			// Assert
			assert.Equal(t, tt.wantCode, code)
			var doc struct {
				SchemaVersion int  `json:"schema_version"`
				InSync        bool `json:"in_sync"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
			assert.Equal(t, 1, doc.SchemaVersion)
			assert.Equal(t, tt.inSync, doc.InSync)
		})
	}
}
