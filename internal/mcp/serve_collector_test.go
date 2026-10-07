package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The live reload rebuilds the catalog from a fresh load of the project on every
// change. Its loads share one collector, so a warning about the project is said
// once for the life of the server, not on every reload.
func TestServeSetup_ReloadsShareOneWarningCollector(t *testing.T) {
	// Arrange
	root := project(t, baseConfig+"\n[lint.budget]\nAR201 = 2\n", map[string]string{
		"skills/core/SKILL.md": skillFile("core", "Core conventions", ""),
	})
	setup := &ServeSetup{WorkDir: root, NoWatch: true, CacheDir: filepath.Join(t.TempDir(), "cache")}
	_, err := setup.NewServer(context.Background())
	require.NoError(t, err)

	// Act
	reloaded, err := setup.build(context.Background(), buildOptions{admit: true})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, setup.Collector)
	assert.Same(t, setup.Collector, reloaded.cfg.Diag, "the reload loaded the project into the server's collector")
}
