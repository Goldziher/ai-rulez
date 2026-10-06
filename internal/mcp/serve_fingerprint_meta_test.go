package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only the cache bookkeeping files at the root of a watched directory are left
// out of the fingerprint (the lock digest leaves out the same two); a file that
// merely starts with the name, or has it deeper down, is skill content.
func TestFingerprint_CacheMetaIsExcludedOnlyAtTheRootByExactName(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		wantsReload  bool
		wantsContent string
	}{
		{"bookkeeping file at the root", ".cache_meta.json", false, `{"a":1}`},
		{"its atomic-write temp file at the root", ".cache_meta.json.tmp", false, "tmp"},
		{"a longer name with the same prefix", ".cache_meta.json.bak", true, "authored"},
		{"a file that merely starts with the prefix", ".cache_metadata", true, "authored"},
		{"the same name inside a skill", "skills/a/.cache_meta.json", true, "authored"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeFile(t, dir, "skills/a/SKILL.md", "x")
			before, err := fingerprint([]string{dir})
			require.NoError(t, err)

			// Act
			writeFile(t, dir, tt.file, tt.wantsContent)
			after, err := fingerprint([]string{dir})
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.wantsReload, before != after)
		})
	}
}
