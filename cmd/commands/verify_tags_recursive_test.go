package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRecursiveVerifyTags(t *testing.T) {
	tests := []struct {
		name       string
		verify     bool
		moved      bool
		wantCode   int
		wantStderr string
	}{
		{name: "a moved tag fails the root with drift", verify: true, moved: true, wantCode: exitDrift, wantStderr: "AR732"},
		{name: "unchanged tags generate", verify: true, wantCode: 0},
		{name: "without the flag the walk stays offline", verify: false, moved: true, wantCode: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := lockedAgeFixture(t, "")
			lockCheck = false
			if tt.moved {
				f.release("evil", "v1.2.0", true)
			}
			generateVerifyTags = tt.verify
			t.Cleanup(func() { recursive = false })
			recursive = true

			// Act
			var code int
			_, stderr := capture(t, func() { code = runRecursiveGenerate() })

			// Assert
			assert.Equal(t, tt.wantCode, code)
			if tt.wantStderr != "" {
				assert.Contains(t, stderr, tt.wantStderr)
			}
		})
	}
}
