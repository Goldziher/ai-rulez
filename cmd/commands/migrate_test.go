package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `ai-rulez migrate v4` was removed in v5; a user who runs the documented 4.x
// command gets the upgrade path, not a bare "unknown command".
func TestMigrate_PointsAtThe4xRelease(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"migrate v4", []string{"migrate", "v4"}},
		{"migrate v4 with flags", []string{"migrate", "v4", "-C", "x", "--dry-run"}},
		{"bare migrate", []string{"migrate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			RootCmd.SetArgs(tt.args)
			t.Cleanup(func() { RootCmd.SetArgs(nil) })

			// Act
			err := RootCmd.Execute()

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "removed in ai-rulez 5")
			assert.Contains(t, err.Error(), "npx ai-rulez@4 migrate v4")
			assert.True(t, MigrateCmd.Hidden)
		})
	}
}
