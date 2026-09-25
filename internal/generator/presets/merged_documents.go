package presets

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

// Base-relative paths of the JSON settings documents preset generators share with
// their consumer: ai-rulez owns a few top-level keys and the user owns the rest,
// so these are read-modify-written rather than replaced (#185).
//
// The set has to be declared rather than observed, because the generator consults
// it precisely when a render did NOT emit the path — a document gated out by
// has-MCP-servers is absent from the render's outputs, and deleting it as stale
// would destroy the user's own settings.
const (
	MergedDocAgentsSettings = ".agents/settings.json"
	MergedDocGeminiSettings = ".gemini/settings.json"
	MergedDocMCPJSON        = ".mcp.json"
)

// mergedDocumentPaths is the registry backing MergedDocumentPaths. Every path a
// preset passes to applyMergedDocument must appear here; applyMergedDocument
// fails loudly otherwise, so a new merged document cannot silently skip the
// stale-deletion guard.
var mergedDocumentPaths = []string{
	MergedDocAgentsSettings,
	MergedDocGeminiSettings,
	MergedDocMCPJSON,
}

// MergedDocumentPaths returns every base-relative, slash-separated path that a
// preset generator renders as a merged JSON document. The generator unions this
// with the equivalent set derived from provider sidecar specs.
func MergedDocumentPaths() []string {
	paths := make([]string, len(mergedDocumentPaths))
	copy(paths, mergedDocumentPaths)
	sort.Strings(paths)

	return paths
}

// applyMergedDocument merges the owned keys into the JSON document at path,
// rejecting a path the registry does not know about.
//
// The check is what keeps the registry honest: a preset that starts merging a new
// document without registering it would otherwise generate correctly and then have
// that document deleted as stale on the next run whose gate drops it.
//
// An empty path is accepted: it means "render a fresh document" rather than
// naming a file, so there is nothing on disk for the stale-deletion guard to
// protect and the registry has no bearing on it.
func applyMergedDocument(path string, owned []jsonmerge.OwnedKey) (jsonmerge.Result, error) {
	if path != "" && !isRegisteredMergedDocument(path) {
		return jsonmerge.Result{}, oops.
			With("path", path).
			Hint("Add the base-relative path to mergedDocumentPaths in presets/merged_documents.go").
			Errorf("merged JSON document path is not registered")
	}

	return jsonmerge.Apply(path, owned)
}

// isRegisteredMergedDocument reports whether path ends in one of the registered
// base-relative paths. Matching on the tail rather than computing a relative path
// keeps callers from having to thread baseDir through their render helpers, and is
// unambiguous because every registered entry is itself a full tail from baseDir.
func isRegisteredMergedDocument(path string) bool {
	slashed := filepath.ToSlash(path)
	for _, relPath := range mergedDocumentPaths {
		if slashed == relPath || strings.HasSuffix(slashed, "/"+relPath) {
			return true
		}
	}

	return false
}
