package generator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// #368: a non-recursive generate of a parent whose scope writes under a
// top-level directory (packages/) must not treat the generated files of a
// nested root below it (packages/web) as orphans: that root owns them.
func TestGenerate_NonRecursiveLeavesNestedRootOutputsUnderAnOutputRoot(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root := t.TempDir()
	scopes := "\n[[scopes]]\npath = \"packages/api\"\npresets = [\"claude\"]\n"
	writeRootConfig(t, root, "version = \"5.0\"\nname = \"outer\"\ngitignore = false\npresets = [\"claude\"]\n"+v4Defaults+scopes)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/outer.md", "---\npriority: high\n---\n# Outer\n\nOUTER_BODY\n")
	nested := filepath.Join(root, "packages", "web")
	writeRootConfig(t, nested, "version = \"5.0\"\nname = \"inner\"\ngitignore = false\npresets = [\"claude\"]\n"+v4Defaults)
	writeAgentsMDFile(t, nested, ".ai-rulez/rules/scoped.md", removedRootGlobRule)
	generateBothRoots(t, root, nested)
	require.FileExists(t, filepath.Join(nested, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(nested, ".claude", "rules", "scoped.md"))

	// Act
	drift, err := loadHashesProject(t, root).CheckDrift("")
	require.NoError(t, err)
	require.NoError(t, loadHashesProject(t, root).Generate(""))

	// Assert
	for _, d := range drift {
		require.NotContains(t, d.Path, "packages/web/", "nested root file reported: %v", d)
	}
	require.FileExists(t, filepath.Join(nested, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(nested, ".claude", "rules", "scoped.md"))
}
