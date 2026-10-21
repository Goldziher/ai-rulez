package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// LoadSkillDir feeds the lint targets, which report a malformed frontmatter as
// AR306, so it must record the flaw on the skill without also warning about it.
func TestLoadSkillDirDoesNotWarnAboutMalformedFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		skillMD string
		wantBad bool
	}{
		{"unparseable frontmatter", "---\nname: bad\ndescription: Args: a, b\n---\nBody\n", true},
		{"valid frontmatter", "---\nname: bad\ndescription: fine\n---\nBody\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			dir := filepath.Join(root, "skills", "bad")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(tt.skillMD), 0o644)) //nolint:gosec // test fixture
			rec := &testutil.LogRecorder{}
			ctx := logger.WithContext(t.Context(), rec)

			// Act
			skill, ok := LoadSkillDir(ctx, root, dir)

			// Assert
			require.True(t, ok)
			assert.Equal(t, tt.wantBad, skill.MalformedFrontmatter)
			assert.Empty(t, rec.Level("WARN"), rec.String())
		})
	}
}
