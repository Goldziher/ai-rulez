package llm

import (
	"os"
	"testing"
)

// TestMain points the home directory at a scratch directory so no test writes a
// cache or a cache secret into the real user directories.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "ai-rulez-llm-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)        //nolint:errcheck,gosec // test setup
	os.Setenv("USERPROFILE", home) //nolint:errcheck,gosec // test setup
	os.Unsetenv("XDG_CONFIG_HOME") //nolint:errcheck,gosec // test setup
	code := m.Run()
	os.RemoveAll(home) //nolint:errcheck,gosec // test cleanup
	os.Exit(code)
}
