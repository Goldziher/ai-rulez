package contentlock

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// fileMode returns the digest mode (ModeRegular or ModeExecutable) of the file at
// abs. On a filesystem that keeps Unix permission bits it is the execute bit of
// the file. Windows filesystems have no execute bit (Go reports 0666 or 0444), so
// there the bit comes from the git index (`git ls-files -s`, the mode git itself
// would check out on Unix); without git or outside a repository the file is
// regular. A checkout therefore pins the same digest on every operating system
// as long as the repository records the bit.
func fileMode(abs string, info os.FileInfo) string {
	return fileModeFor(runtime.GOOS, abs, info, gitIndexMode)
}

func fileModeFor(goos, abs string, info os.FileInfo, index func(abs string) (string, bool)) string {
	if goos == "windows" {
		if mode, ok := index(abs); ok {
			return mode
		}
		return ModeRegular
	}
	return ModeFor(uint32(info.Mode().Perm()))
}

// treeModes returns the mode resolver of one tree. Off Windows it reads the
// file's own execute bit. On Windows the git index is read once for the whole
// tree (one bounded `git ls-files` call through tracked), never once per file.
func treeModes(goos, dir string, tracked func(dir string) (map[string]uint32, bool, error)) func(f treeFile) string {
	if goos != "windows" {
		return func(f treeFile) string { return fileModeFor(goos, f.abs, f.info, nil) }
	}
	files, ok, err := tracked(dir)
	return func(f treeFile) string {
		if ok && err == nil && files[f.rel] == 0o100755 {
			return ModeExecutable
		}
		return ModeRegular
	}
}

// indexLookupTimeout bounds the one-file git index lookup.
const indexLookupTimeout = 30 * time.Second

// gitIndexMode reads the mode git has recorded for abs.
func gitIndexMode(abs string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), indexLookupTimeout)
	defer cancel()
	cmd := gitutil.Command(ctx, filepath.Dir(abs), "ls-files", "-s", "--", filepath.Base(abs))
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 1 {
		return "", false
	}
	switch fields[0] {
	case ModeExecutable:
		return ModeExecutable, true
	case ModeRegular, "120000":
		return ModeRegular, true
	}
	return "", false
}
