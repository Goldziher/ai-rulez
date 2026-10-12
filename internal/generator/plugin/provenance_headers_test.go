package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// skillProject writes a one-skill content tree and returns the base dir and the
// skill's source bytes.
func skillProject(t *testing.T) (dir string, source []byte) {
	t.Helper()
	dir = t.TempDir()
	skillDir := filepath.Join(dir, "skills", "demo")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	source = []byte("---\nname: demo\ndescription: d\n---\n\n# Demo\n")
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), source, 0o644))
	return dir, source
}

func TestGenerateWithoutProvenanceHeadersBundlesContentVerbatim(t *testing.T) {
	// Arrange
	dir, source := skillProject(t)
	off := false
	cfg := &config.Config{BaseDir: dir, Plugin: &config.PluginAuthoring{
		Name:              "demo",
		Description:       "d",
		Version:           "1.0.0",
		Runtimes:          []string{config.PluginRuntimeClaude},
		ProvenanceHeaders: &off,
	}}
	tree := &config.ContentTree{Skills: []config.ContentFile{
		{Name: "demo", Path: filepath.Join(dir, "skills", "demo", "SKILL.md")},
	}}
	m, err := BuildManifest(cfg, tree)
	require.NoError(t, err)

	// Act
	outputs, err := Generate(m, dir)

	// Assert
	require.NoError(t, err)
	skill := outputByPath(t, outputs, "skills/demo/SKILL.md")
	assert.Equal(t, source, skill.RawContent, "provenance_headers = false writes the skill verbatim")

	// A header-less bundle still verifies against its sidecar.
	for _, output := range outputs {
		require.NoError(t, os.MkdirAll(filepath.Dir(output.Path), 0o755))
		require.NoError(t, os.WriteFile(output.Path, output.RawContent, 0o644))
	}
	require.NoError(t, VerifyProvenance(dir))
}

func TestGenerateWithProvenanceHeadersAddsTheHeader(t *testing.T) {
	// Arrange
	dir, _ := skillProject(t)
	cfg := &config.Config{BaseDir: dir, Plugin: &config.PluginAuthoring{
		Name:        "demo",
		Description: "d",
		Version:     "1.0.0",
		Runtimes:    []string{config.PluginRuntimeClaude},
	}}
	tree := &config.ContentTree{Skills: []config.ContentFile{
		{Name: "demo", Path: filepath.Join(dir, "skills", "demo", "SKILL.md")},
	}}
	m, err := BuildManifest(cfg, tree)
	require.NoError(t, err)

	// Act
	outputs, err := Generate(m, dir)

	// Assert
	require.NoError(t, err)
	skill := outputByPath(t, outputs, "skills/demo/SKILL.md")
	assert.Contains(t, string(skill.RawContent), "AI-RULEZ :: GENERATED FILE")
}
