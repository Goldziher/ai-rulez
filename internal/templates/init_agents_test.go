package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestInitAgentsConfigTOML(t *testing.T) {
	tests := []struct {
		name        string
		inAgentsDir bool
		wantSource  string
	}{
		{"in .agents the location selects the source", true, ""},
		{"in another config file the key selects it", false, config.SourceAgents},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			doc := InitAgentsConfigTOML("demo", []string{"claude", "cursor"}, tt.inAgentsDir)
			cfg, err := config.DecodeTOMLConfig([]byte(doc), "ai-rulez.toml")

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantSource, cfg.Source)
			assert.Equal(t, "demo", cfg.Name)
			require.Len(t, cfg.Presets, 2)
			assert.Equal(t, "cursor", cfg.Presets[1].BuiltIn)
		})
	}
}

func TestInitAgentsMD(t *testing.T) {
	assert.Contains(t, InitAgentsMD("demo"), "# demo\n")
	assert.Contains(t, InitAgentsMD("  "), "# project\n")
}
