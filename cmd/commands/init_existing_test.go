package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestHandWrittenNativeFiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  []string
	}{
		{"nothing there", func(t *testing.T, dir string) {}, nil},
		{"a hand-written CLAUDE.md", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Mine\n"), 0o644))
		}, []string{"CLAUDE.md"}},
		{"a generated AGENTS.md is not hand-written", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"),
				[]byte("<!-- GENERATED FILE - DO NOT EDIT -->\n\n# Rules\n"), 0o644))
		}, nil},
		{"an empty file holds nothing", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), nil, 0o644))
		}, nil},
		{"a symlinked CLAUDE.md and a plain AGENTS.md", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Mine\n"), 0o644))
			testutil.SymlinkOrSkip(t, "AGENTS.md", filepath.Join(dir, "CLAUDE.md"))
		}, []string{"AGENTS.md", "CLAUDE.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			tt.setup(t, dir)

			// Act
			got := handWrittenNativeFiles(dir)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
