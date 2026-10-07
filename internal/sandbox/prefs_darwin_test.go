package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfinedProcessCannotWritePreferencesThroughCfprefsd(t *testing.T) {
	sb := realSandbox(t)
	if _, err := os.Stat("/usr/bin/defaults"); err != nil {
		t.Skip("no defaults(1)")
	}
	// Arrange: a bare domain makes defaults ask cfprefsd, outside the sandbox,
	// to write ~/Library/Preferences/<domain>.plist on its behalf.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	domain := "com.airulez.sandboxtest." + strconv.Itoa(os.Getpid())
	plist := filepath.Join(home, "Library", "Preferences", domain+".plist")
	t.Cleanup(func() {
		_ = exec.Command("/usr/bin/defaults", "delete", domain).Run() //nolint:errcheck // absent is fine
		_ = os.Remove(plist)                                          //nolint:errcheck // absent is fine
	})
	w, err := sb.Wrap(Spec{WriteDirs: []string{t.TempDir()}}, []string{"/usr/bin/defaults", "write", domain, "k", "escaped"})
	require.NoError(t, err)
	// Act
	out, runErr := exec.Command(w.Argv[0], w.Argv[1:]...).CombinedOutput() //nolint:gosec // test argv
	// Assert
	read, _ := exec.Command("/usr/bin/defaults", "read", domain, "k").Output() //nolint:errcheck // absent is the pass
	assert.NotEqual(t, "escaped\n", string(read), "cfprefsd wrote the preference for the confined process (%v: %s)", runErr, out)
	assert.NoFileExists(t, plist)
}

func TestProfileDeniesCfprefsd(t *testing.T) {
	assert.Contains(t, sandboxProfile(Spec{}, 0), `(global-name-prefix "com.apple.cfprefsd.")`)
}
