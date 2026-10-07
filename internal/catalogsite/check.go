package catalogsite

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/samber/oops"
)

// CheckResult says how a directory differs from the site Render would write.
type CheckResult struct {
	// Missing are files the site has that the directory lacks (the marker
	// included when it is absent).
	Missing []string
	// Changed are files whose bytes differ, or that are not regular files.
	Changed []string
	// Extra are files in the directory that the site does not have.
	Extra []string
}

// Drift reports whether the directory differs from the site.
func (r *CheckResult) Drift() bool {
	return len(r.Missing)+len(r.Changed)+len(r.Extra) > 0
}

// Check compares dir with site without writing anything: every file of the site
// must exist with the same bytes, and the directory must hold nothing else
// (besides the marker). A missing directory is drift, not an error, so a CI gate
// fails for a site that was never generated. Reading goes through an os.Root, so
// a symlink inside dir cannot lead the comparison outside it; a symlink is
// reported as changed (or extra), never followed.
func Check(dir string, site *Site) (*CheckResult, error) {
	if dir == "" {
		return nil, oops.Errorf("no output directory")
	}
	res := &CheckResult{}
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.Missing = append([]string{MarkerFile}, site.Paths()...)
		sort.Strings(res.Missing)
		return res, nil
	case err != nil:
		return nil, oops.With("dir", dir).Wrapf(err, "inspect output directory")
	case !info.IsDir() && info.Mode()&fs.ModeSymlink == 0:
		return nil, oops.With("dir", dir).Errorf("%s is not a directory", dir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "open output directory")
	}
	defer root.Close() //nolint:errcheck // read-only

	present, walkErr := compareTree(root, site, res)
	if walkErr != nil {
		return nil, oops.With("dir", dir).Wrapf(walkErr, "read output directory")
	}
	if !present[MarkerFile] {
		res.Missing = append(res.Missing, MarkerFile)
	}
	for _, p := range site.Paths() {
		if !present[p] {
			res.Missing = append(res.Missing, p)
		}
	}
	for _, list := range [][]string{res.Missing, res.Changed, res.Extra} {
		sort.Strings(list)
	}
	return res, nil
}

// compareTree walks root, recording every extra and changed file in res, and
// returns the set of files present.
func compareTree(root *os.Root, site *Site, res *CheckResult) (map[string]bool, error) {
	present := map[string]bool{}
	err := fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		present[p] = true
		if p == MarkerFile {
			return nil
		}
		want, ok := site.Files[p]
		switch {
		case !ok:
			res.Extra = append(res.Extra, p)
		case !d.Type().IsRegular():
			res.Changed = append(res.Changed, p)
		default:
			got, readErr := readSiteFile(root, p)
			if readErr != nil || !bytes.Equal(got, want) {
				res.Changed = append(res.Changed, p)
			}
		}
		return nil
	})
	return present, err //nolint:wrapcheck // Check wraps it with the directory
}

func readSiteFile(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(filepath.ToSlash(name))
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller only needs to know it failed
	}
	defer f.Close() //nolint:errcheck // read-only
	return io.ReadAll(io.LimitReader(f, maxSiteFileBytes))
}
