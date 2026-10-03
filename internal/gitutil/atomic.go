package gitutil

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

// WriteFileAtomic writes data to targetPath with perm via an exclusively created
// temp file in the same directory, then renames it over the target, so a reader
// never sees a half-written file.
func WriteFileAtomic(targetPath string, data []byte, perm os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+"-*.tmp")
	if err != nil {
		return oops.With("path", targetPath).Wrapf(err, "create temporary config file")
	}
	tmpPath := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath) //nolint:errcheck // temp file cleanup is best-effort
		}
	}()
	if err = f.Chmod(perm); err != nil {
		_ = f.Close() //nolint:errcheck // already failing
		return oops.With("path", tmpPath).Wrapf(err, "set config file permissions")
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close() //nolint:errcheck // already failing
		return oops.With("path", tmpPath).Wrapf(err, "write temporary config file")
	}
	if err = f.Sync(); err != nil {
		_ = f.Close() //nolint:errcheck // already failing
		return oops.With("path", tmpPath).Wrapf(err, "sync temporary config file")
	}
	if err = f.Close(); err != nil {
		return oops.With("path", tmpPath).Wrapf(err, "close temporary config file")
	}
	if err = os.Rename(tmpPath, targetPath); err != nil {
		return oops.With("src", tmpPath).With("dst", targetPath).Wrapf(err, "rename config file")
	}
	return nil
}
