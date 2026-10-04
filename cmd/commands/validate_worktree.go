package commands

import (
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

var worktreeWarned sync.Map

// warnWorktreeMarketplace warns, once per marketplace and checkout, when the
// managed extraKnownMarketplaces entry is a relative directory source and the
// project sits in a linked git worktree. Claude Code is reported to resolve a
// relative directory marketplace against the main checkout, so plugin content
// edited in the worktree is not seen until it reaches the main checkout.
func warnWorktreeMarketplace(cfg *config.Config) {
	path, ok := cfg.RelativeDirectoryMarketplace()
	if !ok || !gitutil.IsLinkedWorktree(cfg.BaseDir) {
		return
	}
	key := cfg.BaseDir + "\x00" + path
	if _, dup := worktreeWarned.LoadOrStore(key, true); dup {
		return
	}
	name := ""
	if cfg.Marketplace != nil {
		name = cfg.Marketplace.Name
	}
	logger.Warn("This checkout is a linked git worktree and the managed Claude marketplace uses a relative directory source; "+
		"Claude Code may resolve it against the main checkout, so plugin edits made here are not visible until they are merged there. "+
		"Set [claude.settings.marketplace_source] to an absolute path (or a per-user path) to test plugin changes from a worktree",
		"marketplace", name, "path", path)
}
