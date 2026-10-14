package config

import (
	"regexp"
	"strings"

	"github.com/samber/oops"
)

var geminiEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateGeminiSettings(p *PluginAuthoring) error {
	if p.Gemini == nil {
		return nil
	}
	names, variables := map[string]bool{}, map[string]bool{}
	for _, setting := range p.Gemini.Settings {
		if strings.TrimSpace(setting.Name) == "" || strings.TrimSpace(setting.Description) == "" {
			return oops.With("field", "plugin.gemini.settings").Errorf("Gemini setting requires name and description")
		}
		if !geminiEnvName.MatchString(setting.EnvVar) {
			return oops.With("field", "plugin.gemini.settings.env_var").Errorf("invalid Gemini setting environment variable %q", setting.EnvVar)
		}
		if names[setting.Name] || variables[setting.EnvVar] {
			return oops.With("field", "plugin.gemini.settings").Errorf("duplicate Gemini setting name or environment variable %q", setting.EnvVar)
		}
		names[setting.Name], variables[setting.EnvVar] = true, true
	}
	return nil
}
