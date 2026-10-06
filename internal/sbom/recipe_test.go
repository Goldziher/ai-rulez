package sbom_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

// built renders the project with opts as CycloneDX.
func built(t *testing.T, p project, opts sbom.Options) string {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte(p.cfg), 0o644))
	for rel, content := range p.files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
	require.NoError(t, err)
	bom, err := sbom.Build(cfg, "9.9.9", opts)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, sbom.Render(&buf, bom, sbom.FormatCycloneDX))
	return buf.String()
}

func TestRecipeOfReadsTheSliceAMadeDocumentRecords(t *testing.T) {
	p := project{cfg: baseConfig + mcpConfig + "\n[profiles]\nbackend = [\"backend\"]\n", files: sampleFiles}
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		made sbom.Options
		want sbom.Options
	}{
		{"the default document", sbom.Options{}, sbom.Options{}},
		{"files and a profile", sbom.Options{Files: sbom.FilesSkills, Profile: "backend"}, sbom.Options{Files: sbom.FilesSkills, Profile: "backend"}},
		{"a recorded time", sbom.Options{Timestamp: stamp}, sbom.Options{Timestamp: stamp}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			doc := built(t, p, tt.made)

			// Act
			got, ok := sbom.RecipeOf([]byte(doc))

			// Assert
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRecipeOfRebuildsADocumentThatDriftsOnlyWhenTheConfigurationDid(t *testing.T) {
	// Arrange: a committed document made with a role-less, files-listing recipe
	p := project{cfg: baseConfig + mcpConfig, files: sampleFiles}
	committed := built(t, p, sbom.Options{Files: sbom.FilesSkills})
	recipe, ok := sbom.RecipeOf([]byte(committed))
	require.True(t, ok)

	// Act
	same, err := sbom.Drift([]byte(committed), []byte(built(t, p, recipe)))
	changed := project{cfg: baseConfig + mcpConfig, files: map[string]string{"rules/r1.md": "# R1\n\nchanged\n"}}
	diffs, derr := sbom.Drift([]byte(committed), []byte(built(t, changed, recipe)))

	// Assert
	require.NoError(t, err)
	require.NoError(t, derr)
	assert.Empty(t, same, "the recipe reproduces the committed document")
	assert.NotEmpty(t, diffs, "a real change in the sources is drift")
}

func TestRecipeOfRefusesWhatItCannotRebuild(t *testing.T) {
	tests := []struct{ name, doc string }{
		{"not json", "nope"},
		{"another tool's SBOM", `{"bomFormat":"CycloneDX","metadata":{"component":{"name":"x"}}}`},
		{"a spdx document", `{"spdxVersion":"SPDX-2.3"}`},
		{"a recorded attestation check", `{"bomFormat":"CycloneDX","metadata":{"component":{"properties":[{"name":"ai-rulez:tree","value":"sha256:a"},{"name":"ai-rulez:signature","value":"verified"}]}}}`},
		{"redacted reviewers", `{"bomFormat":"CycloneDX","metadata":{"component":{"properties":[{"name":"ai-rulez:tree","value":"sha256:a"}]}},"components":[{"bom-ref":"x","properties":[{"name":"ai-rulez:approvers","value":"reviewer-0a1b2c3d"}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, ok := sbom.RecipeOf([]byte(tt.doc))

			// Assert
			assert.False(t, ok)
		})
	}
}
