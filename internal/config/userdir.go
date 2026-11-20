package config

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"

	"github.com/samber/oops"
)

// UserConfigFileName is the user config file inside UserConfigDir: the one file
// that may set the user-scope-only keys of [llm] and [telemetry] (https://goldziher.github.io/ai-rulez/trust-model/).
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

// CacheDir is a directory below the user cache root: $XDG_CACHE_HOME/ai-rulez/<parts...>
// when XDG_CACHE_HOME is absolute, else ~/.cache/ai-rulez/<parts...>. It is the
// one place that decides where fetched includes, installed skills, skill sources
// and remote documents are cached. There is no fallback to the shared temp
// directory: a cache other users can write to could hold a tree they planted, and
// an unlocked source trusts its cache. Without an absolute XDG_CACHE_HOME or a
// home directory it errors.
func CacheDir(parts ...string) (string, error) { return CacheDirIn(nil, parts...) }

// CacheDirIn is CacheDir with the environment and home directory taken from env
// (nil: the real ones). A relative XDG_CACHE_HOME is ignored, as the XDG base
// directory specification requires.
func CacheDirIn(env ambient.Env, parts ...string) (string, error) {
	if xdg := ambient.Getenv(env, "XDG_CACHE_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(append([]string{xdg, "ai-rulez"}, parts...)...), nil
	}
	home, err := ambient.OrOS(env).UserHomeDir()
	if err != nil || home == "" {
		hint := "Set XDG_CACHE_HOME to an absolute directory or HOME, " +
			"or run from an account with a home directory"
		return "", oops.Hint(hint).Errorf("cannot place the ai-rulez cache: no cache directory")
	}
	return filepath.Join(append([]string{home, ".cache", "ai-rulez"}, parts...)...), nil
}
