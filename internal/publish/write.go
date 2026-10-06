package publish

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

// Write puts the dist files into dir. The directory must not exist, be empty,
// or hold the output of an earlier publish (it has publish-plan.json): the
// artifacts that plan lists are replaced and nothing else is deleted.
func (d *Dist) Write(dir string) error {
	if err := prepareDir(dir); err != nil {
		return err
	}
	for _, p := range d.Paths() {
		target := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return oops.With("path", target).Wrapf(err, "create dist directory")
		}
		if err := os.WriteFile(target, d.Files[p], 0o644); err != nil { //nolint:gosec // release artifacts are meant to be shared
			return oops.With("path", target).Wrapf(err, "write dist file")
		}
	}
	return nil
}

func prepareDir(dir string) error {
	info, err := os.Lstat(dir)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return oops.With("path", dir).Wrapf(err, "create dist directory")
		}
		return nil
	case err != nil:
		return oops.With("path", dir).Wrapf(err, "inspect dist directory")
	case !info.IsDir():
		return newError(CodeBundleUnsafe, ExitFailed, "", "%s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return oops.With("path", dir).Wrapf(err, "read dist directory")
	}
	if len(entries) == 0 {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, PlanFile))
	if err != nil {
		return newError(CodeBundleUnsafe, ExitFailed, "choose an empty --dist directory", "%s is not empty and holds no earlier publish output", dir)
	}
	var old Plan
	if err := json.Unmarshal(data, &old); err != nil {
		return newError(CodeBundleUnsafe, ExitFailed, "choose an empty --dist directory", "%s holds an unreadable %s", dir, PlanFile)
	}
	for _, a := range old.Artifacts {
		if !ValidPath(a.Path) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(a.Path))); err != nil && !os.IsNotExist(err) {
			return oops.With("path", a.Path).Wrapf(err, "remove earlier artifact")
		}
	}
	if err := os.Remove(filepath.Join(dir, PlanFile)); err != nil && !os.IsNotExist(err) {
		return oops.Wrapf(err, "remove earlier plan")
	}
	return nil
}

// CheckTree rejects a bundle that contains a symlink: every component of each
// path below root must be a real directory or regular file.
func CheckTree(root string, paths []string) error {
	for _, p := range paths {
		cur := root
		parts := splitPath(p)
		for i, part := range parts {
			cur = filepath.Join(cur, part)
			info, err := os.Lstat(cur)
			if err != nil {
				return oops.With("path", p).Wrapf(err, "inspect bundle file")
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return newError(CodeBundleUnsafe, ExitFailed, "bundles cannot contain symlinks; copy the content instead", "bundle path %s goes through a symlink (%s)", p, part)
			}
			if i == len(parts)-1 && !info.Mode().IsRegular() {
				return newError(CodeBundleUnsafe, ExitFailed, "", "bundle path %s is not a regular file", p)
			}
		}
	}
	return nil
}

func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			parts = append(parts, p[start:i])
			start = i + 1
		}
	}
	return parts
}
