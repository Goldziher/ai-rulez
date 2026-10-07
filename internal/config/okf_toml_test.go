package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeConfigTOML_OKFTable(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte(`
name = "x"
presets = ["claude", "okf"]
[okf]
dir = "kb"
include = ["rules", "skills"]
spec = "0.2"
`), "config.toml")
	require.NoError(t, err)
	require.NotNil(t, cfg.OKF)
	assert.Equal(t, "kb", cfg.OKFDir())
	assert.Equal(t, []string{"rules", "skills"}, cfg.OKF.Include)
	assert.Equal(t, "0.2", cfg.OKF.Spec)

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	again, err := decodeConfigTOML(out, "config.toml")
	require.NoError(t, err)
	assert.Equal(t, cfg.OKF, again.OKF, "the writer keeps the [okf] table")
}
