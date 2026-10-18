package repoindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExcluder(t *testing.T) {
	tests := []struct {
		name     string
		globs    []string
		path     string
		wantPath bool
		wantDir  bool
	}{
		{name: "no globs excludes nothing", path: "a/b.md"},
		{name: "a directory glob excludes files and the directory", globs: []string{"vendor/**"}, path: "vendor/x.md", wantPath: true, wantDir: true},
		{name: "a directory glob does not match a sibling", globs: []string{"vendor/**"}, path: "vendors/x.md"},
		{name: "a bare name matches a directory in the path", globs: []string{"pkg"}, path: "pkg/api/AGENTS.md", wantPath: true, wantDir: true},
		{name: "a file glob excludes the file, not its directory", globs: []string{"**/*.mdc"}, path: "a/ui.mdc", wantPath: true},
		{name: "a literal file path", globs: []string{"docs/AGENTS.md"}, path: "docs/AGENTS.md", wantPath: true},
		{name: "a nested glob matches at depth", globs: []string{"**/fixtures/**"}, path: "a/b/fixtures/c/AGENTS.md", wantPath: true, wantDir: true},
		{name: "a leading ./ and a backslash are tolerated", globs: []string{"./vendor\\**"}, path: "vendor/x.md", wantPath: true, wantDir: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			e, err := NewExcluder(tt.globs)
			require.NoError(t, err)

			// Act
			gotPath := e.Excluded(tt.path)
			gotDir := e.ExcludedDir(parentOf(tt.path))

			// Assert
			assert.Equal(t, tt.wantPath, gotPath)
			assert.Equal(t, tt.wantDir, gotDir)
		})
	}
}
