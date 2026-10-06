package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTelemetryConfig_ValidateResource(t *testing.T) {
	many := map[string]string{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		many[k] = "x"
	}
	tests := []struct {
		name     string
		resource map[string]string
		want     string // substring of the one expected problem; empty means valid
	}{
		{"none", nil, ""},
		{"team label", map[string]string{"team": "platform", "deployment.environment": "prod"}, ""},
		{"upper case key", map[string]string{"Team": "x"}, "telemetry.resource.Team: key must match"},
		{"key with space", map[string]string{"my team": "x"}, "key must match"},
		{"key too long", map[string]string{"a" + strings.Repeat("b", 64): "x"}, "key must match"},
		{"too many entries", many, "at most 8 entries"},
		{"value too long", map[string]string{"team": strings.Repeat("v", 129)}, "at most 128 characters"},
		{"empty value", map[string]string{"team": ""}, "must not be empty"},
		{"control character", map[string]string{"team": "a\nb"}, "control character"},
		{"service key", map[string]string{"service.name": "x"}, "reserved"},
		{"host key", map[string]string{"host.name": "laptop"}, "reserved"},
		{"user key", map[string]string{"user.email": "a@b.c"}, "reserved"},
		{"schema key", map[string]string{"ai_rulez.schema_version": "9"}, "reserved"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &TelemetryConfig{Resource: tt.resource}

			// Act
			problems := cfg.Validate()

			// Assert
			if tt.want == "" {
				assert.Empty(t, problems)
				return
			}
			if assert.Len(t, problems, 1) {
				assert.Contains(t, problems[0], tt.want)
			}
		})
	}
}
