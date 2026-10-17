package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProjectRoot(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		stopAt string // relative to the temp root; "" means no bound
		found  bool
	}{
		{name: "config directory", file: ".ai-rulez/config.toml", found: true},
		{name: "agents layout", file: ".agents/ai-rulez.toml", found: true},
		{name: "config convention", file: ".config/ai-rulez/config.toml", found: true},
		{name: "root file", file: "ai-rulez.toml", found: true},
		{name: "stop below the config", file: "ai-rulez.toml", stopAt: "a"},
		{name: "stop at the config", file: "ai-rulez.toml", stopAt: ".", found: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeConfigFile(t, filepath.Join(root, filepath.FromSlash(tt.file)), "version = \"5.0\"\nname = \"x\"\n")
			start := filepath.Join(root, "a", "b")
			require.NoError(t, os.MkdirAll(start, 0o755))
			stop := ""
			if tt.stopAt != "" {
				stop = filepath.Join(root, tt.stopAt)
			}

			// Act
			got, err := FindProjectRoot(start, stop)

			// Assert
			if !tt.found {
				require.ErrorIs(t, err, ErrNoConfig)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, root, got)
		})
	}
}
