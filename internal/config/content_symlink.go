package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// ContentProblem is a project content path the loader refused to read.
type ContentProblem struct {
	Path   string
	Reason string
}

// contentWarnWriter receives symlink refusals for the project's own content.
// It is written directly instead of through the logger so the warning survives
// --quiet, which raises the log level above warnings.
var contentWarnWriter io.Writer = os.Stderr

// contentScanner applies the symlink policy while scanning a content tree.
//
// With root empty (content from includes, installed skills, OKF bundles) no
// symlink is ever followed. With root set (the project's own .ai-rulez/) a
// symlink is followed only when its fully resolved target is inside root;
// every refusal is recorded in problems and warned about.
type contentScanner struct {
	root     string
	problems []ContentProblem
}

// newProjectScanner returns a scanner for the project's own content. root is
// the git top-level containing baseDir, or baseDir itself when there is none.
func newProjectScanner(baseDir string) *contentScanner {
	root := baseDir
	if top := gitTopLevel(baseDir); top != "" {
		root = top
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return &contentScanner{root: root}
}

// gitTopLevel walks up from dir looking for a .git entry.
func gitTopLevel(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for cur := abs; ; {
		if _, err := os.Lstat(filepath.Join(cur, gitDirName)); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func (s *contentScanner) refuse(path, reason string) {
	if s.root == "" {
		logger.Warn("Skipping symlink in included content; symlinks are not followed", "path", path)
		return
	}
	s.problems = append(s.problems, ContentProblem{Path: path, Reason: reason})
	fmt.Fprintf(contentWarnWriter, "WARN  refusing symlinked content %s: %s\n", path, reason)
}

// admit reports the FileInfo of path (the target's, for an admitted symlink).
// A missing path is not admitted and not reported.
func (s *contentScanner) admit(path string) (os.FileInfo, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return info, true
	}
	if s.root == "" {
		s.refuse(path, "symlinks are not followed")
		return nil, false
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		s.refuse(path, "symlink cannot be resolved")
		return nil, false
	}
	if !within(s.root, target) {
		s.refuse(path, fmt.Sprintf("target %s is outside the project root %s", target, s.root))
		return nil, false
	}
	tinfo, err := os.Stat(target)
	if err != nil {
		s.refuse(path, "symlink target cannot be read")
		return nil, false
	}
	return tinfo, true
}

// admitTreeRoot clears a content tree root (.ai-rulez/ itself or its local/
// directory). A missing root is admitted: scanning it yields nothing.
func (s *contentScanner) admitTreeRoot(dir string) bool {
	if _, err := os.Lstat(dir); err != nil {
		return true
	}
	_, ok := s.admit(dir)
	return ok
}

// entryInfo reports whether a directory entry is a directory and whether it may
// be used at all under the symlink policy.
func (s *contentScanner) entryInfo(path string, entry os.DirEntry) (isDir, ok bool) {
	if entry.Type()&os.ModeSymlink == 0 {
		return entry.IsDir(), true
	}
	info, ok := s.admit(path)
	if !ok {
		return false, false
	}
	return info.IsDir(), true
}

// dirEntries lists dir. ok is false when dir is missing, is not a directory, or
// is a symlink the policy refuses.
func (s *contentScanner) dirEntries(dir string) (entries []os.DirEntry, ok bool, err error) {
	info, admitted := s.admit(dir)
	if !admitted || !info.IsDir() {
		return nil, false, nil
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		return nil, false, oops.With("path", dir).Wrapf(err, "read directory")
	}
	return entries, true, nil
}

// loadFile loads one content file, following an admitted symlink.
func (s *contentScanner) loadFile(path string) (ContentFile, error) {
	info, ok := s.admit(path)
	if !ok {
		return ContentFile{}, oops.With("path", path).Errorf("content file %s is not readable under the symlink policy", path)
	}
	if !info.Mode().IsRegular() {
		return ContentFile{}, oops.With("path", path).Errorf("content file %s is not a regular file", path)
	}
	return readContentFile(path)
}

// validateContentProblems turns symlink refusals into a validation error.
func (c *Config) validateContentProblems() error {
	if len(c.ContentProblems) == 0 {
		return nil
	}
	lines := make([]string, 0, len(c.ContentProblems))
	for _, p := range c.ContentProblems {
		lines = append(lines, fmt.Sprintf("%s: %s", p.Path, p.Reason))
	}
	return oops.
		Hint("A content symlink is followed only when its target is inside the project; point it at a file in the repository or replace it with a regular file").
		Errorf("refused symlinked content:\n  %s", strings.Join(lines, "\n  "))
}
