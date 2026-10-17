package testutil

import (
	"path/filepath"
	"testing"
)

// IsolateHome points every per-user location (home, cache, config, data and the
// Windows profile directories) at fresh directories under the test's temporary
// directory and returns the home directory. Setting only HOME is not enough:
// os.UserHomeDir reads USERPROFILE on Windows and the cache directory reads
// LocalAppData or XDG_CACHE_HOME, so such a test would read and write the
// developer's real profile.
func IsolateHome(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	home := filepath.Join(root, "home")
	dirs := map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"LOCALAPPDATA":    filepath.Join(root, "localappdata"),
		"APPDATA":         filepath.Join(root, "appdata"),
		"AI_RULEZ_HOME":   filepath.Join(root, "airulez"),
	}
	for name, dir := range dirs {
		tb.Setenv(name, dir)
	}
	return home
}
