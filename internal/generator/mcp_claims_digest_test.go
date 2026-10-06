package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_MergedClaimsNeverRecordSecretValues(t *testing.T) {
	// Arrange: vibe merges the whole server (env included) into an array of tables.
	root := newSecretMCPRepo(t, `["vibe", "claude"]`)

	// Act
	generateRepo(t, root)

	// Assert: no manifest holds the secret, and the committed or local record
	// still names what ai-rulez owns.
	for _, name := range []string{".generated-manifest.json", ".generated-manifest.local.json"} {
		data, err := os.ReadFile(filepath.Join(root, ".ai-rulez", name))
		if err != nil {
			continue
		}
		assert.NotContains(t, string(data), leakToken, name)
	}
	local, err := os.ReadFile(filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json"))
	require.NoError(t, err)
	assert.Contains(t, string(local), `"elementSums"`)
	assert.NotContains(t, string(local), `"elements"`)
}

func TestClean_AfterDigestClaimsRestoresDocument(t *testing.T) {
	// Arrange: the user's own vibe server sits beside the generated one.
	root := newSecretMCPRepo(t, `["vibe"]`)
	vibe := filepath.Join(root, ".vibe", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(vibe), 0o755))
	seed := "[[mcp_servers]]\ncommand = \"x\"\nname = \"mine\"\n"
	require.NoError(t, os.WriteFile(vibe, []byte(seed), 0o644))
	// A hand-authored document holding a secret is the user's to ignore.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".vibe/config.toml\n"), 0o644))
	generateRepo(t, root)
	generated, err := os.ReadFile(vibe)
	require.NoError(t, err)
	require.Contains(t, string(generated), "github")

	// Act
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	_, err = NewGenerator(cfg).Clean("default", CleanOptions{})
	require.NoError(t, err)

	// Assert
	restored, err := os.ReadFile(vibe)
	require.NoError(t, err)
	assert.Equal(t, seed, string(restored))
}
