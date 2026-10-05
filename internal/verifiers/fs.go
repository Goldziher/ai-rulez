package verifiers

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/verifiers/vspec"
	"github.com/samber/oops"
)

// cleanRel normalizes a repo-relative path and refuses one that leaves the root.
func cleanRel(rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", oops.Errorf("path %q is outside the project", rel)
	}
	return clean, nil
}

// realRoot resolves the project root's symlinks once per run.
func (e *Env) realRoot() (string, error) {
	if !e.rootDone {
		e.rootDone = true
		e.rootReal, e.rootErr = filepath.EvalSymlinks(e.Root)
		if e.rootErr != nil {
			e.rootErr = oops.Wrapf(e.rootErr, "resolve project root")
		}
	}
	return e.rootReal, e.rootErr
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isMissing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// stat resolves a repo-relative path component by component, following
// symlinks, and reports whether it exists. A path that does not resolve (a
// missing component, a dangling symlink, a file used as a directory) does not
// exist; a symlink anywhere on the way that resolves outside the project is an
// error, so a probe never reports on what lies beyond the root.
func (e *Env) stat(ctx context.Context, rel string) (real string, exists bool, err error) {
	clean, err := cleanRel(rel)
	if err != nil {
		return "", false, err
	}
	root, err := e.realRoot()
	if err != nil {
		return "", false, err
	}
	cur := root
	if clean == "." {
		return cur, true, nil
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if err := ctx.Err(); err != nil {
			return "", false, oops.Wrapf(err, "verifier canceled")
		}
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if err != nil {
			if isMissing(err) {
				return "", false, nil
			}
			return "", false, oops.Wrapf(err, "stat %s", rel)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			cur = next
			continue
		}
		target, err := filepath.EvalSymlinks(next)
		if err != nil {
			if isMissing(err) {
				return "", false, nil
			}
			return "", false, oops.Wrapf(err, "resolve %s", rel)
		}
		if !within(root, target) {
			return "", false, oops.Errorf("%s resolves outside the project", rel)
		}
		cur = target
	}
	return cur, true, nil
}

// readFile reads up to maxFileBytes of a repo-relative file. truncated is true
// when the file is larger, in which case data is only its prefix.
func (e *Env) readFile(ctx context.Context, rel string) (data []byte, truncated bool, err error) {
	real, exists, err := e.stat(ctx, rel)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, oops.Errorf("read %s: no such file", rel)
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, false, oops.Wrapf(err, "read %s", rel)
	}
	defer f.Close()
	data, err = io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, false, oops.Wrapf(err, "read %s", rel)
	}
	if len(data) > maxFileBytes {
		return data[:maxFileBytes], true, nil
	}
	return data, false, nil
}

// match returns the sorted files selected by the verifier's glob and exclude.
// incomplete is non-empty when part of the tree could not be read, so the
// answer would be unreliable; callers report it as the finding.
func (e *Env) match(ctx context.Context, v config.VerifierConfig) (files []string, incomplete string, err error) {
	inc, err := vspec.CompileGlob(v.Glob)
	if err != nil {
		return nil, "", oops.Wrapf(err, "invalid glob %q", v.Glob)
	}
	excl := make([]vspec.Glob, 0, len(v.Exclude))
	for _, p := range v.Exclude {
		g, err := vspec.CompileGlob(p)
		if err != nil {
			return nil, "", oops.Wrapf(err, "invalid exclude glob %q", p)
		}
		excl = append(excl, g)
	}
	all, err := e.listFiles(ctx)
	if err != nil {
		return nil, "", err
	}
	var out []string
	for _, f := range all {
		if inc.Match(f) && !excludedBy(excl, f) {
			out = append(out, f)
		}
	}
	var blocked []string
	for _, d := range e.unreadable {
		if !excludedBy(excl, d+"/x") {
			blocked = append(blocked, d)
		}
	}
	if len(blocked) > 0 {
		incomplete = "cannot read directory " + listFirst(blocked) + ": the result would be incomplete"
	}
	return out, incomplete, nil
}

func excludedBy(excl []vspec.Glob, f string) bool {
	for _, g := range excl {
		if g.Match(f) {
			return true
		}
	}
	return false
}

// listFiles walks the project once and returns its regular files, slash
// separated, sorted. Symlinks are not followed or listed. A directory that
// cannot be read is recorded in e.unreadable and skipped, not fatal.
func (e *Env) listFiles(ctx context.Context) ([]string, error) {
	if e.built {
		return e.files, nil
	}
	err := filepath.WalkDir(e.Root, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr //nolint:wrapcheck // wrapped below
		}
		if err != nil {
			if p == e.Root {
				return err
			}
			rel, rerr := filepath.Rel(e.Root, p)
			if rerr != nil {
				return rerr //nolint:wrapcheck // wrapped below
			}
			dir := filepath.ToSlash(rel)
			if d != nil && !d.IsDir() {
				dir = filepath.ToSlash(filepath.Dir(rel))
			}
			e.unreadable = append(e.unreadable, dir)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != e.Root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(e.Root, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		e.files = append(e.files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "walk %s", e.Root)
	}
	sort.Strings(e.files)
	sort.Strings(e.unreadable)
	e.built = true
	return e.files, nil
}
