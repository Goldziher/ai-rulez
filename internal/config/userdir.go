package config

import (
	"os"
	"path/filepath"
)

// UserConfigDir is the user-level ai-rulez directory: $XDG_CONFIG_HOME/ai-rulez
// when XDG_CONFIG_HOME is absolute, else <home>/.config/ai-rulez. home "" means
// the current user's home directory; getenv nil means os.Getenv. It returns ""
// when no home directory can be determined. The user config file (config.toml,
// read for [llm] and [telemetry]) and user scope share this one resolution.
func UserConfigDir(getenv func(string) string, home string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "ai-rulez")
	}
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = h
	}
	return filepath.Join(home, ".config", "ai-rulez")
}
