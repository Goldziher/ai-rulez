package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
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
	// v reads the tree being scanned.
	v workspace.View
	// root is non-empty for the project's own content: the root of v's workspace.
	root     string
	problems []ContentProblem
	// git answers which files a work tree ignores (bundle filtering); the zero
	// value runs real git.
	git gitutil.Git
}

// newProjectScanner returns a scanner for the project's own content, read
// through v. A symlink may point anywhere inside v's workspace, which is rooted at
// the repository top-level containing the project (see workspace.Around), or at
// the project itself when there is no repository.
func newProjectScanner(v workspace.View) *contentScanner {
	return &contentScanner{v: v, root: v.Root()}
}

// newIncludeScanner returns a scanner for included, installed or bundled
// content, which never follows a symlink.
func newIncludeScanner(v workspace.View) *contentScanner {
	return &contentScanner{v: v}
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
	info, err := s.v.Lstat(path)
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
	target, err := s.v.EvalSymlinks(path)
	var outside *workspace.OutsideError
	if errors.As(err, &outside) {
		s.refuse(path, fmt.Sprintf("target %s is outside the repository root %s", outside.Target, s.root))
		return nil, false
	}
	if err != nil {
		s.refuse(path, "symlink cannot be resolved")
		return nil, false
	}
	tinfo, err := s.v.Stat(target)
	if err != nil {
		s.refuse(path, "symlink target cannot be read")
		return nil, false
	}
	return tinfo, true
}

// admitTreeRoot clears a content tree root (.ai-rulez/ itself or its local/
// directory). A missing root is admitted: scanning it yields nothing.
func (s *contentScanner) admitTreeRoot(dir string) bool {
	if _, err := s.v.Lstat(dir); err != nil {
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
	entries, err = s.v.ReadDir(dir)
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
	return readContentFile(s.v, path)
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
