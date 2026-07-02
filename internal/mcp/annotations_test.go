package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnnotationHelpers pins the MCP tool-annotation hint semantics: read-only
// tools must not be destructive, deletes must be destructive, creates must be
// additive (non-destructive), and updates must be additive + idempotent. These
// hints drive client confirmation UX, so a regression here silently changes how
// assistants treat the tools.
func TestAnnotationHelpers(t *testing.T) {
	t.Parallel()

	t.Run("readOnly", func(t *testing.T) {
		a := readOnlyAnnotations()
		assert.True(t, a.ReadOnlyHint, "read-only tools must set ReadOnlyHint")
		require.NotNil(t, a.DestructiveHint)
		assert.False(t, *a.DestructiveHint, "read-only tools are not destructive")
	})

	t.Run("destructive", func(t *testing.T) {
		a := destructiveAnnotations()
		assert.False(t, a.ReadOnlyHint)
		require.NotNil(t, a.DestructiveHint)
		assert.True(t, *a.DestructiveHint, "delete/remove tools must be destructive")
	})

	t.Run("additive", func(t *testing.T) {
		a := additiveAnnotations()
		assert.False(t, a.ReadOnlyHint)
		require.NotNil(t, a.DestructiveHint)
		assert.False(t, *a.DestructiveHint, "create/add tools are additive, not destructive")
		assert.False(t, a.IdempotentHint, "creating twice is not idempotent")
	})

	t.Run("idempotent", func(t *testing.T) {
		a := idempotentAnnotations()
		assert.False(t, a.ReadOnlyHint)
		require.NotNil(t, a.DestructiveHint)
		assert.False(t, *a.DestructiveHint, "update/set tools are non-destructive")
		assert.True(t, a.IdempotentHint, "re-applying the same update has no additional effect")
	})
}
