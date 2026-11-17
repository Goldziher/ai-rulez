package worktreeprovision

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// copyOne carries one regular file without replacing anything that already
// exists: the content is written to a temporary file in the destination
// directory and linked into place, so a file an edit of the destination
// created between the plan and the copy is never overwritten. The link fails
// when the destination appeared meanwhile; the run reports the conflict.
func copyOne(w *worktrees, f copyFile) error {
	data, err := readCopyFile(w.src, f.rel)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	root, err := os.OpenRoot(w.dst)
	if err != nil {
		return fmt.Errorf("open destination root: %w", err)
	}
	defer root.Close() //nolint:errcheck // read-only directory handle
	rel := filepath.FromSlash(relTo(w.dst, f.dst))
	if err := ensureCopyDir(root, filepath.Dir(rel), true); err != nil {
		return err
	}
	tmpName := filepath.Join(filepath.Dir(rel), ".ai-rulez-provision-"+rand.Text())
	tmp, err := root.OpenFile(tmpName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create the temporary file: %w", err)
	}
	defer func() { _ = root.Remove(tmpName) }() //nolint:errcheck // best-effort temp cleanup
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return fmt.Errorf("write the temporary file: %w", err)
	}
	if err := root.Link(tmpName, rel); err != nil {
		return fmt.Errorf("link %s into place without replacing it: %w", f.rel, err)
	}
	return nil
}

func checkCopyDir(base, path string) error {
	root, err := os.OpenRoot(base)
	if err != nil {
		return fmt.Errorf("open worktree root: %w", err)
	}
	defer root.Close() //nolint:errcheck // read-only directory handle
	err = ensureCopyDir(root, filepath.FromSlash(relTo(base, path)), false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func ensureCopyDir(root *os.Root, parent string, create bool) error {
	if parent == "." {
		return nil
	}
	dir := "."
	for part := range strings.SplitSeq(parent, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		info, err := root.Lstat(dir)
		if create && errors.Is(err, fs.ErrNotExist) {
			if err := root.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("create directory %s: %w", dir, err)
			}
			info, err = root.Lstat(dir)
		}
		if err != nil {
			return fmt.Errorf("read directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is a symlink or is not a directory", dir)
		}
	}
	return nil
}

func readCopyFile(base, rel string) ([]byte, error) {
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, fmt.Errorf("open worktree root: %w", err)
	}
	defer root.Close() //nolint:errcheck // read-only directory handle
	if err := ensureCopyDir(root, filepath.Dir(filepath.FromSlash(rel)), false); err != nil {
		return nil, err
	}
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is a symlink or is not a regular file", rel)
	}
	file, err := root.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", rel, err)
	}
	defer file.Close() //nolint:errcheck // read-only file handle
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened %s: %w", rel, err)
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("%s changed while opening it", rel)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	return data, nil
}
