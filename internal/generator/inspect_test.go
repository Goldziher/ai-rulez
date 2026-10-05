package generator

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergedDocumentPaths_StaysInsideTheProject(t *testing.T) {
	// Arrange: a manifest (it is a committed file, so anyone can edit it) that
	// names documents outside the project.
	base := t.TempDir()
	configDir := filepath.Join(base, ".ai-rulez")
	claim := []jsonmerge.Claim{{Path: []string{"mcpServers"}}}
	g := NewGenerator(&config.Config{BaseDir: base, ConfigDir: configDir})
	require.NoError(t, writeManifestFile(g.manifestPath(), nil, map[string][]jsonmerge.Claim{
		".mcp.json":           claim,
		"../outside.json":     claim,
		"a/../../escape.json": claim,
		"nested/ok.json":      claim,
	}))

	// Act
	got := g.MergedDocumentPaths()

	// Assert
	assert.Equal(t, []string{
		filepath.Join(base, ".mcp.json"),
		filepath.Join(base, "nested", "ok.json"),
	}, got)
}

func TestGeneratedPaths_ListsRecordedOutputsAndMergedDocuments(t *testing.T) {
	// Arrange
	base := t.TempDir()
	g := NewGenerator(&config.Config{BaseDir: base, ConfigDir: filepath.Join(base, ".ai-rulez")})
	require.NoError(t, writeManifestFile(g.manifestPath(),
		[]string{"CLAUDE.md", "../outside.md", ".claude/rules/a.md"},
		map[string][]jsonmerge.Claim{".mcp.json": {{Path: []string{"mcpServers"}}}}))

	// Act
	got := NewGenerator(&config.Config{BaseDir: base, ConfigDir: filepath.Join(base, ".ai-rulez")}).GeneratedPaths()

	// Assert
	assert.Equal(t, []string{
		filepath.Join(base, ".claude", "rules", "a.md"),
		filepath.Join(base, ".mcp.json"),
		filepath.Join(base, "CLAUDE.md"),
	}, got)
}
