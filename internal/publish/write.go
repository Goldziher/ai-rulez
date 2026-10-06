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
//
// Everything is staged in a sibling directory first. A new or empty dir is
// replaced by the staged directory in one rename; an earlier publish keeps its
// plan until the new plan (installed last) lands, so a crash at any point
// leaves a directory publish can still reuse. A symlink at an artifact, at a
// directory component or as the dist directory itself is refused, never
// followed: renames replace a link, they do not write through it.
func (d *Dist) Write(dir string) error {
	old, err := inspectDir(dir)
	if err != nil {
		return err
	}
	paths := d.Paths()
	if old != nil {
		if err := checkInstallTargets(dir, old, paths); err != nil {
			return err
		}
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return oops.With("path", parent).Wrapf(err, "create dist directory")
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+".staging-*")
	if err != nil {
		return oops.With("path", parent).Wrapf(err, "create staging directory")
	}
	defer os.RemoveAll(stage) //nolint:errcheck // best effort; the staging directory is ours
	for _, p := range paths {
		target := filepath.Join(stage, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return oops.With("path", p).Wrapf(err, "create dist directory")
		}
		if err := os.WriteFile(target, d.Files[p], 0o644); err != nil { //nolint:gosec // release artifacts are meant to be shared
			return oops.With("path", p).Wrapf(err, "write dist file")
		}
	}
	if old == nil {
		return installFresh(stage, dir)
	}
	return installOver(stage, dir, old, paths)
}

// installFresh replaces an absent or empty dir with the staged directory.
func installFresh(stage, dir string) error {
	if err := os.Chmod(stage, 0o750); err != nil {
		return oops.With("path", dir).Wrapf(err, "set dist directory mode")
	}
	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		return oops.With("path", dir).Wrapf(err, "replace empty dist directory")
	}
	if err := os.Rename(stage, dir); err != nil {
		return oops.With("path", dir).Wrapf(err, "install dist directory")
	}
	return nil
}

// installOver renames the staged files into an earlier publish's directory,
// the plan last, then removes the earlier artifacts the new plan no longer has.
func installOver(stage, dir string, old *Plan, paths []string) error {
	ordered := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != PlanFile {
			ordered = append(ordered, p)
		}
	}
	ordered = append(ordered, PlanFile)
	for _, p := range ordered {
		target := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return oops.With("path", p).Wrapf(err, "create dist directory")
		}
		if err := os.Rename(filepath.Join(stage, filepath.FromSlash(p)), target); err != nil {
			return oops.With("path", p).Wrapf(err, "install dist file")
		}
	}
	keep := map[string]bool{}
	for _, p := range paths {
		keep[p] = true
	}
	for _, a := range old.Artifacts {
		if !ValidPath(a.Path) || keep[a.Path] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(a.Path))); err != nil && !os.IsNotExist(err) {
			return oops.With("path", a.Path).Wrapf(err, "remove earlier artifact")
		}
	}
	return nil
}

// inspectDir validates dir. It returns nil for a directory that is absent or
// empty, and the earlier plan for one holding an earlier publish's output.
func inspectDir(dir string) (*Plan, error) {
	info, err := os.Lstat(dir)
	switch {
	case os.IsNotExist(err):
		return nil, nil //nolint:nilnil // nil plan means a fresh directory
	case err != nil:
		return nil, oops.With("path", dir).Wrapf(err, "inspect dist directory")
	case !info.IsDir():
		return nil, newError(CodeBundleUnsafe, ExitFailed, "", "%s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, oops.With("path", dir).Wrapf(err, "read dist directory")
	}
	if len(entries) == 0 {
		return nil, nil //nolint:nilnil // nil plan means a fresh directory
	}
	data, err := readRegular(filepath.Join(dir, PlanFile))
	if err != nil {
		return nil, newError(CodeBundleUnsafe, ExitFailed, "choose an empty --dist directory", "%s is not empty and holds no earlier publish output", dir)
	}
	var old Plan
	if err := json.Unmarshal(data, &old); err != nil {
		return nil, newError(CodeBundleUnsafe, ExitFailed, "choose an empty --dist directory", "%s holds an unreadable %s", dir, PlanFile)
	}
	return &old, nil
}

// checkInstallTargets refuses, before anything is touched, a dist directory
// where a path this publish writes or removes goes through a symlink or ends
// in something other than a regular file.
func checkInstallTargets(dir string, old *Plan, paths []string) error {
	check := func(p string) error {
		cur := dir
		parts := splitPath(p)
		for i, part := range parts {
			cur = filepath.Join(cur, part)
			info, err := os.Lstat(cur)
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return oops.With("path", p).Wrapf(err, "inspect dist path")
			}
			last := i == len(parts)-1
			if info.Mode()&os.ModeSymlink != 0 || (last && !info.Mode().IsRegular()) || (!last && !info.IsDir()) {
				return newError(CodeBundleUnsafe, ExitFailed, "remove it or choose another --dist directory",
					"dist path %s is a symlink or not a regular file; refusing to write or remove through it", p)
			}
		}
		return nil
	}
	for _, p := range paths {
		if err := check(p); err != nil {
			return err
		}
	}
	for _, a := range old.Artifacts {
		if ValidPath(a.Path) {
			if err := check(a.Path); err != nil {
				return err
			}
		}
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
