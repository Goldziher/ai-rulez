package telemetry

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestLoadRepo_RootLayoutReadsOnlyTheAIRulezFiles(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantNil     bool
		wantEnabled bool
		wantService string
	}{
		{
			name:    "a stray config.toml at the root is ignored",
			files:   map[string]string{"ai-rulez.toml": "version = \"5.0\"\n", "config.toml": "[telemetry]\nenabled = true\n"},
			wantNil: true,
		},
		{
			name:    "a stray config.json at the root is ignored",
			files:   map[string]string{"ai-rulez.toml": "version = \"5.0\"\n", "config.json": "{\"telemetry\": {\"enabled\": true}}"},
			wantNil: true,
		},
		{
			name: "an opt-out in ai-rulez.toml is honored over a stray config.toml",
			files: map[string]string{
				"ai-rulez.toml": "version = \"5.0\"\n[telemetry]\nenabled = false\n",
				"config.toml":   "[telemetry]\nenabled = true\n",
			},
			wantEnabled: false,
		},
		{
			name: "the local overlay wins per key",
			files: map[string]string{
				"ai-rulez.toml":       "version = \"5.0\"\n[telemetry]\nenabled = true\nservice_name = \"shared\"\n",
				"ai-rulez.local.toml": "[telemetry]\nenabled = false\nservice_name = \"mine\"\n",
			},
			wantEnabled: false,
			wantService: "mine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, body := range tt.files {
				write(t, filepath.Join(root, rel), body)
			}

			// Act
			cfg, err := LoadRepo(root, config.RootConfigDirName)

			// Assert
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, cfg)
				return
			}
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.Enabled)
			assert.Equal(t, tt.wantEnabled, *cfg.Enabled)
			if tt.wantService != "" {
				assert.Equal(t, tt.wantService, cfg.ServiceName)
			}
		})
	}
}
