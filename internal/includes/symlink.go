package includes

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// isRealDir reports whether path is a directory that is not a symlink. Included
// content never follows symlinks, so a symlinked .ai-rulez/ or content
// directory is skipped with a warning that names it.
func isRealDir(log logger.Logger, path string) bool {
	return isRealDirIn(workspace.OSView(path), log, path)
}

// isRealDirIn is isRealDir reading through v.
func isRealDirIn(v workspace.View, log logger.Logger, path string) bool {
	info, err := v.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		log.Warn("Skipping symlinked directory in included content; symlinks are not followed", "path", path)
		return false
	}
	return info.IsDir()
}
