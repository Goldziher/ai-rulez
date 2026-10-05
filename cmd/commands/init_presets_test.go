package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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
			assert.Contains(t, out, "# Built-in presets: "+strings.Join(config.IndividualPresetNames(), ", ")+"\n")
		})
	}
}
