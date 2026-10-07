package sandbox

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// injectProbe is what a confined process does to run a command in the user's
// shell: push a byte into the input queue of its terminal (TIOCSTI). A
// read-only open is enough for the ioctl, so the probe does not ask for writes.
func injectProbe(path string) string {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "denied"
	}
	defer f.Close() //nolint:errcheck // probe
	c := byte('\n')
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(unix.TIOCSTI), uintptr(unsafe.Pointer(&c)))
	if errno != 0 {
		return "denied"
	}
	return "injected"
}

// runWithTerminal runs the confined helper as the leader of a session whose
// controlling terminal is a fresh pseudo-terminal, so only the profile stands
// between it and the terminal.
func runWithTerminal(t *testing.T, sb *Sandbox, mode, arg string) string {
	t.Helper()
	ptmx, name, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer ptmx.Close()                               //nolint:errcheck // test
	go func() { _, _ = io.Copy(io.Discard, ptmx) }() //nolint:errcheck // drain the terminal
	tty, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	require.NoError(t, err)
	defer tty.Close() //nolint:errcheck // test
	exe, err := os.Executable()
	require.NoError(t, err)
	w, err := sb.Wrap(Spec{WriteDirs: []string{t.TempDir()}}, []string{exe, mode, arg})
	require.NoError(t, err)
	cmd := exec.Command(w.Argv[0], w.Argv[1:]...) //nolint:gosec // test helper
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	cmd.Stdin = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestConfinedProcessCannotUseTheTerminal(t *testing.T) {
	sb := realSandbox(t)
	tests := []struct {
		name, mode, path string
	}{
		{"write /dev/tty", "write", "/dev/tty"},
		{"inject through /dev/tty", "inject", "/dev/tty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange, Act
			got := runWithTerminal(t, sb, tt.mode, tt.path)
			// Assert
			assert.Equal(t, "denied", got)
		})
	}
}

func TestProfileDeniesTerminalWritesAndIoctls(t *testing.T) {
	profile := sandboxProfile(Spec{}, 1)
	assert.NotContains(t, profile, `(literal "/dev/tty")`)
	assert.Contains(t, profile, `(deny file-ioctl`)
}
