package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const overlayWithHTTPSecret = `[[mcp_servers]]
name = "svc"
transport = "http"
url = "https://example.com/mcp"
[mcp_servers.headers]
Authorization = "Bearer HEADER-SECRET-1"
`

// localOutputs are the files the overlay above makes the claude preset write.
var localOutputs = []string{".mcp.json", ".claude/settings.json"}

func generateWithSecretOverlay(t *testing.T) *driftProject {
	t.Helper()
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	p.overlay(t, overlayWithHTTPSecret)
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	manifest := p.read(t, ".ai-rulez/.generated-manifest.local.json")
	for _, rel := range localOutputs {
		require.Contains(t, manifest, rel)
		require.True(t, p.exists(rel))
	}
	require.NoError(t, os.Remove(filepath.Join(p.dir, "config.local.toml")))
	return p
}

func TestClean_RemovesLocalManifestFilesAfterTheOverlayIsDeleted(t *testing.T) {
	// Arrange
	p := generateWithSecretOverlay(t)

	// Act
	_, err := NewGenerator(p.load(t)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	for _, rel := range localOutputs {
		assert.False(t, p.exists(rel), "%s holds the overlay secret and must be removed", rel)
	}
	assert.False(t, p.exists(".ai-rulez/.generated-manifest.local.json"))
}

func TestGenerate_RemovesLocalManifestFilesAfterTheOverlayIsDeleted(t *testing.T) {
	// Arrange
	p := generateWithSecretOverlay(t)

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	for _, rel := range localOutputs {
		assert.False(t, p.exists(rel), "%s is stale once the overlay is gone", rel)
	}
}
