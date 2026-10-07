package commands

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

// TestLock_SaysEachRenderWarningOnce is RV-CLI-5: lock loads the configuration
// more than once (the release-age gate, the lock, the served-skill views) and
// renders it more than once, and each pass used to repeat its warnings.
func TestLock_SaysEachRenderWarningOnce(t *testing.T) {
	const hook = "\n[[hooks]]\nevent = \"pre_tool_use\"\ncommand = \"echo hi\"\n"
	const warning = "[[hooks]] not generated for claude"
	tests := []struct {
		name  string
		check bool
	}{
		{name: "lock"},
		{name: "lock --check", check: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			lockProject(t, hook)
			if tt.check {
				capture(t, func() { require.Equal(t, 0, runLockFor("", nil)) })
				lockCheck = true
			}
			var said []string
			t.Cleanup(diag.SetDefaultSink(func(msg string, args ...any) { said = append(said, msg+" "+fmt.Sprint(args...)) }))

			// Act
			var code int
			_, stderr := capture(t, func() { code = runLockFor("", nil) })

			// Assert
			require.Equal(t, 0, code, stderr)
			assert.Equal(t, 1, strings.Count(strings.Join(said, "\n"), warning), "the warning is said once: %q", said)
		})
	}
}
