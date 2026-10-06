package config

import (
	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"path/filepath"

	"github.com/samber/oops"
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
		getenv = func(name string) string { return ambient.Getenv(nil, name) }
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "ai-rulez")
	}
	if home == "" {
		h, err := ambient.OS().UserHomeDir()
		if err != nil {
			return ""
		}
		home = h
	}
	return filepath.Join(home, ".config", "ai-rulez")
}

// CacheDir is a directory below the user cache root, ~/.cache/ai-rulez/<parts...>.
// It is the one place that decides where fetched includes, installed skills, skill
// sources and remote documents are cached. There is no fallback to the shared
// temp directory: a cache other users can write to could hold a tree they planted,
// and an unlocked source trusts its cache. Without a home directory it errors.
func CacheDir(parts ...string) (string, error) { return CacheDirIn(nil, parts...) }

// CacheDirIn is CacheDir with the home directory taken from env (nil: the real
// one).
func CacheDirIn(env ambient.Env, parts ...string) (string, error) {
	home, err := ambient.OrOS(env).UserHomeDir()
	if err != nil || home == "" {
		return "", oops.Hint("Set HOME, or run from an account with a home directory").
			Errorf("cannot place the ai-rulez cache: no home directory")
	}
	return filepath.Join(append([]string{home, ".cache", "ai-rulez"}, parts...)...), nil
}
