package rulefiles

import (
	"path"
	"slices"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
)

// rootPresets maps each root file to the presets that write it. A file shared
// by several presets must render identically whichever writes it last, so a
// target naming any of them selects an item for all. The local variants
// (CLAUDE.local.md, ...) resolve through their base file.
var rootPresets = map[string][]string{
	"agents.md":                       {"codex", "opencode", "xum", "amp", "pi", "baz", "junie"},
	"gemini.md":                       {"gemini", "antigravity"},
	"claude.md":                       {"claude"},
	".hermes.md":                      {"hermes"},
	".github/copilot-instructions.md": {"copilot"},
}

var rootPresetsMu sync.RWMutex

// RegisterRootOwner records that preset writes rootFile, so declarative provider
// specs join the owners table without a hand edit. It is idempotent and keeps the
// order owners were first added in. Call it from init() only.
func RegisterRootOwner(rootFile, preset string) {
	key := targetmatch.Normalize(rootFile)
	rootPresetsMu.Lock()
	defer rootPresetsMu.Unlock()
	if !slices.Contains(rootPresets[key], preset) {
		rootPresets[key] = append(rootPresets[key], preset)
	}
}

func rootOwnersFor(rootFile string) ([]string, bool) {
	rootPresetsMu.RLock()
	defer rootPresetsMu.RUnlock()
	owners, ok := rootPresets[targetmatch.Normalize(rootFile)]
	return owners, ok
}

// RootOwners returns the presets that write the root file by default, or nil
// when no table entry exists. The caller owns the returned slice.
func RootOwners(rootFile string) []string {
	owners, _ := rootOwnersFor(rootFile)
	return append([]string(nil), owners...)
}

// SharedRootFile reports whether several presets write the root file, so it
// must render identically whichever of them writes it last.
func SharedRootFile(rootFile string) bool {
	owners, _ := rootOwnersFor(rootFile)
	return len(owners) > 1
}

// RootTarget describes a preset's root file for inline target filtering.
func RootTarget(preset, rootFile string) Target {
	return Target{Preset: preset, RootFile: rootFile}
}

// rootOwners returns the preset names a target must name to select the root
// file of t.
func rootOwners(t Target) []string {
	if len(t.Owners) > 0 {
		return t.Owners
	}
	if owners, ok := rootOwnersFor(t.RootFile); ok {
		return owners
	}
	if t.Preset == "" {
		return nil
	}
	return []string{t.Preset}
}

// TargetsAllow reports whether an item with the given frontmatter targets may
// appear in output of t. An item without targets is allowed everywhere.
//
// relPath is the rule file path relative to the output root, or "" for the
// inlined root file. A rule file is selected by the preset name, the preset's
// root file, its path or base name, a directory prefix or a glob; the inlined
// root file by the name of any preset writing it, or the root file itself (by
// path or base name), or by a root file alias of t.
func TargetsAllow(targets []string, t Target, relPath string) bool {
	if relPath == "" {
		names := []string{t.RootFile, path.Base(t.RootFile)}
		for _, alias := range t.RootAliases {
			names = append(names, alias, path.Base(alias))
		}
		return targetmatch.Allow(targets, rootOwners(t), names...)
	}
	var presets []string
	if t.Preset != "" {
		presets = []string{t.Preset}
	}
	return targetmatch.Allow(targets, presets, relPath, path.Base(relPath), t.RootFile, path.Base(t.RootFile))
}

func itemTargets(cf config.ContentFile) []string {
	if cf.Metadata == nil {
		return nil
	}
	return cf.Metadata.Targets
}

// InlineAllowed reports whether cf may be inlined into the root file of t.
func InlineAllowed(cf config.ContentFile, t Target) bool {
	return TargetsAllow(itemTargets(cf), t, "")
}

// FilterInline drops the items whose targets exclude the root file of t.
func FilterInline(items []config.ContentFile, t Target) []config.ContentFile {
	var out []config.ContentFile
	for _, cf := range items {
		if InlineAllowed(cf, t) {
			out = append(out, cf)
		}
	}
	return out
}

// filterInlineFor is FilterInline for a possibly nil target (no rules folder,
// hence no preset to match targets against: everything stays).
func filterInlineFor(t *Target, items []config.ContentFile) []config.ContentFile {
	if t == nil {
		return items
	}
	return FilterInline(items, *t)
}

// FileAllowed reports whether cf may be written as a rule file of t.
func FileAllowed(cf config.ContentFile, t Target, kind Kind) bool {
	return fileAllowed(cf, t, kind, ScopeInfo{})
}

func fileAllowed(cf config.ContentFile, t Target, kind Kind, scope ScopeInfo) bool {
	targets := itemTargets(cf)
	if len(targets) == 0 {
		return true
	}
	it := Item{File: cf, Kind: kind, ID: ScopedID(t, scope.Slug, ItemID(cf.Name))}
	return TargetsAllow(targets, t, path.Join(t.Dir, FileName(t, it)))
}
