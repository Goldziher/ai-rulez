package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiSettingValidation(t *testing.T) {
	base := GeminiSetting{Name: "Provider key", Description: "Provider credential.", EnvVar: "OPENAI_API_KEY", Sensitive: true}
	for _, test := range []struct {
		name     string
		settings []GeminiSetting
		invalid  bool
	}{
		{name: "allowed", settings: []GeminiSetting{base}},
		{name: "empty", settings: []GeminiSetting{}},
		{name: "missing variable", settings: []GeminiSetting{{Name: "Provider key", Description: "Credential."}}, invalid: true},
		{name: "duplicate variable", settings: []GeminiSetting{base, {Name: "Another key", Description: "Credential.", EnvVar: "OPENAI_API_KEY"}}, invalid: true},
		{name: "duplicate name", settings: []GeminiSetting{base, {Name: "Provider key", Description: "Credential.", EnvVar: "ANTHROPIC_API_KEY"}}, invalid: true},
		{name: "duplicate", settings: []GeminiSetting{base, base}, invalid: true},
		{name: "missing name", settings: []GeminiSetting{{Description: "Credential.", EnvVar: "OPENAI_API_KEY"}}, invalid: true},
		{name: "invalid variable", settings: []GeminiSetting{{Name: "Provider key", Description: "Credential.", EnvVar: "BAD-KEY"}}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &PluginAuthoring{Gemini: &GeminiExtras{Settings: test.settings}}
			err := validateGeminiSettings(p)
			if test.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
