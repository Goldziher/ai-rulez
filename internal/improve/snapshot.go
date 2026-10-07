package improve

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// snapEntry is one path of a run-directory snapshot.
type snapEntry struct {
	dir    bool
	data   []byte
	mode   os.FileMode
	target string // symlink target; non-empty marks a symlink
}

// runSnapshot is the content of a run directory outside the optimizer's own
// areas (workspace, home, tmp): plan, original copy, train cases and earlier
// rounds. An optimizer runs unsandboxed and can reach all of it, so each round
// is checked against, and rolled back to, this snapshot.
type runSnapshot struct {
	root    string
	entries map[string]snapEntry
}

var optimizerAreas = map[string]bool{"workspace": true, "home": true, "tmp": true}

func snapshotRunDir(root string) (*runSnapshot, error) {
	s := &runSnapshot{root: root, entries: map[string]snapEntry{}}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || rel == "." {
			return rerr
		}
		slash := filepath.ToSlash(rel)
		if optimizerAreas[strings.SplitN(slash, "/", 2)[0]] {
			if d.IsDir() && !strings.Contains(slash, "/") {
				return filepath.SkipDir
			}
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr //nolint:wrapcheck // contextual below
		}
		e := snapEntry{mode: info.Mode().Perm()}
		switch {
		case d.IsDir():
			e.dir = true
		case info.Mode()&os.ModeSymlink != 0:
			if e.target, ierr = os.Readlink(path); ierr != nil {
				return ierr //nolint:wrapcheck // contextual below
			}
		case info.Mode().IsRegular():
			if e.data, ierr = os.ReadFile(path); ierr != nil { //nolint:gosec // our own run directory
				return ierr //nolint:wrapcheck // contextual below
			}
		default:
			return nil
		}
		s.entries[slash] = e
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", root, err)
	}
	return s, nil
}

// diff returns the paths that differ now, sorted; none means untouched.
func (s *runSnapshot) diff() ([]string, error) {
	now, err := snapshotRunDir(s.root)
	if err != nil {
		return nil, err
	}
	var out []string
	for p, e := range s.entries {
		if n, ok := now.entries[p]; !ok || !sameEntry(e, n) {
			out = append(out, p)
		}
	}
	for p := range now.entries {
		if _, ok := s.entries[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func sameEntry(a, b snapEntry) bool {
	return a.dir == b.dir && a.target == b.target && bytes.Equal(a.data, b.data) && a.mode == b.mode
}

// restore puts the snapshot back: paths created since are removed, changed or
// deleted ones are rewritten.
func (s *runSnapshot) restore() error {
	now, err := snapshotRunDir(s.root)
	if err != nil {
		return err
	}
	var extra []string
	for p := range now.entries {
		if _, ok := s.entries[p]; !ok {
			extra = append(extra, p)
		}
	}
	sort.Strings(extra)
	for _, p := range extra {
		if err := os.RemoveAll(filepath.Join(s.root, filepath.FromSlash(p))); err != nil {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	paths := make([]string, 0, len(s.entries))
	for p := range s.entries {
		paths = append(paths, p)
	}
	sort.Strings(paths) // parents sort before their children
	for _, p := range paths {
		e := s.entries[p]
		if n, ok := now.entries[p]; ok && sameEntry(e, n) {
			continue
		}
		if err := restoreEntry(filepath.Join(s.root, filepath.FromSlash(p)), p, &e); err != nil {
			return err
		}
	}
	return nil
}

// restoreEntry puts one snapshotted entry (named p, at full) back in place of whatever is there now.
func restoreEntry(full, p string, e *snapEntry) error {
	if err := os.RemoveAll(full); err != nil && !e.dir {
		return fmt.Errorf("replace %s: %w", p, err)
	}
	switch {
	case e.dir:
		if info, serr := os.Lstat(full); serr != nil || !info.IsDir() {
			_ = os.RemoveAll(full) //nolint:errcheck // MkdirAll reports the failure
			if err := os.MkdirAll(full, 0o700); err != nil {
				return fmt.Errorf("restore %s: %w", p, err)
			}
		}
	case e.target != "":
		if err := os.Symlink(e.target, full); err != nil {
			return fmt.Errorf("restore %s: %w", p, err)
		}
	default:
		if err := os.WriteFile(full, e.data, e.mode); err != nil {
			return fmt.Errorf("restore %s: %w", p, err)
		}
		if err := os.Chmod(full, e.mode); err != nil {
			return fmt.Errorf("restore %s: %w", p, err)
		}
	}
	return nil
}
