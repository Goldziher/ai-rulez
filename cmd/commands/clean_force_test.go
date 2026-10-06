package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanOptions_ForceRemovesEditedFiles(t *testing.T) {
	tests := []struct {
		name  string
		force bool
	}{
		{"without force keeps hand-edited files", false},
		{"with force removes hand-edited files", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			prev := cleanForce
			t.Cleanup(func() { cleanForce = prev })
			cleanForce = tt.force

			// Act
			opts := cleanOptions(false)

			// Assert
			assert.Equal(t, tt.force, opts.RemoveEdited)
			assert.True(t, opts.DryRun)
		})
	}
}
