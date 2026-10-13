package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteConfigLayers(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"config.toml", "config.ai.toml", "config.custom.toml", "config.local.toml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o644))
	}

	var text bytes.Buffer
	require.NoError(t, writeConfigLayers(&text, dir, []string{"ai", "custom"}, map[string]string{"ai": "default", "custom": "env"}, false))
	assert.Contains(t, text.String(), "active (1, default)")
	assert.Contains(t, text.String(), "active (2, env)")
	assert.Contains(t, text.String(), "config.ai.toml")
	assert.NotContains(t, text.String(), "local")

	var js bytes.Buffer
	require.NoError(t, writeConfigLayers(&js, dir, []string{"custom"}, map[string]string{"custom": "env"}, true))
	assert.Contains(t, js.String(), `"active": false`)
	assert.Contains(t, js.String(), `"origin": "env"`)

	var none bytes.Buffer
	require.NoError(t, writeConfigLayers(&none, t.TempDir(), nil, nil, false))
	assert.Contains(t, none.String(), "No layers")
}
