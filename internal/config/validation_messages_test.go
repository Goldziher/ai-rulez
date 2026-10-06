package config_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_RemovedPresetAndMissingVersionMessages(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr []string
	}{
		{
			name:    "windsurf was renamed",
			cfg:     config.Config{Version: "4.0", Name: "p", Presets: []config.Preset{{BuiltIn: "windsurf"}}},
			wantErr: []string{"unknown built-in preset", "renamed to devin"},
		},
		{
			name:    "continue-dev was removed",
			cfg:     config.Config{Version: "4.0", Name: "p", Presets: []config.Preset{{BuiltIn: "continue-dev"}}},
			wantErr: []string{"unknown built-in preset", "removed"},
		},
		{
			name:    "missing version says the key is missing",
			cfg:     config.Config{Name: "p", Presets: []config.Preset{{BuiltIn: "claude"}}},
			wantErr: []string{"missing required key: version"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			err := tt.cfg.Validate()

			// Assert
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
