package okf

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// File is one file of a bundle to write.
type File struct {
	// Path is the slash-separated bundle-relative path.
	Path string
	Data []byte
	// Mode is the permission bits; zero means 0o644.
	Mode fs.FileMode
}

func (f File) mode() fs.FileMode {
	if f.Mode == 0 {
		return 0o644
	}
	return f.Mode.Perm()
}

// ValidatePath rejects a path that could escape the bundle directory.
func ValidatePath(p string) error {
	if p == "" || path.IsAbs(p) || strings.Contains(p, "\\") || strings.ContainsRune(p, 0) {
		return oops.Errorf("unsafe bundle path %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return oops.Errorf("unsafe bundle path %q", p)
		}
	}
	return nil
}

// Drift is the difference between a set of files and a directory.
type Drift struct {
	Missing []string `json:"missing,omitempty"`
	Changed []string `json:"changed,omitempty"`
	// Extra are files in the directory that the set does not contain.
	Extra []string `json:"extra,omitempty"`
}

// Empty reports whether the directory matches the set exactly.
func (d Drift) Empty() bool { return len(d.Missing)+len(d.Changed)+len(d.Extra) == 0 }

// Compare reports how dir differs from files. A missing dir makes every file missing.
func Compare(dir string, files []File) (Drift, error) {
	var d Drift
	want := map[string]bool{}
	for _, f := range files {
		if err := ValidatePath(f.Path); err != nil {
			return d, err
		}
		want[f.Path] = true
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			d.Missing = append(d.Missing, f.Path)
		case err != nil:
			return d, oops.Wrapf(err, "read %s", f.Path)
		case !bytes.Equal(got, f.Data):
			d.Changed = append(d.Changed, f.Path)
		}
	}
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) && p == dir {
				return fs.SkipAll
			}
			return walkErr
		}
		if e.IsDir() {
			if e.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		if rel = filepath.ToSlash(rel); !want[rel] {
			d.Extra = append(d.Extra, rel)
		}
		return nil
	})
	if err != nil {
		return d, oops.Wrapf(err, "scan %s", dir)
	}
	sort.Strings(d.Missing)
	sort.Strings(d.Changed)
	sort.Strings(d.Extra)
	return d, nil
}

// LooksLikeBundle reports whether dir holds a root index.md, which is how an
// existing export is recognized before stale files are removed from it.
func LooksLikeBundle(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, IndexFile))
	return err == nil && bytes.Contains(data, []byte("okf_version"))
}

// WriteFiles writes files into dir, creating it. Writes cannot escape dir, even
// through symlinks. With prune, files of a previous export that are no longer in
// the set are removed; that is refused unless dir is empty or already a bundle.
func WriteFiles(dir string, files []File, prune bool) error {
	for _, f := range files {
		if err := ValidatePath(f.Path); err != nil {
			return err
		}
	}
	if prune {
		if err := checkPrunable(dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: shareable bundle directory
		return oops.Wrapf(err, "create %s", dir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return oops.Wrapf(err, "open %s", dir)
	}
	defer root.Close() //nolint:errcheck // read-only handle
	for _, f := range files {
		if err := writeOne(root, f); err != nil {
			return err
		}
	}
	if !prune {
		return nil
	}
	return removeExtras(root, dir, files)
}

func checkPrunable(dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 && !LooksLikeBundle(dir) {
		return oops.Errorf("%s is not empty and is not an OKF bundle (no index.md with okf_version); refusing to remove files", dir)
	}
	return nil
}

func removeExtras(root *os.Root, dir string, files []File) error {
	drift, err := Compare(dir, files)
	if err != nil {
		return err
	}
	for _, extra := range drift.Extra {
		if err := root.Remove(extra); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return oops.Wrapf(err, "remove %s", extra)
		}
	}
	return pruneEmptyDirs(dir)
}

func writeOne(root *os.Root, f File) error {
	if sub := path.Dir(f.Path); sub != "." {
		if err := root.MkdirAll(sub, 0o755); err != nil { //nolint:gosec // G301: shareable bundle directory
			return oops.Wrapf(err, "create %s", sub)
		}
	}
	if err := root.WriteFile(f.Path, f.Data, f.mode()); err != nil {
		return oops.Wrapf(err, "write %s", f.Path)
	}
	if err := root.Chmod(f.Path, f.mode()); err != nil {
		return oops.Wrapf(err, "chmod %s", f.Path)
	}
	return nil
}

func pruneEmptyDirs(dir string) error {
	var dirs []string
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() && p != dir {
			dirs = append(dirs, p)
		}
		return nil
	})
	if err != nil {
		return oops.Wrapf(err, "scan %s", dir)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		if entries, err := os.ReadDir(d); err == nil && len(entries) == 0 {
			if err := os.Remove(d); err != nil {
				return oops.Wrapf(err, "remove %s", d)
			}
		}
	}
	return nil
}
