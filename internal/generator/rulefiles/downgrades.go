package rulefiles

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// The downgrade collector and the warn sink are process-global: a generate run
// calls ResetDowngrades at its start and FlushDowngrades at its end, so runs
// must not overlap. The generator serializes its public entry points.
var (
	downgradeMu sync.Mutex
	downgrades  = map[string]struct{}{}
	// scopeWarned holds the scopes already warned about unscoped auto/manual rules.
	scopeWarned = map[string]struct{}{}
	// negatedWarned holds the items already warned about for having only negated globs.
	negatedWarned = map[string]struct{}{}
	// warn is the sink for warnings from this package; tests replace it.
	warn = logger.Warn
	// warnedMessages holds the warnings already issued since the last reset, so a
	// message the baseline render and the real render both produce is shown once.
	warnedMessages = map[string]struct{}{}
)

// SetWarnSink replaces the sink for warnings from this package (the aggregated
// downgrade warning and ReportNotes) and returns a function restoring the
// previous one. It exists for tests.
func SetWarnSink(fn func(msg string, args ...any)) (restore func()) {
	downgradeMu.Lock()
	defer downgradeMu.Unlock()
	prev := warn
	warn = fn
	clear(warnedMessages)
	return func() {
		downgradeMu.Lock()
		defer downgradeMu.Unlock()
		warn = prev
	}
}

// Warn emits a warning through the sink, so presets and tests share one channel.
// A message already issued since the last ResetDowngrades is not repeated.
func Warn(msg string, args ...any) {
	downgradeMu.Lock()
	_, seen := warnedMessages[msg]
	warnedMessages[msg] = struct{}{}
	sink := warn
	downgradeMu.Unlock()
	if !seen {
		sink(msg, args...)
	}
}

func warnSink() func(string, ...any) {
	downgradeMu.Lock()
	defer downgradeMu.Unlock()
	return warn
}

// RecordDowngrade notes that an item's activation mode could not be expressed
// by the target (an inline root file or a rules-folder dialect) and was
// rendered as always-on. Duplicates (the same
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
	collisionMu.Lock()
	clear(collisionWarned)
	collisionMu.Unlock()
	clear(scopeWarned)
	clear(negatedWarned)
	clear(warnedMessages)
}

// warnScopeOnce reports whether this is the first call for slug since the last
// ResetDowngrades.
func warnScopeOnce(slug string) bool {
	downgradeMu.Lock()
	defer downgradeMu.Unlock()
	if _, ok := scopeWarned[slug]; ok {
		return false
	}
	scopeWarned[slug] = struct{}{}
	return true
}

// WarnOnlyNegated warns, once per item and run, that a rule or context file
// with only negated globs cannot be scoped by the rule files and stays in the
// root file.
func WarnOnlyNegated(kind string, cf config.ContentFile, rootFile string) {
	downgradeMu.Lock()
	_, seen := negatedWarned[kind+" "+cf.Name]
	negatedWarned[kind+" "+cf.Name] = struct{}{}
	downgradeMu.Unlock()
	if seen {
		return
	}
	warnSink()(kind+" \""+cf.Name+"\" has only negated globs, which rule files cannot express; kept in "+rootFile,
		"path", cf.Path)
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
	warnSink()(strconv.Itoa(len(entries))+" rules/context items use an activation the target tool cannot express; "+
		"they are loaded always:", "items", strings.Join(entries, ", "))
}

// ReportNotes forwards the notes Render returned for the file at path:
// downgrades are aggregated into the per-run warning, anything else is warned
// about once, naming the file.
func ReportNotes(path string, notes []Note) {
	for _, n := range notes {
		if n.Downgrade {
			RecordDowngrade(kindLabel(n.Kind), n.Name, string(n.Mode))
			continue
		}
		warnSink()(n.Text, "file", path)
	}
}
