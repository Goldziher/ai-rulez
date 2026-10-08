package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

type captureLog struct {
	mu   sync.Mutex
	info []string
}

func (c *captureLog) Debug(string, ...any) {}
func (c *captureLog) Warn(string, ...any)  {}
func (c *captureLog) Error(string, ...any) {}
func (c *captureLog) Info(msg string, _ ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info = append(c.info, msg)
}

func (c *captureLog) deprecations() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, m := range c.info {
		if strings.Contains(m, "legacy layout") {
			n++
		}
	}
	return n
}

func legacyProject(t *testing.T, okfRoot bool) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "rules", "r.md"), []byte("# R\n"), 0o644))
	if okfRoot {
		require.NoError(t, os.WriteFile(filepath.Join(cfg, "index.md"), []byte("---\nokf_version: \"0.2\"\n---\n\n# Concepts\n\n* [R](rules/r.md)\n"), 0o644))
	}
	return dir
}

func TestLegacyLayoutIsDeprecated(t *testing.T) {
	log := &captureLog{}

	_, err := LoadConfig(context.Background(), legacyProject(t, false), WithHost(ambient.Host{Log: log}))

	require.NoError(t, err)
	assert.Equal(t, 1, log.deprecations(), "a tree without an OKF root index.md is the legacy layout: %v", log.info)
}

func TestOKFBundleIsNotDeprecated(t *testing.T) {
	log := &captureLog{}

	_, err := LoadConfig(context.Background(), legacyProject(t, true), WithHost(ambient.Host{Log: log}))

	require.NoError(t, err)
	assert.Zero(t, log.deprecations())
}

func TestEmptyConfigDirIsNotDeprecated(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfg, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n"), 0o644))
	log := &captureLog{}

	_, err := LoadConfig(context.Background(), dir, WithHost(ambient.Host{Log: log}))

	require.NoError(t, err)
	assert.Zero(t, log.deprecations(), "nothing to migrate")
}
