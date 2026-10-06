package catalogsite

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

const (
	fileMode = 0o644
	dirMode  = 0o755
	// maxMarkerBytes bounds the marker read; a real one lists a few thousand paths.
	maxMarkerBytes = 8 << 20
	markerHeader   = "# ai-rulez catalog output. Files below were written by `ai-rulez catalog --html`;\n" +
		"# `--clean` removes only these. Delete this file to stop ai-rulez touching the directory.\n"
)

// WriteResult says what Write did.
type WriteResult struct {
	Written int
	Removed []string
}

// Write writes site into dir. The directory must be new, empty or marked with
// MarkerFile; anything else is refused so `--html .` can never overwrite a
// project. With clean, files a previous run wrote (listed in the marker) that
// the new site no longer has are removed; nothing else is ever removed.
//
// All access goes through an os.Root, so a symlink inside dir cannot lead a
// write or a removal outside it.
func Write(dir string, site *Site, clean bool) (*WriteResult, error) {
	if dir == "" {
		return nil, oops.Errorf("no output directory")
	}
	previous, err := prepare(dir, clean)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "open output directory")
	}
	defer root.Close() //nolint:errcheck // nothing buffered

	paths := site.Paths()
	for _, p := range paths {
		if err := writeOne(root, p, site.Files[p]); err != nil {
			return nil, err
		}
	}
	res := &WriteResult{Written: len(paths)}
	keep := map[string]bool{}
	for _, p := range paths {
		keep[p] = true
	}
	var listed []string
	listed = append(listed, paths...)
	for _, old := range previous {
		if keep[old] {
			continue
		}
		if clean {
			if removeOne(root, old) {
				res.Removed = append(res.Removed, old)
			}
		} else if _, statErr := root.Lstat(old); statErr == nil {
			listed = append(listed, old) // stays listed so a later --clean can remove it
		}
	}
	sort.Strings(res.Removed)
	sort.Strings(listed)
	marker := markerHeader + strings.Join(listed, "\n") + "\n"
	if err := writeOne(root, MarkerFile, []byte(marker)); err != nil {
		return nil, err
	}
	return res, nil
}

// prepare creates dir if needed, enforces the marker rule and returns the files
// the previous run listed.
func prepare(dir string, clean bool) ([]string, error) {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if mkErr := os.MkdirAll(dir, dirMode); mkErr != nil {
			return nil, oops.With("dir", dir).Wrapf(mkErr, "create output directory")
		}
		return nil, nil
	case err != nil:
		return nil, oops.With("dir", dir).Wrapf(err, "inspect output directory")
	case !info.IsDir() && info.Mode()&fs.ModeSymlink == 0:
		return nil, oops.With("dir", dir).Errorf("%s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "read output directory")
	}
	marker := filepath.Join(dir, MarkerFile)
	mInfo, mErr := os.Lstat(marker)
	hasMarker := mErr == nil && mInfo.Mode().IsRegular()
	if !hasMarker {
		if len(entries) > 0 {
			return nil, oops.With("dir", dir).Hint("write into a new or empty directory, or one a previous `catalog --html` run filled").
				Errorf("%s is not empty and has no %s marker: refusing to write%s", dir, MarkerFile, cleanNote(clean))
		}
		return nil, nil
	}
	data, err := os.ReadFile(marker) //nolint:gosec // the marker of the directory the user named
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "read %s", MarkerFile)
	}
	if len(data) > maxMarkerBytes {
		return nil, oops.With("dir", dir).Errorf("%s is too large", MarkerFile)
	}
	return parseMarker(string(data)), nil
}

func cleanNote(clean bool) string {
	if clean {
		return " and cleaning"
	}
	return ""
}

// parseMarker returns the listed paths, dropping anything that is not a plain
// relative path below the directory: the marker is data on disk, not trusted.
func parseMarker(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !safeRel(line) || line == MarkerFile {
			continue
		}
		out = append(out, line)
	}
	return out
}

func safeRel(p string) bool {
	if p == "" || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return false
		}
	}
	return fs.ValidPath(p)
}

func writeOne(root *os.Root, name string, data []byte) error {
	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, dirMode); err != nil {
			return oops.With("path", name).Wrapf(err, "create directory")
		}
	}
	tmp := name + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return oops.With("path", name).Wrapf(err, "create file")
	}
	_, writeErr := f.Write(data)
	chmodErr := f.Chmod(fileMode)
	closeErr := f.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		_ = root.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return oops.With("path", name).Wrapf(err, "write file")
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return oops.With("path", name).Wrapf(err, "replace file")
	}
	return nil
}

// removeOne removes a listed file and any directories it leaves empty; it
// reports whether the file existed.
func removeOne(root *os.Root, name string) bool {
	if err := root.Remove(name); err != nil {
		return false
	}
	for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if root.Remove(dir) != nil { // not empty: stop
			break
		}
	}
	return true
}
