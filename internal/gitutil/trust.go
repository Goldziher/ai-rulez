package gitutil

import (
	"context"
	"os"
	"path/filepath"
)

// UntrustedLocalFileContext reports why a gitignored, machine-local state file cannot be
// believed, or "" when it can. A file a repository commits arrives with whatever
// content its author chose, so it is untrusted when git tracks it, when it is not
// owned by the current user, or when anyone but the owner may write to it. A file
// that does not exist is trusted (there is nothing to believe). A symlink is
// untrusted: the state file is always a regular file ai-rulez wrote itself.
func (g Git) UntrustedLocalFileContext(ctx context.Context, path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if !info.Mode().IsRegular() {
		return "it is not a regular file"
	}
	if writableByOthers(info) {
		return "it is writable by group or others"
	}
	if !ownedByCurrentUser(info) {
		return "it is not owned by the current user"
	}
	if g.IsTrackedContext(ctx, path) {
		return "git tracks it"
	}
	return ""
}

// IsTrackedContext reports whether git tracks the file at path. Outside a repository it
// is false; when git fails inside one it is true, so the caller fails closed.
func (g Git) IsTrackedContext(ctx context.Context, path string) bool {
	dir, name := filepath.Dir(path), filepath.Base(path)
	tracked, err := g.TrackedAmongContext(ctx, dir, []string{name})
	if err != nil {
		return true
	}
	return tracked[name]
}

// UntrustedLocalFile is UntrustedLocalFileContext without a caller's context.
func (g Git) UntrustedLocalFile(path string) string {
	return g.UntrustedLocalFileContext(context.Background(), path)
}

// IsTracked is IsTrackedContext without a caller's context.
func (g Git) IsTracked(path string) bool {
	return g.IsTrackedContext(context.Background(), path)
}
