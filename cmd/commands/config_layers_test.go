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
	require.NoError(t, writeConfigLayers(&text, dir, []string{"custom"}, false))
	assert.Contains(t, text.String(), "active (1)")
	assert.Contains(t, text.String(), "config.ai.toml")
	assert.NotContains(t, text.String(), "local")

	var js bytes.Buffer
	require.NoError(t, writeConfigLayers(&js, dir, nil, true))
	assert.Contains(t, js.String(), `"active": false`)

	var none bytes.Buffer
	require.NoError(t, writeConfigLayers(&none, t.TempDir(), nil, false))
	assert.Contains(t, none.String(), "No layers")
}
