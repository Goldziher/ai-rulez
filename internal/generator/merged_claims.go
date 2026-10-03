package generator

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// Merged JSON documents (.claude/settings.json, .gemini/settings.json,
// opencode.json, ...) are shared with the consumer, so ai-rulez never deletes or
// regenerates them whole. What it wrote into them is recorded as claims in the
// machine-local manifest (the names of MCP servers can come from the local
// overlay, so the record is per machine), and a claim that stops being produced,
// or that clean is asked to remove, is taken back out with jsonmerge.Unmerge.
//
// Without a record (a document merged by 4.23.0 or earlier, or a fresh clone) the
// fallback is deliberately narrow: only values ai-rulez would write itself, and
// only MCP server names the current config declares. See presets.LegacyMergeClaims.

// mergedEdit is the result of taking ai-rulez's claims out of one document.
type mergedEdit struct {
	rel    string
	abs    string
	body   string
	delete bool // nothing user-authored remains
}

// previousMergedClaims returns the claims the previous run recorded; empty when
// the run deliberately ignores local inputs (the record lives beside them).
func (g *Generator) previousMergedClaims() map[string][]jsonmerge.Claim {
	if g.localSkipped {
		return nil
	}
	return readManifestFile(g.localManifestPath()).Merged
}

// currentMergedClaims collects the claims of this run's outputs by manifest path.
// With sharedOnly set, only documents shared with the user are included: those are
// the ones recorded in the manifest, because a document ai-rulez wrote whole is
// deleted whole and needs no record (keeping one for every project that merely
// has a gemini or opencode preset would put a machine-local file in all of them).
func (g *Generator) currentMergedClaims(outputs []config.OutputFile, sharedOnly bool) map[string][]jsonmerge.Claim {
	current := map[string][]jsonmerge.Claim{}
	for _, output := range outputs {
		if output.IsDir || len(output.MergeClaims) == 0 || (sharedOnly && !output.PartiallyOwned) {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if g.skippedPaths[rel] {
			continue
		}
		current[rel] = append(current[rel], output.MergeClaims...)
	}
	return current
}

// subtractClaims returns the claims in prev that cur no longer makes: a path cur
// claims whole (or through an ancestor) is still ours, and an array claim keeps
// only the elements cur does not claim.
func subtractClaims(prev, cur []jsonmerge.Claim) []jsonmerge.Claim {
	var gone []jsonmerge.Claim
	for _, claim := range prev {
		remaining := claim
		covered := false
		for _, now := range cur {
			if !isPathPrefix(now.Path, claim.Path) {
				continue
			}
			if now.Elements == nil || len(now.Path) < len(claim.Path) {
				covered = true
				break
			}
			if claim.Elements == nil {
				covered = true
				break
			}
			remaining.Elements = elementsWithout(remaining.Elements, now.Elements)
		}
		if covered || (claim.Elements != nil && len(remaining.Elements) == 0) {
			continue
		}
		gone = append(gone, remaining)
	}
	return gone
}

func isPathPrefix(prefix, path []string) bool {
	return len(prefix) <= len(path) && slices.Equal(prefix, path[:len(prefix)])
}

func elementsWithout(elements, drop []any) []any {
	kept := []any{}
	for _, element := range elements {
		if !slices.ContainsFunc(drop, func(d any) bool { return jsonValuesEqual(d, element) }) {
			kept = append(kept, element)
		}
	}
	return kept
}

func jsonValuesEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

// mergedDocumentOwners lists the presets that write each merged document, so the
// fallback never touches a document whose preset is still on.
var mergedDocumentOwners = map[string][]string{
	presets.MergedDocGeminiSettings: {"gemini"},
	presets.MergedDocOpencodeConfig: {"opencode"},
	presets.MergedDocAgentsSettings: {"antigravity"},
	presets.MergedDocXumMCP:         {"xum"},
	presets.MergedDocMCPJSON:        {"cursor", "copilot", "mcp"},
	".claude/settings.json":         {"claude"},
}

// presetEnabled reports whether any of the named presets generates this run.
func (g *Generator) presetEnabled(names []string) bool {
	for _, name := range names {
		if name == "mcp" && (len(g.config.MCPServers) > 0 || g.config.HasSelfServer()) {
			return true
		}
		for _, preset := range g.config.Presets {
			if preset.GetName() == name {
				return true
			}
		}
	}
	return false
}

// mergedDocuments lists every base-relative merged document path, sorted.
func mergedDocuments() []string {
	docs := append(providers.MergedSidecarPaths(), presets.MergedDocumentPaths()...)
	sort.Strings(docs)
	return slices.Compact(docs)
}

// planUnmerge computes, for every merged document with claims to take back, the
// document without them. For generate (clean false) that is what the previous
// run claimed and this run no longer does; for clean it is everything either run
// claims. A document with no record falls back to presets.LegacyMergeClaims: on
// clean always, on generate only when no preset that writes it is enabled.
func (g *Generator) planUnmerge(outputs []config.OutputFile, clean bool) []mergedEdit {
	claims := g.claimsToTakeBack(outputs, clean)
	rels := make([]string, 0, len(claims))
	for rel := range claims {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	var edits []mergedEdit
	for _, rel := range rels {
		if len(claims[rel]) == 0 {
			continue
		}
		abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(rel))
		if !isUnderBaseDir(g.config.BaseDir, abs) || !pathIsFile(abs) {
			continue
		}
		result, err := jsonmerge.Unmerge(abs, claims[rel])
		if err != nil {
			warnUnmerge(rel, err)
			continue
		}
		if result.Changed {
			edits = append(edits, mergedEdit{rel: rel, abs: abs, body: result.Body, delete: result.Empty})
		}
	}
	return edits
}

// claimsToTakeBack selects, by manifest path, the claims planUnmerge removes.
func (g *Generator) claimsToTakeBack(outputs []config.OutputFile, clean bool) map[string][]jsonmerge.Claim {
	previous := g.previousMergedClaims()
	current := g.currentMergedClaims(outputs, false)

	claims := map[string][]jsonmerge.Claim{}
	for rel, prev := range previous {
		if clean {
			claims[rel] = append(slices.Clone(prev), current[rel]...)
		} else {
			claims[rel] = subtractClaims(prev, current[rel])
		}
	}
	if clean {
		for rel, cur := range current {
			if _, recorded := previous[rel]; !recorded {
				claims[rel] = cur
			}
		}
	}
	names := g.serverNames()
	for _, rel := range mergedDocuments() {
		if _, recorded := previous[rel]; recorded {
			continue
		}
		if !clean && (len(current[rel]) > 0 || g.presetEnabled(mergedDocumentOwners[rel])) {
			continue
		}
		claims[rel] = append(claims[rel], presets.LegacyMergeClaims(rel, names)...)
	}
	return claims
}

// warnUnmerge reports a document ai-rulez could not take its content out of.
func warnUnmerge(rel string, err error) {
	if errors.Is(err, jsonmerge.ErrNotStrictJSON) {
		logger.Warn(rel+" has comments or trailing commas, so ai-rulez leaves it alone and what it merged there stays",
			"hint", "remove the ai-rulez entries by hand")
		return
	}
	logger.Warn("Could not remove ai-rulez content from "+rel, "error", err)
}

// serverNames lists the MCP servers the config declares, sorted.
func (g *Generator) serverNames() []string {
	names := make([]string, 0, len(g.config.MCPServers))
	for name := range g.config.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// applyUnmerge writes the edits: a document with nothing user-authored left is
// deleted, any other is rewritten with its mode kept.
func (g *Generator) applyUnmerge(edits []mergedEdit) {
	for _, edit := range edits {
		if edit.delete {
			g.removeStaleFile(edit.abs)
			continue
		}
		mode := os.FileMode(0o644)
		if info, err := os.Stat(edit.abs); err == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(edit.abs, []byte(edit.body), mode); err != nil {
			logger.Warn("Failed to remove ai-rulez content from merged document",
				"path", edit.rel, "error", oops.Wrapf(err, "write merged document"))
			continue
		}
		logger.Debug("Removed ai-rulez content from merged document", "path", edit.rel)
	}
}

// deletedPaths lists the absolute paths of documents an edit set deletes.
func deletedPaths(edits []mergedEdit) []string {
	var paths []string
	for _, edit := range edits {
		if edit.delete {
			paths = append(paths, edit.abs)
		}
	}
	return paths
}

// editedPaths lists the absolute paths of documents an edit set rewrites.
func editedPaths(edits []mergedEdit) []string {
	var paths []string
	for _, edit := range edits {
		if !edit.delete {
			paths = append(paths, edit.abs)
		}
	}
	return paths
}

// rewrites keeps the edits that rewrite a document rather than delete it.
func rewrites(edits []mergedEdit) []mergedEdit {
	return slices.DeleteFunc(slices.Clone(edits), func(edit mergedEdit) bool { return edit.delete })
}
