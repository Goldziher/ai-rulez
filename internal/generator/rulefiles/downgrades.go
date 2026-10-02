package rulefiles

import (
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

var (
	downgradeMu sync.Mutex
	downgrades  = map[string]struct{}{}
	// warn is the sink for the aggregated downgrade warning; tests replace it.
	warn = logger.Warn
)

// RecordDowngrade notes that an item's activation mode could not be expressed
// in an inline root file and was rendered as always-on. Duplicates (the same
// item rendered into several root files) collapse into one entry.
func RecordDowngrade(kind, name, mode string) {
	logger.Debug("Activation downgraded to always-on in inline output", "kind", kind, "name", name, "mode", mode)
	downgradeMu.Lock()
	defer downgradeMu.Unlock()
	downgrades[kind+" "+name] = struct{}{}
}

// ResetDowngrades discards recorded downgrades. Call it at the start of a
// generate run.
func ResetDowngrades() {
	downgradeMu.Lock()
	defer downgradeMu.Unlock()
	clear(downgrades)
}

// FlushDowngrades emits one warning listing every recorded downgrade, sorted,
// then clears them. It logs nothing when none were recorded.
func FlushDowngrades() {
	downgradeMu.Lock()
	entries := make([]string, 0, len(downgrades))
	for k := range downgrades {
		entries = append(entries, k)
	}
	clear(downgrades)
	downgradeMu.Unlock()

	if len(entries) == 0 {
		return
	}
	sort.Strings(entries)
	warn("Manual activation cannot be expressed in inline root files; rendered as always-on",
		"items", strings.Join(entries, ", "))
}
