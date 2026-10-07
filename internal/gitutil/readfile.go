package gitutil

import (
	"errors"
	"io"
	"os"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// ErrNotRegular is returned by ReadIgnoreFile for a path that resolves to
// something other than a regular file (a device, a FIFO, a directory).
var ErrNotRegular = oops.Errorf("ignore file is not a regular file")

// ErrTooLarge is returned by ReadIgnoreFile for a file over the size limit.
var ErrTooLarge = oops.Errorf("ignore file exceeds the size limit")

// ReadIgnoreFile reads an ignore file (.gitignore, .git/info/exclude) with a
// bounded read. A symlink is followed, but only a target that is a regular file
// no larger than the size limit is read: following a link to /dev/zero would
// otherwise exhaust memory. A missing file yields an error satisfying
// os.IsNotExist; anything else unreadable yields ErrNotRegular or ErrTooLarge.
func ReadIgnoreFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.IsNotExist
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	f, err := os.Open(path) //nolint:gosec // ignore file located through git or the project
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.IsNotExist
	}
	defer f.Close() //nolint:errcheck // read-only handle
	data, err := io.ReadAll(io.LimitReader(f, maxIgnoreFileSize+1))
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read ignore file")
	}
	if int64(len(data)) > maxIgnoreFileSize {
		return nil, ErrTooLarge
	}
	return data, nil
}

// ReadIgnoreFileOrEmpty is ReadIgnoreFile for pattern evaluation: an ignore file
// that is not a bounded regular file reads as empty (with a warning). A missing
// file still returns the os.IsNotExist error.
func ReadIgnoreFileOrEmpty(log logger.Logger, path string) ([]byte, error) {
	data, err := ReadIgnoreFile(path)
	if errors.Is(err, ErrNotRegular) || errors.Is(err, ErrTooLarge) {
		logger.Or(log).Warn("Ignoring an ignore file that is not a regular file within the size limit", "path", path, "reason", err.Error())
		return nil, nil
	}
	return data, err
}
