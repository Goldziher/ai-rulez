package runner

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// markerID identifies the run's marker pipe by its inode (both ends share it).
func markerID(r *os.File) (uint64, error) {
	fi, err := r.Stat()
	if err != nil {
		return 0, err //nolint:wrapcheck // the caller runs without the marker
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("no inode for the marker pipe")
	}
	return st.Ino, nil
}

// holdsMarker reports whether pid has a descriptor on the write end of the
// marker pipe identified by id. /proc/<pid>/fd/* reads "pipe:[<inode>]" for
// both ends, so the access mode in /proc/<pid>/fdinfo tells them apart. The
// read end does not count: a process this one forks holds a copy of it until
// its exec closes it, and that process is not the run's.
func holdsMarker(pid int, id uint64) bool {
	base := "/proc/" + strconv.Itoa(pid)
	ents, err := os.ReadDir(base + "/fd")
	if err != nil {
		return false
	}
	want := "pipe:[" + strconv.FormatUint(id, 10) + "]"
	for _, e := range ents {
		if l, err := os.Readlink(base + "/fd/" + e.Name()); err == nil && l == want && writeOnly(base+"/fdinfo/"+e.Name()) {
			return true
		}
	}
	return false
}

// writeOnly reports whether the fdinfo file at path describes a descriptor
// opened for writing only.
func writeOnly(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "flags:"); ok {
			flags, err := strconv.ParseUint(strings.TrimSpace(v), 8, 64)
			return err == nil && flags&syscall.O_ACCMODE == syscall.O_WRONLY
		}
	}
	return false
}
