package sbom_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

func property(c sbom.Component, name string) string {
	for _, p := range c.Properties {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

func TestSerialNumberIsDerivedFromTheLockTree(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	root := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte(baseConfig), 0o644))
	rule := filepath.Join(root, "rules", "r1.md")
	require.NoError(t, os.WriteFile(rule, []byte("# R1\n"), 0o644))
	load := func() *sbom.BOM {
		cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
		require.NoError(t, err)
		bom, err := sbom.Build(cfg, "9.9.9")
		require.NoError(t, err)
		return bom
	}
	noLock := load()
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutLocal())
	require.NoError(t, err)
	snap, err := govview.Snapshot(cfg, "", true, "9.9.9")
	require.NoError(t, err)
	lock := &lockfile.File{}
	contentlock.Build(lock, snap)
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))

	// Act
	withLock := load()
	require.NoError(t, os.WriteFile(rule, []byte("# R1 edited\n"), 0o644))
	drifted := load()

	// Assert
	assert.Equal(t, "absent", property(noLock.Metadata.Component, "ai-rulez:lock"))
	assert.Equal(t, "present", property(withLock.Metadata.Component, "ai-rulez:lock"))
	assert.Equal(t, lock.Tree, property(withLock.Metadata.Component, "ai-rulez:tree"))
	assert.Equal(t, noLock.SerialNumber, withLock.SerialNumber, "a fresh lock and the computed tree agree")
	assert.Equal(t, "true", property(withLock.Metadata.Component, "ai-rulez:lock-in-sync"))
	assert.Equal(t, "false", property(drifted.Metadata.Component, "ai-rulez:lock-in-sync"))
	assert.Equal(t, withLock.SerialNumber, drifted.SerialNumber, "the serial follows the lock tree, drift is reported in a property")
}
