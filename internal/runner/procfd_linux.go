package runner

import (
	"errors"
	"os"
	"strconv"
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

// holdsMarker reports whether pid has a descriptor on the marker pipe
// identified by id (/proc/<pid>/fd/* reads "pipe:[<inode>]").
func holdsMarker(pid int, id uint64) bool {
	dir := "/proc/" + strconv.Itoa(pid) + "/fd"
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	want := "pipe:[" + strconv.FormatUint(id, 10) + "]"
	for _, e := range ents {
		if l, err := os.Readlink(dir + "/" + e.Name()); err == nil && l == want {
			return true
		}
	}
	return false
}
