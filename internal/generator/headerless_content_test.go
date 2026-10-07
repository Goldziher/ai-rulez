package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// codexAgentProject is a project whose codex preset renders the `helper` agent
// as .codex/agents/helper.toml: an output built from Content that carries no
// generated banner.
func codexAgentProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCodexAgent(t, dir, "Help.")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"),
		[]byte("version = \"4.0\"\nname = \"x\"\npresets = [\"codex\"]\ngitignore = false\n"), 0o644))
	return dir
}

func writeCodexAgent(t *testing.T, dir, body string) {
	t.Helper()
	agents := filepath.Join(dir, ".ai-rulez", "agents")
	require.NoError(t, os.MkdirAll(agents, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agents, "helper.md"),
		[]byte("---\nname: helper\ndescription: Use when helping with code.\n---\n"+body+"\n"), 0o644))
}

func TestGenerate_OverwritesAHeaderlessContentOutputAnOlderRunListed(t *testing.T) {
	tests := []struct {
		name string
		// lose drops what this machine recorded about the first run.
		lose func(t *testing.T, gen *Generator)
	}{
		{
			name: "fresh clone: no machine-local manifest",
			lose: func(t *testing.T, gen *Generator) {
				t.Helper()
				require.NoError(t, os.Remove(gen.localManifestPath()))
			},
		},
		{
			name: "upgrade: the local manifest recorded no digests",
			lose: func(t *testing.T, gen *Generator) {
				t.Helper()
				require.NoError(t, gen.writeManifest(gen.localManifestPath(), nil, nil, nil))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := codexAgentProject(t)
			require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
			toml := filepath.Join(dir, ".codex", "agents", "helper.toml")
			first, err := os.ReadFile(toml)
			require.NoError(t, err)
			require.NotContains(t, string(first), "GENERATED", "the Codex agent TOML has no banner")
			tt.lose(t, newProjectGenerator(t, dir))
			writeCodexAgent(t, dir, "Help again.")

			warned, host := capturedMergeWarnings()
			cfg, err := config.LoadConfig(context.Background(), dir, host)
			require.NoError(t, err)

			// Act
			err = NewGenerator(cfg).Generate("default")

			// Assert
			require.NoError(t, err, "a manifest-listed header-less output is not a hand-written file")
			data, rerr := os.ReadFile(toml)
			require.NoError(t, rerr)
			assert.Contains(t, string(data), "Help again.")
			assert.Equal(t, 1, countContaining(warned.Level("WARN"), "Overwriting .codex/agents/helper.toml"), warned.String())
		})
	}
}

func TestGenerate_RefusesAHeaderlessContentOutputEditedAfterItsDigest(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := codexAgentProject(t)
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	toml := filepath.Join(dir, ".codex", "agents", "helper.toml")
	f, err := os.OpenFile(toml, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("\n# my edit\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	edited, err := os.ReadFile(toml)
	require.NoError(t, err)
	writeCodexAgent(t, dir, "Help again.")

	// Act
	err = newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".codex/agents/helper.toml")
	data, rerr := os.ReadFile(toml)
	require.NoError(t, rerr)
	assert.Equal(t, string(edited), string(data))
}
