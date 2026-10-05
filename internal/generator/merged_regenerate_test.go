package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

const regenerateExtra = `
[[mcp_servers]]
name = "fs"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "."]

[permissions]
allow = ["Bash(npm run test:*)", "Read(./src/**)"]
deny = ["Read(./.env)", "Bash(rm -rf:*)"]

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
command = "echo pre"
`

// projectFiles reads every file below base outside .ai-rulez, keyed by path.
func projectFiles(t *testing.T, base string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if rel == ".ai-rulez" {
				return filepath.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		files[rel] = string(data)
		return readErr
	}))
	return files
}

// assertOwnedContentPresent checks that every owned key of every merged output
// reached the document on disk: the last segment of its path (and the name of an
// owned array element) appears in the file.
func assertOwnedContentPresent(t *testing.T, base string, outputs []config.OutputFile) {
	t.Helper()
	for _, out := range outputs {
		if out.Merge == nil || out.IsDir {
			continue
		}
		rel := out.Path
		if filepath.IsAbs(rel) {
			r, err := filepath.Rel(base, rel)
			require.NoError(t, err)
			rel = r
		}
		data, err := os.ReadFile(filepath.Join(base, rel))
		require.NoError(t, err, rel)
		for _, key := range out.Merge.Owned {
			if key.Remove {
				continue
			}
			segs := key.Segments()
			assert.Contains(t, string(data), segs[len(segs)-1], "%s lost owned key %v", rel, segs)
			for _, element := range key.Elements {
				if m, ok := element.(map[string]any); ok {
					if name, ok := m["name"].(string); ok {
						assert.Contains(t, string(data), name, "%s lost element %q of %v", rel, name, segs)
					}
				}
			}
		}
	}
}

// TestGenerate_MergedDocumentsSurviveRegeneration generates each preset twice (the
// second run reads the manifest the first wrote, with the outputs deleted as a
// fresh checkout of a git-ignored output would be), requires every sidecar kind's
// owned content in the shared documents, and then requires generate --check to be
// clean and clean to restore the project.
func TestGenerate_MergedDocumentsSurviveRegeneration(t *testing.T) {
	quietWarnings(t)
	for _, preset := range config.IndividualPresetNames() {
		t.Run(preset, func(t *testing.T) {
			// Arrange
			base := writeChecksProject(t, []string{preset}, regenerateFiles, regenerateExtra)
			cfg, err := config.LoadConfig(context.Background(), base)
			require.NoError(t, err)
			outputs, _, err := NewGenerator(cfg).collectOutputs("")
			require.NoError(t, err)

			// Act
			generateChecksProject(t, base, "default")
			first := projectFiles(t, base)
			assertOwnedContentPresent(t, base, outputs)
			generateChecksProject(t, base, "default")
			second := projectFiles(t, base)
			for rel := range first {
				if !strings.HasSuffix(rel, ".md") {
					require.NoError(t, os.Remove(filepath.Join(base, rel)))
				}
			}
			generateChecksProject(t, base, "default")
			third := projectFiles(t, base)

			// Assert
			assert.Equal(t, first, second, "a second generate changed the outputs")
			assert.Equal(t, first, third, "generate over a manifest with the outputs deleted changed them")
			assertOwnedContentPresent(t, base, outputs)
			cfg, err = config.LoadConfig(context.Background(), base)
			require.NoError(t, err)
			drift, err := NewGenerator(cfg).CheckDrift("default")
			require.NoError(t, err)
			assert.Empty(t, drift)

			_, err = NewGenerator(cfg).Clean("default", CleanOptions{})
			require.NoError(t, err)
			assert.Empty(t, projectFiles(t, base), "clean left generated files behind")
		})
	}
}

func TestGenerate_AllPresetsKeepEverySidecarKind(t *testing.T) {
	quietWarnings(t)
	base := writeChecksProject(t, config.IndividualPresetNames(), regenerateFiles, regenerateExtra)
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	outputs, _, err := NewGenerator(cfg).collectOutputs("")
	require.NoError(t, err)

	generateChecksProject(t, base, "default")
	first := projectFiles(t, base)
	generateChecksProject(t, base, "default")

	assert.Equal(t, first, projectFiles(t, base))
	assertOwnedContentPresent(t, base, outputs)
	assert.Contains(t, first[".vibe/config.toml"], "[[mcp_servers]]")
	cfg, err = config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	drift, err := NewGenerator(cfg).CheckDrift("default")
	require.NoError(t, err)
	assert.Empty(t, drift)
	_, err = NewGenerator(cfg).Clean("default", CleanOptions{})
	require.NoError(t, err)
	assert.Empty(t, projectFiles(t, base))
}

var regenerateFiles = map[string]string{"rules/style.md": "---\npriority: high\n---\n\nUse tabs.\n"}

func TestSubtractClaims_ComparesElementsAsJSON(t *testing.T) {
	// Arrange: the previous record as the manifest decodes it, the current claim
	// as the renderer builds it (typed args).
	var previous []jsonmerge.Claim
	require.NoError(t, json.Unmarshal([]byte(`[{"path":["mcp_servers"],"elements":[{"name":"fs","args":["-y","."]}]}]`), &previous))
	current := []jsonmerge.Claim{{
		Path:     []string{"mcp_servers"},
		Elements: []any{map[string]any{"name": "fs", "args": []string{"-y", "."}}},
	}}

	// Act
	gone := subtractClaims(previous, current)

	// Assert
	assert.Empty(t, gone)
}
