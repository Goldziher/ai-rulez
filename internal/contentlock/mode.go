package contentlock

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// modeResolver gives the digest mode (ModeRegular or ModeExecutable) of single
// files across one run. On a filesystem that keeps Unix permission bits it is the
// execute bit of the file. Windows filesystems have no execute bit (Go reports
// 0666 or 0444), so there the bit comes from the git index (the mode git itself
// would check out on Unix); without git or outside a repository the file is
// regular. A checkout therefore pins the same digest on every operating system
// as long as the repository records the bit. The index is read once per tree
// (tracked, one bounded `git ls-files` call), never once per file.
type modeResolver struct {
	goos    string
	tracked func(dir string) (map[string]uint32, bool, error)
	trees   map[string]map[string]uint32
}

func newModeResolver() *modeResolver {
	return &modeResolver{goos: runtime.GOOS, tracked: gitutil.TrackedFiles, trees: map[string]map[string]uint32{}}
}

// mode returns the digest mode of the file at abs, whose index entries are read
// from the tree rooted at root.
func (r *modeResolver) mode(root, abs string, info os.FileInfo) string {
	if r.goos != "windows" {
		return ModeFor(uint32(info.Mode().Perm()))
	}
	files, seen := r.trees[root]
	if !seen {
		var ok bool
		var err error
		if files, ok, err = r.tracked(root); !ok || err != nil {
			files = nil
		}
		r.trees[root] = files
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ModeRegular
	}
	if files[filepath.ToSlash(rel)] == 0o100755 {
		return ModeExecutable
	}
	return ModeRegular
}

// treeModes returns the mode resolver of one tree. Off Windows it reads the
// file's own execute bit. On Windows the git index is read once for the whole
// tree (one bounded `git ls-files` call through tracked), never once per file.
func treeModes(goos, dir string, tracked func(dir string) (map[string]uint32, bool, error)) func(f treeFile) string {
	if goos != "windows" {
		return func(f treeFile) string { return ModeFor(uint32(f.info.Mode().Perm())) }
	}
	files, ok, err := tracked(dir)
	return func(f treeFile) string {
		if ok && err == nil && files[f.rel] == 0o100755 {
			return ModeExecutable
		}
		return ModeRegular
	}
}
