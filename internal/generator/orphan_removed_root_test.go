package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// removedRootGlobRule is a path-scoped rule, so the claude preset writes it to
// <root>/.claude/rules/scoped.md rather than inlining it into CLAUDE.md. The
// removed nested root therefore leaves both a root file and a rules file behind.
const removedRootGlobRule = "---\nglobs: [\"**/*.go\"]\n---\nSCOPED_BODY\n"

// removedRootProject writes a top-level root and a nested root, each generating
// CLAUDE.md and a .claude/rules file. It returns the project root and the nested
// root's directory.
func removedRootProject(t *testing.T) (root, nested string) {
	t.Helper()
	root = t.TempDir()
	writeRootConfig(t, root, "version = \"5.0\"\nname = \"outer\"\ngitignore = false\npresets = [\"claude\"]\n"+v4Defaults)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/outer.md", "---\npriority: high\n---\n# Outer\n\nOUTER_BODY\n")
	nested = filepath.Join(root, "nested")
	writeRootConfig(t, nested, "version = \"5.0\"\nname = \"inner\"\ngitignore = false\npresets = [\"claude\"]\n"+v4Defaults)
	writeAgentsMDFile(t, nested, ".ai-rulez/rules/scoped.md", removedRootGlobRule)
	return root, nested
}

// generateBothRoots generates the top-level root and the nested root.
func generateBothRoots(t *testing.T, root, nested string) {
	t.Helper()
	require.NoError(t, loadHashesProject(t, root).Generate(""))
	require.NoError(t, loadHashesProject(t, nested).Generate(""))
}

// #345: removing an entire nested root leaves its generated CLAUDE.md and
// .claude/rules/*.md in a directory no current root emits. Without a manifest,
// --check reports them as orphans and generate removes them.
func TestGenerate_RemovesAndReportsALeftoverFromARemovedNestedRoot(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root, nested := removedRootProject(t)
	generateBothRoots(t, root, nested)
	require.FileExists(t, filepath.Join(nested, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(nested, ".claude", "rules", "scoped.md"))

	// The nested root's .ai-rulez is removed: the whole root is gone.
	require.NoError(t, os.RemoveAll(filepath.Join(nested, ".ai-rulez")))

	// Act: --check reports the leftovers, then generate removes them.
	drift, err := loadHashesProject(t, root).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, drift, Drift{Path: "nested/CLAUDE.md", Kind: DriftOrphan})
	assert.Contains(t, drift, Drift{Path: "nested/.claude/rules/scoped.md", Kind: DriftOrphan})
	require.FileExists(t, filepath.Join(nested, "CLAUDE.md"), "--check must not write")
	require.NoError(t, loadHashesProject(t, root).Generate(""))
	assert.NoFileExists(t, filepath.Join(nested, "CLAUDE.md"))
	assert.NoFileExists(t, filepath.Join(nested, ".claude", "rules", "scoped.md"))
}

// #345: a leftover whose body was edited by hand no longer matches its own
// Content-Hash, so it is kept: the orphan pass only removes a clean leftover.
func TestGenerate_KeepsAHandEditedLeftoverFromARemovedNestedRoot(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root, nested := removedRootProject(t)
	generateBothRoots(t, root, nested)
	require.NoError(t, os.RemoveAll(filepath.Join(nested, ".ai-rulez")))
	claude := filepath.Join(nested, "CLAUDE.md")
	appendTo(t, claude, "\nmy own note\n")

	// Act
	drift, err := loadHashesProject(t, root).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, drift, Drift{Path: "nested/CLAUDE.md", Kind: DriftOrphan})
	require.NoError(t, loadHashesProject(t, root).Generate(""))
	assert.FileExists(t, claude)
	assert.Contains(t, readFileString(t, claude), "my own note")
}

// #345: a symlinked leftover is the user's, so it is left in place.
func TestGenerate_LeavesASymlinkedLeftoverFromARemovedNestedRoot(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root, nested := removedRootProject(t)
	generateBothRoots(t, root, nested)
	require.NoError(t, os.RemoveAll(filepath.Join(nested, ".ai-rulez")))
	claude := filepath.Join(nested, "CLAUDE.md")
	require.NoError(t, os.Remove(claude))
	elsewhere := filepath.Join(root, "elsewhere.md")
	require.NoError(t, os.WriteFile(elsewhere, []byte("# mine\n\nKEEP_ME\n"), 0o644))
	testutil.SymlinkOrSkip(t, elsewhere, claude)

	// Act
	drift, err := loadHashesProject(t, root).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, drift, Drift{Path: "nested/CLAUDE.md", Kind: DriftOrphan})
	require.NoError(t, loadHashesProject(t, root).Generate(""))
	info, lerr := os.Lstat(claude)
	require.NoError(t, lerr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is the user's")
}

// #345: while a nested root still exists its generated files are its own run's
// to report and remove, so the top-level walk must not claim them. The shared
// root AGENTS.md the top-level root still emits is likewise not an orphan.
func TestGenerate_DoesNotReportAFileACurrentRootStillEmits(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root, nested := removedRootProject(t)
	generateBothRoots(t, root, nested)

	// Act: the nested root is still present, so the top-level root is in sync.
	drift, err := loadHashesProject(t, root).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, drift, "a nested current root's files are not the top-level root's orphans")
	require.FileExists(t, filepath.Join(nested, "CLAUDE.md"))
}

// #345: the stray walk is bounded by the recursive discovery's prune list, so a
// generated file inside a dependency tree is never reported or removed.
func TestGenerate_IgnoresLeftoversInsidePrunedTrees(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root, nested := removedRootProject(t)
	generateBothRoots(t, root, nested)
	require.NoError(t, os.RemoveAll(filepath.Join(nested, ".ai-rulez")))
	// A copy of a generated file planted in node_modules must be ignored.
	deps := filepath.Join(root, "node_modules", "pkg")
	require.NoError(t, os.MkdirAll(deps, 0o755))
	data, rerr := os.ReadFile(filepath.Join(nested, "CLAUDE.md"))
	require.NoError(t, rerr)
	planted := filepath.Join(deps, "CLAUDE.md")
	require.NoError(t, os.WriteFile(planted, data, 0o644))

	// Act
	drift, err := loadHashesProject(t, root).CheckDrift("")

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, drift, Drift{Path: "node_modules/pkg/CLAUDE.md", Kind: DriftOrphan})
	require.NoError(t, loadHashesProject(t, root).Generate(""))
	assert.FileExists(t, planted, "a file inside a pruned tree is left alone")
}

// #376: the stray-tree walk reads a header only from files that can be a preset
// output (a root file name or a file below an output directory), not from every
// source file in the work tree.
func TestStrayCandidateFilter_SkipsOrdinarySourceFiles(t *testing.T) {
	root, _ := removedRootProject(t)
	g := loadHashesProject(t, root)
	base := filepath.Clean(g.config.BaseDir)
	mayBeOutput := g.strayCandidateFilter(base)

	tests := []struct {
		rel  string
		want bool
	}{
		{"nested/CLAUDE.md", true},
		{"nested/.claude/rules/scoped.md", true},
		{"pkg/deep/.claude/skills/x/SKILL.md", true},
		{"src/main.go", false},
		{"lib/guide.md", false},
		{"a/b/c/data.json", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, mayBeOutput(filepath.Join(base, filepath.FromSlash(tt.rel))), tt.rel)
	}
}
