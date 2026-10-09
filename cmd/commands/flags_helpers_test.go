package commands

import "testing"

// useConfigFile selects the project the way -C/--config does, and restores the
// flag when the test ends.
func useConfigFile(t *testing.T, path string) {
	t.Helper()
	previous := cfgFile
	cfgFile = path
	t.Cleanup(func() { cfgFile = previous })
}
