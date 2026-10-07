package generator

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// maxSymlinkHops bounds how many links one write path may traverse, matching the
// kernel's own limit so a link cycle fails instead of looping.
const maxSymlinkHops = 40

// guardWrite resolves the file a write to abs would really land on and refuses
// it when a symlink (the file itself, or any parent directory) leads out of the
// places the run may write: the project directory, and in user scope the home
// directory and the relocated tool homes. It returns the resolved path to write
// to, and whether the final component was itself a link (a mode must never be
// changed through one). A write that crosses no symlink is returned unchanged.
//
// Writing through a link that stays inside the project is deliberate (a
// CLAUDE.md -> AGENTS.md link); a link out of it would let a hostile checkout
// overwrite arbitrary files, so it is an error naming the link.
func (g *Generator) guardWrite(abs string) (target string, linked bool, err error) {
	abs = filepath.Clean(abs)
	// Fail closed: a generator never writes outside the project (or the user-scope
	// roots), whatever a config-supplied path says, and never into a git directory.
	if !g.withinScope(abs) {
		return "", false, oops.With("path", abs).
			Hint("Output paths must stay inside the project directory").
			Errorf("refusing to write %s: it is outside the project", abs)
	}
	if g.inGitDir(abs) {
		return "", false, oops.With("path", abs).
			Hint("ai-rulez never writes inside .git; a path like that in a preset or provider spec is rejected").
			Errorf("refusing to write %s: it is inside a git directory", abs)
	}
	hops := 0
	resolved, viaLink, err := resolveWriteTarget(abs, &hops)
	if err != nil {
		return "", false, oops.With("path", abs).Wrapf(err, "resolve %s", abs)
	}
	for _, root := range g.writeRoots() {
		realRoot, _, rerr := resolveWriteTarget(root, new(int))
		if rerr != nil {
			realRoot = root
		}
		if isUnderBaseDir(realRoot, resolved) {
			return resolved, viaLink, nil
		}
	}
	return "", false, oops.With("path", abs).With("resolves_to", resolved).
		Hint("Remove the symlink, or point it at a file inside the project; ai-rulez never writes through a link that leaves it").
		Errorf("refusing to write %s: a symlink resolves it outside the project, to %s", abs, resolved)
}

// writeRoots lists the directories a write may resolve into.
func (g *Generator) writeRoots() []string {
	roots := []string{g.config.BaseDir}
	if g.userMode {
		roots = append(roots, g.userHomes...)
	}
	return slices.Clone(roots)
}

// resolveWriteTarget resolves every symlink in p, parent directories first,
// following a dangling final link lexically, so the result is where a write
// would land. Components that do not exist yet cannot be links and are kept.
func resolveWriteTarget(p string, hops *int) (out string, link bool, err error) {
	p = filepath.Clean(p)
	parent := filepath.Dir(p)
	if parent == p {
		return p, false, nil
	}
	realParent, _, err := resolveWriteTarget(parent, hops)
	if err != nil {
		return "", false, err
	}
	candidate := filepath.Join(realParent, filepath.Base(p))
	info, err := os.Lstat(candidate)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return candidate, false, nil //nolint:nilerr // an absent component is not a link
	}
	*hops++
	if *hops > maxSymlinkHops {
		return "", false, oops.Errorf("too many levels of symbolic links at %s", candidate)
	}
	dest, err := os.Readlink(candidate)
	if err != nil {
		return "", false, oops.Wrapf(err, "read link %s", candidate)
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(realParent, dest)
	}
	resolved, _, err := resolveWriteTarget(dest, hops)
	return resolved, true, err
}

// inGitDir reports whether abs, relative to the root that scopes it, has a
// segment naming a git directory.
func (g *Generator) inGitDir(abs string) bool {
	for _, root := range g.writeRoots() {
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
			if config.IsGitDirName(seg) {
				return true
			}
		}
		return false
	}
	return false
}
