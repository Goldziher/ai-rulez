package runner

import (
	"bytes"
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
)

// processEnv returns the environment pid was started with (KERN_PROCARGS2).
// The kernel answers only for the caller's own processes.
func processEnv(pid int) ([]string, error) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller skips the process
	}
	return parseProcArgs(buf)
}

// parseProcArgs reads a KERN_PROCARGS2 buffer: argc (int32), the executable
// path, NUL padding, argc arguments, then the environment, each NUL-terminated,
// up to an empty string.
func parseProcArgs(buf []byte) ([]string, error) {
	if len(buf) < 4 {
		return nil, errors.New("procargs: short buffer")
	}
	argc := int(binary.LittleEndian.Uint32(buf[:4]))
	rest := buf[4:]
	next := func() (string, bool) {
		i := bytes.IndexByte(rest, 0)
		if i < 0 {
			return "", false
		}
		s := string(rest[:i])
		rest = rest[i+1:]
		return s, true
	}
	if _, ok := next(); !ok { // the executable path
		return nil, errors.New("procargs: no executable path")
	}
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	for range argc {
		if _, ok := next(); !ok {
			return nil, errors.New("procargs: truncated arguments")
		}
	}
	var env []string
	for {
		s, ok := next()
		if !ok || s == "" {
			return env, nil
		}
		env = append(env, s)
	}
}
