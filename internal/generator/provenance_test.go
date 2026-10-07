package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaceByHand overwrites CLAUDE.md with text that has no banner and no hash,
// leaving the manifest that lists it in place.
func replaceByHand(t *testing.T, dir string) string {
	t.Helper()
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))
	return claude
}

func TestGenerate_RefusesAManifestListedFileReplacedByHand(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	generateHashesProject(t, dir)
	claude := replaceByHand(t, dir)

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE.md")
	data, rerr := os.ReadFile(claude)
	require.NoError(t, rerr)
	assert.Equal(t, handWritten, string(data))
}

func TestGenerateAndClean_AgreeOnAHandReplacedManifestListedFile(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	generateHashesProject(t, dir)
	claude := replaceByHand(t, dir)

	// Act
	genErr := newProjectGenerator(t, dir).Generate("default")
	plan, cleanErr := newProjectGenerator(t, dir).Clean("default", CleanOptions{})

	// Assert
	require.Error(t, genErr, "generate refuses what clean keeps")
	require.NoError(t, cleanErr)
	assert.NotContains(t, plan.Files, claude)
}

func TestOutputProvenance(t *testing.T) {
	const rel = "out/agent.toml"
	const banner = "# GENERATED FILE - DO NOT EDIT\n\nbody\n"
	tests := []struct {
		name       string
		listed     bool
		digest     string
		headerless bool
		content    string
		want       provenance
	}{
		{"digest matches", true, fileDigest([]byte("x = 1\n")), true, "x = 1\n", provenStrong},
		{"digest mismatch is a hand edit", true, fileDigest([]byte("x = 1\n")), true, "x = 2\n", provenNone},
		{"old manifest without a digest, headerless format", true, "", true, "x = 1\n", provenLegacy},
		{"old manifest without a digest, format with a header", true, "", false, "# mine\n", provenNone},
		{"listed with a banner", true, "", false, banner, provenStrong},
		{"banner but unlisted", false, "", false, banner, provenWeak},
		{"unlisted and bare", false, "", true, "x = 1\n", provenNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := hashesProject(t, "")
			generateHashesProject(t, dir)
			gen := newProjectGenerator(t, dir)
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			require.NoError(t, os.WriteFile(abs, []byte(tt.content), 0o644))
			var files []string
			if tt.listed {
				files = append(files, rel)
			}
			digests := map[string]string{}
			if tt.digest != "" {
				digests[rel] = tt.digest
			}
			require.NoError(t, gen.writeManifest(gen.manifestPath(), files, nil, nil))
			require.NoError(t, gen.writeManifest(gen.localManifestPath(), nil, nil, digests))

			// Act
			got := gen.outputProvenance(abs, rel, []byte(tt.content), tt.headerless)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
