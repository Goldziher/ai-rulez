package config

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
)

// rooted reports whether p names a location from a root rather than relative to
// a directory: an absolute path, a path with a volume (C:x, \\host\share), or one
// that starts with a separator. filepath.IsAbs alone misses the last two on
// Windows, where "/virtual/proj" (the root of a workspace in memory) and
// "/etc/x" (a path a committed config must not name) are "relative".
func rooted(p string) bool {
	return filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`)
}

// resourceMode is the permission a skill resource is recorded with. Windows has
// no permission bits beyond read-only: Go reports 0666 for every writable file,
// where a git snapshot of the same tree reports 0644, so a resource read from
// the disk there is 0644 too and a plan does not depend on the workspace.
func resourceMode(m fs.FileMode) fs.FileMode {
	if runtime.GOOS == "windows" {
		return 0o644
	}
	return m.Perm()
}
