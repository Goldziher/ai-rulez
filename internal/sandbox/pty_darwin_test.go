package sandbox

import (
	"bytes"
	"os"
	"unsafe"

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
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		_ = f.Close() //nolint:errcheck // test helper
		return nil, "", err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		_ = f.Close() //nolint:errcheck // test helper
		return nil, "", err
	}
	buf := make([]byte, 128)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		_ = f.Close() //nolint:errcheck // test helper
		return nil, "", errno
	}
	return f, string(buf[:bytes.IndexByte(buf, 0)]), nil
}
