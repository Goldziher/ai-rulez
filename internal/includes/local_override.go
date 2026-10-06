package includes

import (
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// checkLocalOverride refuses a local_override that bypasses an enforced lock.
// local_override swaps a pinned remote source for an unpinned local directory,
// so it is a development shortcut: it is honoured from the machine-local
// overlay (config.local.*, never committed), or when no lock is enforced. A
// committed local_override under --locked, --frozen or an enforced lock fails.
// listKey is the config list the entry lives in ("includes", "installed_skills").
func checkLocalOverride(cfg *config.Config, listKey, name string) error {
	if !strictLock(cfg) || overlaySetsLocalOverride(cfg, listKey, name) {
		return nil
	}
	return oops.
		With("name", name).
		Hint("Set local_override in config.local.toml (machine-local, not committed), or remove it from the committed config; a lock is enforced whenever ai-rulez.lock exists (unless [lock] enforce = false) and under --locked or --frozen").
		Wrapf(config.ErrLockViolation, "%s %q sets local_override in the committed config, which would bypass the lock", listKey, name)
}

// overlaySetsLocalOverride reports whether the machine-local overlay is what
// sets local_override on the named entry.
func overlaySetsLocalOverride(cfg *config.Config, listKey, name string) bool {
	return overlaySetsField(cfg, listKey, name, "local_override")
}

// overlaySetsField reports whether the machine-local overlay sets field on the
// named entry of listKey.
func overlaySetsField(cfg *config.Config, listKey, name, field string) bool {
	if cfg == nil || cfg.LocalOverlay == nil {
		return false
	}
	list, _ := cfg.LocalOverlay.Doc[listKey].([]any)
	for _, item := range list {
		entry, _ := item.(map[string]any)
		if entry == nil {
			continue
		}
		if n, _ := entry["name"].(string); n != name {
			continue
		}
		if v, _ := entry[field].(string); v != "" {
			return true
		}
	}
	return false
}
