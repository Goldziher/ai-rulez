package generator

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// Merged JSON documents (.claude/settings.json, .gemini/settings.json,
// opencode.json, ...) are shared with the consumer, so ai-rulez never deletes or
// regenerates them whole. What it wrote into them is recorded as claims in the
// machine-local manifest (the names of MCP servers can come from the local
// overlay, so the record is per machine), and a claim that stops being produced,
// or that clean is asked to remove, is taken back out with docmerge.Unmerge.
//
// Every claim is guarded by a digest of the value ai-rulez wrote, and only a value
// that still matches it is taken back: an entry the user edited is theirs, stays,
// and is reported once. Claims without a guard (an older record) are held to the
// value the current config renders now, and kept when there is none.
//
// Without a record (a document merged by 4.23.0 or earlier, or a fresh clone) clean
// alone falls back, and deliberately narrowly: only values ai-rulez would write
// itself, and only MCP servers the current config declares whose entry equals what
// it would render. Generate never uses the fallback, because a hand-written entry
// it cannot tell from its own must not be deleted on every run. See
// presets.LegacyMergeClaims.

// mergedWarn reports a merged document ai-rulez left alone; tests replace it.
var mergedWarn = logger.Warn

// mergedEdit is the result of taking ai-rulez's claims out of one document.
type mergedEdit struct {
	rel    string
	abs    string
	body   string
	delete bool // nothing user-authored remains
}

// previousMergedClaims returns the claims the previous run recorded: the
// committed manifest's (documents ai-rulez wrote whole) and the machine-local
// one's (documents shared with the user, or carrying machine-local servers). The
// local record is skipped when the run deliberately ignores local inputs.
func (g *Generator) previousMergedClaims() map[string][]jsonmerge.Claim {
	previous := map[string][]jsonmerge.Claim{}
	for rel, claims := range g.readManifest(g.manifestPath()).Merged {
		if g.trustedMergedPath(rel) {
			previous[rel] = claims
		}
	}
	if !g.localSkipped {
		for rel, claims := range g.readManifest(g.localManifestPath()).Merged {
			if g.trustedMergedPath(rel) {
				previous[rel] = claims
			}
		}
	}
	return previous
}

// trustedMergedPath reports whether a manifest may claim keys in rel. The manifest
// is a committed file anyone can edit, so a claim on a document no preset merges
// into (package.json, only.json) or one that leaves the project is ignored.
func (g *Generator) trustedMergedPath(rel string) bool {
	if g.userMode {
		// The user manifest records destinations in relocated tool homes; user scope
		// vets every entry against its layouts (userMayTouch) instead.
		return true
	}
	slashed := filepath.ToSlash(rel)
	if slashed == "" || strings.HasPrefix(slashed, "/") || slices.Contains(strings.Split(slashed, "/"), "..") ||
		!isMergedDocumentPath(mergedDocuments(), slashed) {
		g.warnOnce("Ignoring a generated-manifest claim on " + rel + ": no preset merges into that file")
		return false
	}
	return true
}

// mergedDocDeletable reports whether ai-rulez may delete the merged document at
// abs once nothing user-authored is left in it: only when the manifest recorded
// the digest of the document as ai-rulez wrote it whole and the file still has
// exactly those bytes. A forged or older manifest has no such digest, and the
// document is then left (emptied of ai-rulez's keys) instead of deleted.
func (g *Generator) mergedDocDeletable(rel, abs string) bool {
	want, ok := g.manifestDigestSet()[rel]
	if !ok {
		return false
	}
	data, err := os.ReadFile(abs)
	return err == nil && fileDigest(data) == want
}

// currentMergedClaims collects the claims of this run's outputs by manifest path.
func (g *Generator) currentMergedClaims(outputs []config.OutputFile) map[string][]jsonmerge.Claim {
	committed, local := g.splitMergedClaims(outputs)
	for rel, claims := range local {
		committed[rel] = append(committed[rel], claims...)
	}
	return committed
}

// splitMergedClaims separates the claims worth recording by where they are kept.
// A document ai-rulez wrote whole is deleted whole, but a server dropped from the
// config still has to leave it without taking a hand-written one along, so its
// claims are kept in the committed manifest (they hold names and digests only).
// Claims of a document shared with the user, of one carrying resolved secrets or
// of one the machine-local inputs change go to the machine-local manifest
// instead: the server names there can come from the local overlay, and putting a record in the
// committed manifest of every project with an MCP server would also leave a
// gitignored file behind in all of them.
func (g *Generator) splitMergedClaims(outputs []config.OutputFile,
) (committed, local map[string][]jsonmerge.Claim) {
	committed, local = map[string][]jsonmerge.Claim{}, map[string][]jsonmerge.Claim{}
	for _, output := range outputs {
		if output.IsDir || len(output.MergeClaims) == 0 {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if g.skippedPaths[rel] {
			continue
		}
		if output.PartiallyOwned || output.Sensitive || output.LocalOnly || g.plan.diverges(rel, output.MergeClaims) {
			local[rel] = append(local[rel], output.MergeClaims...)
		} else {
			committed[rel] = append(committed[rel], output.MergeClaims...)
		}
	}
	return committed, local
}

// reclaimStaleMembers clears PartiallyOwned on a merged document whose only
// content besides this run's is what the previous run recorded writing and that
// still holds the recorded value: a server just dropped from the config is still
// in the file when it is rendered, and would otherwise make a document
// ai-rulez wrote whole look hand-authored for one run. The stale entries leave
// afterwards (planUnmerge); a value the user edited does not count.
func (g *Generator) reclaimStaleMembers(outputs []config.OutputFile) {
	previous := g.previousMergedClaims()
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir || !output.PartiallyOwned || len(output.MergeClaims) == 0 {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		claims := previous[rel]
		if len(claims) == 0 {
			continue
		}
		claims = append(guardClaims(claims, output.MergeClaims), output.MergeClaims...)
		result, err := docmerge.UnmergeDocument(rel, mergedDocFormat(rel), g.finalContent(*output), claims)
		if err == nil && result.Empty {
			output.PartiallyOwned = false
		}
	}
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
			if !now.HasElements() || len(now.Path) < len(claim.Path) {
				covered = true
				break
			}
			if !claim.HasElements() {
				covered = true
				break
			}
			remaining = remaining.WithoutElements(now)
		}
		if covered || (claim.HasElements() && len(remaining.ElementDigests()) == 0) {
			continue
		}
		gone = append(gone, remaining)
	}
	return gone
}

func isPathPrefix(prefix, path []string) bool {
	return len(prefix) <= len(path) && slices.Equal(prefix, path[:len(prefix)])
}

// guardClaims holds the claims of an older record, which carry no guard, to the
// value the current claims record for the same path; one with no current
// counterpart is dropped, so what it addresses stays.
func guardClaims(claims, current []jsonmerge.Claim) []jsonmerge.Claim {
	guarded := make([]jsonmerge.Claim, 0, len(claims))
	for _, claim := range claims {
		if claim.Guarded() || claim.HasElements() {
			guarded = append(guarded, claim)
			continue
		}
		for _, now := range current {
			if now.Guarded() && slices.Equal(now.Path, claim.Path) {
				claim.Equals, claim.Sum = now.Equals, now.Sum
				guarded = append(guarded, claim)
				break
			}
		}
	}
	return guarded
}

// mergedDocFormat is the syntax of the merged document at the base-relative path
// rel: the format a builtin provider spec declares, else the one the extension
// names (a custom provider's document), else JSON, which every Go preset document
// is.
func mergedDocFormat(rel string) docmerge.Format {
	for _, doc := range providers.MergedSidecarDocs() {
		if doc.Path == rel && doc.Format != "" {
			return docmerge.Format(doc.Format)
		}
	}
	if format, ok := docmerge.FormatFromPath(rel); ok {
		return format
	}
	if strings.EqualFold(filepath.Ext(rel), ".md") {
		// The only Markdown documents with claims are the review-check files, where
		// ai-rulez owns a marker-delimited block (also at a monorepo scope's path).
		return docmerge.FormatMarkdown
	}
	return docmerge.FormatJSON
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
// claims. A document with no record falls back, on clean only, to
// presets.LegacyMergeClaims.
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
		if !g.withinScope(abs) || !pathIsFile(abs) {
			continue
		}
		if g.userMode && !g.userMayTouch(abs) {
			continue
		}
		result, err := docmerge.Unmerge(abs, mergedDocFormat(rel), claims[rel])
		if err != nil {
			g.warnUnmerge(rel, err)
			continue
		}
		for _, path := range result.Kept {
			g.warnOnce("Leaving "+strings.Join(path, ".")+" in "+rel+": it is not the value ai-rulez writes, "+
				"so it is treated as yours", "hint", "remove it by hand if you do not want it")
		}
		if result.Changed {
			remove := result.Empty
			if remove && !g.mergedDocDeletable(rel, abs) {
				g.warnOnce("Keeping " + rel + ": it is empty of ai-rulez content but ai-rulez cannot show it created the file")
				remove = false
			}
			edits = append(edits, mergedEdit{rel: rel, abs: abs, body: result.Body, delete: remove})
		}
	}
	return edits
}

// claimsToTakeBack selects, by manifest path, the claims planUnmerge removes.
func (g *Generator) claimsToTakeBack(outputs []config.OutputFile, clean bool) map[string][]jsonmerge.Claim {
	previous := g.previousMergedClaims()
	current := g.currentMergedClaims(outputs)

	claims := map[string][]jsonmerge.Claim{}
	for rel, prev := range previous {
		prev = guardClaims(prev, current[rel])
		if clean {
			claims[rel] = append(slices.Clone(prev), current[rel]...)
		} else {
			claims[rel] = subtractClaims(prev, current[rel])
		}
	}
	if !clean {
		return claims
	}
	for rel, cur := range current {
		if _, recorded := previous[rel]; !recorded {
			claims[rel] = cur
		}
	}
	for _, rel := range mergedDocuments() {
		if _, recorded := previous[rel]; recorded {
			continue
		}
		claims[rel] = append(claims[rel], g.legacyClaims(rel)...)
	}
	return claims
}

// legacyClaims are the guarded claims of the fallback for a document without a
// record (see presets.LegacyMergeClaims).
func (g *Generator) legacyClaims(rel string) []jsonmerge.Claim {
	return append(presets.LegacyMergeClaims(rel, g.config), providers.LegacyMergeClaims(rel, g.config)...)
}

// warnOnce reports a merged-document problem once per run, however many times the
// outputs are planned (a dry run, a baseline render and the real one).
func (g *Generator) warnOnce(msg string, args ...any) {
	if g.warned[msg] {
		return
	}
	if g.warned == nil {
		g.warned = map[string]bool{}
	}
	g.warned[msg] = true
	mergedWarn(msg, args...)
}

// warnUnmerge reports a document ai-rulez could not take its content out of.
func (g *Generator) warnUnmerge(rel string, err error) {
	g.warnOnce("Could not remove ai-rulez content from "+rel, "error", err)
}

// applyUnmerge writes the edits: a document with nothing user-authored left is
// deleted, any other is rewritten with its mode kept.
func (g *Generator) applyUnmerge(edits []mergedEdit) {
	for _, edit := range edits {
		if edit.delete {
			g.removeStaleFile(edit.abs)
			continue
		}
		if g.userMode && !g.userMayTouch(edit.abs) {
			continue
		}
		target, _, guardErr := g.guardWrite(edit.abs)
		if guardErr != nil {
			logger.Warn("Skipped a merged document behind a symlink that leaves the project",
				"path", edit.rel, "error", guardErr)
			continue
		}
		if err := writeFileAtomic(target, []byte(edit.body)); err != nil {
			logger.Warn("Failed to remove ai-rulez content from merged document",
				"path", edit.rel, "error", oops.Wrapf(err, "write merged document"))
			continue
		}
		logger.Debug("Removed ai-rulez content from merged document", "path", edit.rel)
	}
}

// writeFileAtomic replaces path with data through a temporary file in the same
// directory, so an interrupted write never leaves the user's file truncated, and
// keeps the file's mode.
func writeFileAtomic(path string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved // write through a symlink instead of replacing it
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".ai-rulez-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) } //nolint:errcheck // best-effort removal of our own temp file
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write error is the one reported
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
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
