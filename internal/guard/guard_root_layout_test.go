package guard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rootManifestBody = `{"version":"1","files":[".claude/skills/demo/SKILL.md",".mcp.json"]}`

func newRootLayoutProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	writeFile(t, root, "ai-rulez.toml", "version = \"5.0\"\n")
	writeFile(t, root, ".ai-rulez-manifest.json", rootManifestBody)
	writeFile(t, root, ".claude/skills/demo/SKILL.md", "---\nname: demo\n---\n")
	writeFile(t, root, "CLAUDE.md", "# Hand written\n")
	writeFile(t, root, "AGENTS.md", "# Hand written\n")
	return root
}

func TestCheck_RootLayoutManifest(t *testing.T) {
	root := newRootLayoutProject(t)
	tests := []struct {
		name      string
		file      string
		wantBlock bool
	}{
		{"a generated skill is guarded", ".claude/skills/demo/SKILL.md", true},
		{"a generated MCP file is guarded", ".mcp.json", true},
		{"the hand-written CLAUDE.md stays editable", "CLAUDE.md", false},
		{"the hand-written AGENTS.md stays editable", "AGENTS.md", false},
		{"the configuration itself stays editable", "ai-rulez.toml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			payload := `{"tool_name":"Edit","tool_input":{"file_path":"` + esc(filepath.Join(root, tt.file)) + `"}}`

			// Act
			got := Check(strings.NewReader(payload), root)

			// Assert
			assert.Equal(t, tt.wantBlock, got.Block)
			if tt.wantBlock {
				assert.Equal(t, tt.file, got.Path)
			}
		})
	}
}

func TestCheck_RootLayoutLocalManifestMustBeTrusted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not carry the trust signal on Windows")
	}
	// Arrange
	root := newRootLayoutProject(t)
	local := filepath.Join(root, ".ai-rulez-manifest.local.json")
	writeFile(t, root, ".ai-rulez-manifest.local.json", `{"version":"1","files":["CLAUDE.local.md"]}`)
	require.NoError(t, os.Chmod(local, 0o600))
	check := func() Decision {
		payload := `{"tool_name":"Edit","tool_input":{"file_path":"CLAUDE.local.md"},"cwd":"` + esc(root) + `"}`
		return Check(strings.NewReader(payload), root)
	}

	// Act
	trusted := check()
	require.NoError(t, os.Chmod(local, 0o666)) //nolint:gosec // a forged, world-writable manifest is the case under test
	forged := check()

	// Assert
	assert.True(t, trusted.Block, "an owner-only local manifest protects its outputs")
	assert.False(t, forged.Block, "a world-writable local manifest must not decide what is protected")
}

func TestFindProject_RootLayoutManifestMarksTheProject(t *testing.T) {
	// Arrange
	root := newRootLayoutProject(t)
	nested := filepath.Join(root, "src", "deep")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	// Act
	found, ok := findProject(nested)

	// Assert
	assert.True(t, ok)
	assert.Equal(t, root, found)
}

func TestManifestRefs_ListsTheRootLayoutManifests(t *testing.T) {
	// Act
	refs := manifestRefs("/p")

	// Assert
	want := []manifestRef{
		{path: filepath.Join("/p", ".ai-rulez", ".generated-manifest.json")},
		{path: filepath.Join("/p", ".ai-rulez", ".generated-manifest.local.json"), local: true},
		{path: filepath.Join("/p", ".agents", ".ai-rulez", ".generated-manifest.json")},
		{path: filepath.Join("/p", ".agents", ".ai-rulez", ".generated-manifest.local.json"), local: true},
		{path: filepath.Join("/p", ".ai-rulez-manifest.json")},
		{path: filepath.Join("/p", ".ai-rulez-manifest.local.json"), local: true},
	}
	assert.Equal(t, want, refs)
}
