package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestGeneratedConfigListsEveryBuiltinPreset(t *testing.T) {
	tests := []struct {
		name     string
		generate func(string) string
	}{
		{"yaml", generateConfig},
		{"toml", generateConfigTOML},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			out := tt.generate("demo")

			// Assert
			for _, preset := range config.IndividualPresetNames() {
				assert.Contains(t, out, preset)
			}
			assert.Contains(t, out, "# Built-in presets: amp, antigravity, baz, claude")
		})
	}
}
