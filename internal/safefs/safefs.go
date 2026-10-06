// Package safefs holds the file primitives for machine-local files that a
// repository can plant symlinks next to (usage log, telemetry spool, salt).
// None of them follow a symlink at or below the project's config directory.
package safefs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// configDirName is the directory whose parent is treated as the trusted project
// root: everything below it is checked for symlinks.
const configDirName = ".ai-rulez"

// maxReadBytes bounds ReadRegular.
const maxReadBytes = 1 << 20

// splitRoot returns the trusted base directory for path and the slash-free
// relative path below it. When path sits under a ".ai-rulez" component the base
// is that component's parent (the project root, which may legitimately be
// reached through symlinks); otherwise it is path's directory.
func splitRoot(path string) (base, rel string) {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	for i := len(parts) - 2; i >= 0; i-- { // the file itself is never the config dir
		if parts[i] == configDirName {
			base = strings.Join(parts[:i], string(filepath.Separator))
			if base == "" {
				base = "."
				if filepath.IsAbs(clean) {
					base = string(filepath.Separator)
				}
			}
			return base, filepath.Join(parts[i:]...)
		}
	}
	return filepath.Dir(clean), filepath.Base(clean)
}

func refuse(path, why string) error {
	return oops.With("path", path).Errorf("refusing to use %s: %s", path, why)
}

// openBase opens the trusted base of path as a root, creating missing
// directories. Below the project root every directory is created one component
// at a time and Lstat-checked, so no ancestor symlink is followed. A path outside
// any config directory keeps the older rule: its immediate directory must not be
// a symlink.
func openBase(path string) (root *os.Root, rel string, err error) {
	base, rel := splitRoot(path)
	if rel == filepath.Base(path) {
		if info, statErr := os.Lstat(base); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, "", refuse(path, base+" is a symlink")
		}
	}
	if err := os.MkdirAll(base, 0o750); err != nil {
		return nil, "", oops.With("path", path).Wrapf(err, "create directory")
	}
	root, err = os.OpenRoot(base)
	if err != nil {
		return nil, "", oops.With("path", path).Wrapf(err, "open directory")
	}
	fail := func(err error) (*os.Root, string, error) {
		_ = root.Close() //nolint:errcheck // the original error is the one to report
		return nil, "", err
	}
	cur := ""
	dirRel := filepath.Dir(rel)
	if dirRel == "." {
		return root, rel, nil
	}
	for _, part := range strings.Split(dirRel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		err := checkDir(root, cur, path)
		if errors.Is(err, os.ErrNotExist) {
			if mkErr := root.Mkdir(cur, 0o750); mkErr != nil && !errors.Is(mkErr, os.ErrExist) {
				return fail(oops.With("path", path).Wrapf(mkErr, "create directory"))
			}
			err = checkDir(root, cur, path)
		}
		if err != nil {
			return fail(err)
		}
	}
	return root, rel, nil
}

func checkDir(root *os.Root, rel, path string) error {
	info, err := root.Lstat(rel)
	if err != nil {
		return err //nolint:wrapcheck // the caller distinguishes not-exist
	}
	if !info.IsDir() {
		return refuse(path, rel+" is a symlink or not a directory")
	}
	return nil
}

// OpenAppend opens path for appending, creating it (and its directories) when
// missing, without following a symlink at the file or at any directory below the
// project root. An existing file must be regular and is checked against a fresh
// Lstat after the open; a missing one is created with O_EXCL.
func OpenAppend(path string) (*os.File, error) {
	root, rel, err := openBase(path)
	if err != nil {
		return nil, err
	}
	defer root.Close() //nolint:errcheck // read-only handle on the directory
	for attempt := 0; attempt < 2; attempt++ {
		info, err := root.Lstat(rel)
		switch {
		case err == nil:
			if !info.Mode().IsRegular() {
				return nil, refuse(path, "it is a symlink or not a regular file")
			}
			file, openErr := root.OpenFile(rel, os.O_APPEND|os.O_WRONLY, 0o600)
			if openErr != nil {
				return nil, oops.With("path", path).Wrapf(openErr, "open file")
			}
			if opened, statErr := file.Stat(); statErr != nil || !os.SameFile(info, opened) {
				_ = file.Close() //nolint:errcheck // the file was swapped under us
				return nil, refuse(path, "it changed while it was opened")
			}
			return file, nil
		case errors.Is(err, os.ErrNotExist):
			file, openErr := root.OpenFile(rel, os.O_APPEND|os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if openErr == nil {
				return file, nil
			}
			if !errors.Is(openErr, os.ErrExist) {
				return nil, oops.With("path", path).Wrapf(openErr, "create file")
			}
		default:
			return nil, oops.With("path", path).Wrapf(err, "open file")
		}
	}
	return nil, refuse(path, "it keeps changing")
}

// AppendLine appends line with a single write, through OpenAppend.
func AppendLine(path string, line []byte) error {
	file, err := OpenAppend(path)
	if err != nil {
		return err
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close() //nolint:errcheck // the write error is the one to report
		return oops.With("path", path).Wrapf(err, "write file")
	}
	return oops.Wrapf(file.Close(), "close file")
}

// WriteFileAtomic writes data (mode 0600) to a temp file beside path and renames
// it over path. A symlink at path is replaced, never written through.
func WriteFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return oops.With("path", path).Wrapf(err, "create temp file")
	}
	name := tmp.Name()
	if err := os.Chmod(name, 0o600); err != nil {
		_ = tmp.Close()     //nolint:errcheck // the chmod error is the one to report
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return oops.Wrapf(err, "chmod temp file")
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return oops.With("path", path).Wrapf(err, "write temp file")
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return oops.With("path", path).Wrapf(err, "replace file")
	}
	return nil
}

// ReadRegular reads a small regular file, refusing a symlink or anything else.
// A loose mode is tightened to 0600 on the opened handle, never on a link target.
func ReadRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	if !info.Mode().IsRegular() {
		return nil, refuse(path, "it is a symlink or not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY, 0) //nolint:gosec // Lstat-checked machine-local file
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	defer file.Close() //nolint:errcheck // read-only
	if opened, statErr := file.Stat(); statErr != nil || !os.SameFile(info, opened) {
		return nil, refuse(path, "it changed while it was opened")
	}
	if info.Mode().Perm()&0o077 != 0 {
		_ = file.Chmod(0o600) //nolint:errcheck // best effort: the content is still usable
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReadBytes))
	return data, oops.With("path", path).Wrapf(err, "read file")
}

// OpenRegular opens a regular file for streaming, refusing a symlink or anything
// else, for files too large for ReadRegular. The caller closes it.
func OpenRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	if !info.Mode().IsRegular() {
		return nil, refuse(path, "it is a symlink or not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY, 0) //nolint:gosec // Lstat-checked machine-local file
	if err != nil {
		return nil, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	if opened, statErr := file.Stat(); statErr != nil || !os.SameFile(info, opened) {
		_ = file.Close() //nolint:errcheck // read-only
		return nil, refuse(path, "it changed while it was opened")
	}
	return file, nil
}

// EnsureParent creates the directories above path with the same symlink checks
// as OpenAppend, so a caller that writes beside path (a temp file) does not
// create files through a planted link.
func EnsureParent(path string) error {
	root, _, err := openBase(path)
	if err != nil {
		return err
	}
	return oops.Wrapf(root.Close(), "close directory")
}
