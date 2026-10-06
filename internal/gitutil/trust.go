package gitutil

import (
	"os"
	"path/filepath"
)

// UntrustedLocalFile reports why a gitignored, machine-local state file cannot be
// believed, or "" when it can. A file a repository commits arrives with whatever
// content its author chose, so it is untrusted when git tracks it, when it is not
// owned by the current user, or when anyone but the owner may write to it. A file
// that does not exist is trusted (there is nothing to believe). A symlink is
// untrusted: the state file is always a regular file ai-rulez wrote itself.
func UntrustedLocalFile(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if !info.Mode().IsRegular() {
		return "it is not a regular file"
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "it is writable by group or others"
	}
	if !ownedByCurrentUser(info) {
		return "it is not owned by the current user"
	}
	if IsTracked(path) {
		return "git tracks it"
	}
	return ""
}

// IsTracked reports whether git tracks the file at path. Outside a repository it
// is false; when git fails inside one it is true, so the caller fails closed.
func IsTracked(path string) bool {
	dir, name := filepath.Dir(path), filepath.Base(path)
	tracked, err := TrackedAmong(dir, []string{name})
	if err != nil {
		return true
	}
	return tracked[name]
}
