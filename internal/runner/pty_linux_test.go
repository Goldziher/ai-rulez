package runner

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// openPTY opens a pseudo-terminal pair: the controller and the device path of
// the other end.
func openPTY() (*os.File, string, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	f := os.NewFile(uintptr(fd), "/dev/ptmx")
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		_ = f.Close() //nolint:errcheck // test helper
		return nil, "", err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		_ = f.Close() //nolint:errcheck // test helper
		return nil, "", err
	}
	return f, "/dev/pts/" + strconv.Itoa(n), nil
}
