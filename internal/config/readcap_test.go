package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bigFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))
	require.NoError(t, f.Close())
	return p
}

func TestReadCapped(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"small", 10, false},
		{"at the cap", maxContentFileBytes, false},
		{"over the cap", maxContentFileBytes + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := bigFile(t, dir, tt.name, tt.size)
			data, err := readCapped(osView(dir), p)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "MiB limit")
				return
			}
			require.NoError(t, err)
			assert.Len(t, data, int(tt.size))
		})
	}
}

func TestOversizedRepositoryFilesAreRefused(t *testing.T) {
	dir := t.TempDir()
	big := bigFile(t, dir, "big.md", maxContentFileBytes+1)
	loaders := map[string]func(string) (*Config, error){"toml": func(p string) (*Config, error) { return loadConfigTOML(osView(dir), p) }}
	for ext, load := range loaders {
		cfg := bigFile(t, dir, "config."+ext, maxContentFileBytes+1)
		_, err := load(cfg)
		require.Error(t, err, ext)
		assert.Contains(t, err.Error(), "MiB limit", ext)
	}
	_, err := readContentFile(osView(dir), big)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MiB limit")
}
