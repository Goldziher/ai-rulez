package improve

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Bounds on a skill tree the optimizer may return.
const (
	maxTreeFiles     = 2000
	maxTreeFileBytes = 1 << 20
	maxTreeBytes     = 32 << 20
)

// Suffixes Tree.Odd entries carry after the path, so a caller can recover the path (OddPath).
const (
	oddLargeSuffix   = " (too large)"
	oddControlSuffix = " (control characters in the name)"
)

// OddPath returns the slash-separated path of a Tree.Odd entry.
func OddPath(odd string) string {
	for _, suffix := range []string{oddLargeSuffix, oddControlSuffix} {
		if p, ok := strings.CutSuffix(odd, suffix); ok {
			return p
		}
	}
	return odd
}

// Entry is one file of a skill tree.
type Entry struct {
	Data []byte
	// Exec is the owner execute bit, the only mode bit the lock pins.
	Exec bool
}

// Tree is a skill directory: slash-separated relative path to file.
type Tree struct {
	Files map[string]Entry
	// Odd lists paths that are not plain regular files (symlinks, devices,
	// hard-linked files) or that exceed the bounds.
	Odd []string
}

// Paths returns the sorted file paths.
func (t *Tree) Paths() []string {
	out := make([]string, 0, len(t.Files))
	for p := range t.Files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ReadTree reads dir without following symlinks, leaving out the top-level
// evals/ directory (not part of the skill a harness loads, and the lock digest
// leaves it out too).
func ReadTree(dir string) (*Tree, error) {
	t := &Tree{Files: map[string]Entry{}}
	var total int64
	err := filepath.WalkDir(dir, t.walk(dir, &total))
	if err != nil {
		return nil, fmt.Errorf("read skill tree %s: %w", dir, err)
	}
	sort.Strings(t.Odd)
	return t, nil
}

// walk returns the WalkDir callback that fills t from dir, adding each regular file's size to total.
func (t *Tree) walk(dir string, total *int64) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err //nolint:wrapcheck // contextual below
		}
		if rel == "." {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if slash == "evals" && d.IsDir() {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // contextual below
		}
		if done, cerr := t.classify(slash, d, info, *total); done {
			return cerr
		}
		data, err := os.ReadFile(path) //nolint:gosec // Lstat-checked regular file below the skill directory
		if err != nil {
			return err //nolint:wrapcheck // contextual below
		}
		*total += int64(len(data))
		t.Files[slash] = Entry{Data: data, Exec: info.Mode().Perm()&0o100 != 0}
		return nil
	}
}

// classify records an entry that is not a readable skill file (an odd one) or skips a directory. done is false for
// a regular file within the bounds, which the caller reads; err is the WalkDir result when done.
func (t *Tree) classify(slash string, d fs.DirEntry, info fs.FileInfo, total int64) (done bool, err error) {
	switch {
	case hasControlRune(slash):
		// A name with terminal escapes is never a skill file: refuse it like any other odd entry.
		t.Odd = append(t.Odd, slash+oddControlSuffix)
		if d.IsDir() {
			return true, filepath.SkipDir
		}
		return true, nil
	case d.IsDir():
		return true, nil
	case !info.Mode().IsRegular() || hardLinked(info):
		t.Odd = append(t.Odd, slash)
		return true, nil
	case info.Size() > maxTreeFileBytes || len(t.Files) >= maxTreeFiles || total+info.Size() > maxTreeBytes:
		t.Odd = append(t.Odd, slash+oddLargeSuffix)
		return true, nil
	}
	return false, nil
}

// hasControlRune reports a control or format character (terminal escapes, bidi overrides) in s.
func hasControlRune(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0
}

// WriteTree writes tree below dir (created if missing) through safefs, so no
// existing symlink is followed.
func WriteTree(dir string, tree *Tree) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, rel := range tree.Paths() {
		if err := writeEntry(filepath.Join(dir, filepath.FromSlash(rel)), tree.Files[rel]); err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(file string, e Entry) error {
	if err := safefs.WriteFileAtomic(file, e.Data); err != nil {
		return err //nolint:wrapcheck // safefs errors name the path
	}
	mode := os.FileMode(0o644)
	if e.Exec {
		mode = 0o755
	}
	if err := os.Chmod(file, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", file, err)
	}
	return nil
}

// matchGlob matches a slash path against a pattern that is either an exact
// path, a path.Match pattern or "dir/**" for everything below dir.
func matchGlob(pattern, rel string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	ok, err := path.Match(pattern, rel)
	return err == nil && ok
}

func matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if matchGlob(p, rel) {
			return true
		}
	}
	return false
}
