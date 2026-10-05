package config

import (
	"os"
	"path/filepath"
)

// UserConfigFileName is the user config file inside UserConfigDir: the one file
// that may set the user-scope-only keys of [llm] and [telemetry] (docs/trust-model.md).
const UserConfigFileName = "config.toml"

// UserConfigFile is the path of the user config file, "" when no home directory
// can be determined. Every reader of it (the [llm] and [telemetry] resolvers)
// goes through here, so the location is defined once.
func UserConfigFile(getenv func(string) string) string {
	dir := UserConfigDir(getenv, "")
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, UserConfigFileName)
}

// UserConfigDir is the user-level ai-rulez directory: $XDG_CONFIG_HOME/ai-rulez
// when XDG_CONFIG_HOME is absolute, else <home>/.config/ai-rulez. home "" means
// the current user's home directory; getenv nil means os.Getenv. It returns ""
// when no home directory can be determined. The user config file (see UserConfigFile)
// and user scope share this one resolution.
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
