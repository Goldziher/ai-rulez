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
	"github.com/samber/oops"
)

// Merged JSON documents (.claude/settings.json, .gemini/settings.json,
// opencode.json, ...) are shared with the consumer, so ai-rulez never deletes or
// regenerates them whole. What it wrote into them is recorded as claims in the
// machine-local manifest, and a claim that stops being produced, or that clean is
// asked to remove, is taken back out with docmerge.Unmerge.
//
// Trust: the committed manifest is a file anyone can edit, so it proves nothing.
// A claim, and the digest that lets a document be deleted, count only when this
// machine recorded them (the gitignored machine-local manifest); the committed
// manifest lists the paths of merged documents for teammates and nothing more. A
// document that existed before ai-rulez first wrote to it is the user's
// (markPreexistingDocuments): it is never deleted, only emptied of what ai-rulez
// itself recorded writing.
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

// mergedEdit is the result of taking ai-rulez's claims out of one document.
type mergedEdit struct {
	rel    string
	abs    string
	body   string
	delete bool // nothing user-authored remains
}

// previousMergedClaims returns the claims this machine's previous run recorded in
// the machine-local manifest. The committed manifest is attacker-editable and
// never supplies one. A run that deliberately ignores local inputs reads it too
// but takes nothing back (see claimsToTakeBack), because the record cannot tell a
// shared entry from a machine-local one.
func (g *Generator) previousMergedClaims() map[string][]jsonmerge.Claim {
	previous := map[string][]jsonmerge.Claim{}
	for rel, claims := range g.readManifest(g.localManifestPath()).Merged {
		if len(claims) > 0 && g.trustedMergedPath(rel) {
			previous[rel] = claims
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
	if g.renderedMerged[slashed] {
		return true // a document this run's own outputs merge into (a custom provider's)
	}
	if slashed == "" || strings.HasPrefix(slashed, "/") || slices.Contains(strings.Split(slashed, "/"), "..") ||
		!isMergedDocumentPath(mergedDocuments(), slashed) {
		g.warnOnce("Ignoring a generated-manifest claim on " + rel + ": no preset merges into that file")
		return false
	}
	return true
}

// mergedDocDeletable reports whether ai-rulez may delete the merged document at
// abs once nothing user-authored is left in it: only when this machine recorded
// the digest of the document as ai-rulez wrote it whole and the file still has
// exactly those bytes. A forged, committed or older record has no such digest, and
// the document is then left (emptied of ai-rulez's keys) instead of deleted.
func (g *Generator) mergedDocDeletable(rel, abs string) bool {
	want, ok := g.manifestDigestSet()[rel]
	if !ok {
		return false
	}
	data, err := g.config.ReadExisting(abs)
	return err == nil && fileDigest(data) == want
}

// currentMergedClaims collects the claims of this run's outputs by manifest path.
func (g *Generator) currentMergedClaims(outputs []config.OutputFile) map[string][]jsonmerge.Claim {
	_, local := g.splitMergedClaims(outputs)
	return local
}

// splitMergedClaims separates what is recorded where. Every claim goes to the
// machine-local manifest: it alone may license taking something back out of a
// document, and a record in the committed manifest would be both forgeable and,
// for the names of servers from the local overlay, a leak of machine-local setup.
// The committed manifest gets the path of each document ai-rulez wrote whole and
// that carries nothing machine-local, with no claims, so a teammate's guard still
// knows the document is merged and shared.
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
		fromLocal := output.Sensitive || output.LocalOnly || g.plan.diverges(rel, output.MergeClaims)
		for i := range output.MergeClaims {
			claim := output.MergeClaims[i]
			claim.Local = fromLocal
			local[rel] = append(local[rel], claim)
		}
		if !output.PartiallyOwned && !fromLocal {
			committed[rel] = []jsonmerge.Claim{}
		}
	}
	return committed, local
}

// markPreexistingDocuments makes a merged document that was already on disk the
// first time ai-rulez wrote to it the user's: absent from every manifest's file
// list, it was not created by ai-rulez, whatever its content (it may equal what
// ai-rulez renders). It is flagged PartiallyOwned, so clean never deletes it and
// only takes back what this machine recorded writing. A document ai-rulez wrote
// whole is listed, because a whole document is recorded as a generated file.
func (g *Generator) markPreexistingDocuments(outputs []config.OutputFile) {
	g.renderedMerged = map[string]bool{}
	for _, output := range outputs {
		if !output.IsDir && len(output.MergeClaims) > 0 {
			g.renderedMerged[filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))] = true
		}
	}
	if g.userMode {
		return
	}
	listed := map[string]bool{}
	for _, rel := range g.previousManifestFiles() {
		listed[filepath.ToSlash(rel)] = true
	}
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir || output.PartiallyOwned || len(output.MergeClaims) == 0 {
			continue
		}
		abs := g.absOutputPath(output.Path)
		if listed[filepath.ToSlash(g.convertToRelativePath(abs))] || !g.pathIsFile(abs) {
			continue
		}
		output.PartiallyOwned = true
	}
}

// carryClaimAnnotations copies what the previous run learned about a document's
// original shape onto this run's claims. A claim is annotated from the document as
// found when ai-rulez first merged into it (no final newline, an empty table the
// user left); from the second run on the document already holds ai-rulez's keys,
// so recomputing would forget those facts and clean could no longer restore the
// original bytes.
func (g *Generator) carryClaimAnnotations(outputs []config.OutputFile) {
	previous := g.previousMergedClaims()
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir || len(output.MergeClaims) == 0 {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		prev := previous[rel]
		if len(prev) == 0 || !g.pathIsFile(g.absOutputPath(output.Path)) {
			continue
		}
		noEOL := slices.ContainsFunc(prev, func(c jsonmerge.Claim) bool { return c.NoFinalNewline })
		claims := slices.Clone(output.MergeClaims)
		for j := range claims {
			claims[j].NoFinalNewline = claims[j].NoFinalNewline || noEOL
			for k := range prev {
				if !slices.Equal(prev[k].Path, claims[j].Path) {
					continue
				}
				if claims[j].HasElements() && !prev[k].IsPreexisting(claims[j].Path) {
					// An array that was empty when this run read it was ai-rulez's own
					// (an earlier run created it): only the first run can tell the user's
					// empty array from the one it made.
					claims[j].Preexisting = slices.DeleteFunc(slices.Clone(claims[j].Preexisting),
						func(p []string) bool { return slices.Equal(p, claims[j].Path) })
				}
				for _, empty := range prev[k].EmptyMaps {
					if !claims[j].IsEmptyMap(empty) {
						claims[j].EmptyMaps = append(claims[j].EmptyMaps, empty)
					}
				}
				for _, ancestor := range prev[k].Preexisting {
					if !claims[j].IsPreexisting(ancestor) {
						claims[j].Preexisting = append(claims[j].Preexisting, ancestor)
					}
				}
			}
		}
		output.MergeClaims = claims
	}
}

// dropUserHeldClaims removes from the claims what the document already held, as
// the user wrote it, before ai-rulez first merged into it: an array element or a
// value identical to what ai-rulez renders was never ai-rulez's to take back.
// What an earlier run claimed (a claim in prev for the same path) stays ours.
func (g *Generator) dropUserHeldClaims(outputs []config.OutputFile) {
	previous := g.previousMergedClaims()
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir || len(output.MergeClaims) == 0 {
			continue
		}
		abs := g.absOutputPath(output.Path)
		rel := filepath.ToSlash(g.convertToRelativePath(abs))
		data, err := g.config.ReadExisting(abs)
		if err != nil {
			continue
		}
		before, err := docmerge.DecodeTree(mergedDocFormat(rel), string(data))
		if err != nil || len(before) == 0 {
			continue
		}
		output.MergeClaims = withoutUserHeld(output.MergeClaims, previous[rel], before)
	}
}

// withoutUserHeld is dropUserHeldClaims for one document: before is the document
// as it stood on disk and prev what the previous run recorded for it.
func withoutUserHeld(claims, prev []jsonmerge.Claim, before map[string]any) []jsonmerge.Claim {
	kept := make([]jsonmerge.Claim, 0, len(claims))
	for i := range claims {
		claim := claims[i]
		index := slices.IndexFunc(prev, func(c jsonmerge.Claim) bool { return slices.Equal(c.Path, claim.Path) })
		value, present := jsonmerge.LookupTree(before, claim.Path)
		if !present {
			kept = append(kept, claim)
			continue
		}
		if claim.HasElements() {
			array, isArray := value.([]any)
			if !isArray {
				kept = append(kept, claim)
				continue
			}
			held := array
			if index >= 0 {
				ours := prev[index].ElementsIn(array)
				held = slices.DeleteFunc(slices.Clone(array), func(v any) bool {
					i := slices.IndexFunc(ours, func(o any) bool { return jsonmerge.Digest(o) == jsonmerge.Digest(v) })
					if i < 0 {
						return false
					}
					ours = slices.Delete(ours, i, i+1)
					return true
				})
			}
			claim = claim.WithoutElements(jsonmerge.Claim{Elements: held})
			// An empty claim on an array an earlier run created stays: the array is
			// ai-rulez's (Cursor requires it) and leaves with the document.
			createdEmpty := len(array) == 0 && index >= 0 && !prev[index].IsPreexisting(claim.Path)
			if len(claim.ElementDigests()) > 0 || createdEmpty {
				kept = append(kept, claim)
			}
			continue
		}
		if index < 0 && claim.Sum != "" && jsonmerge.Digest(value) == claim.Sum {
			continue // the user's own value, identical to the rendering
		}
		kept = append(kept, claim)
	}
	return kept
}

// reclaimStaleMembers clears PartiallyOwned on a merged document whose only
// content besides this run's is what the previous run recorded writing and that
// still holds the recorded value: a server just dropped from the config is still
// in the file when it is rendered, and would otherwise make a document
// ai-rulez wrote whole look hand-authored for one run. The stale entries leave
// afterwards (planUnmerge); a value the user edited does not count.
func (g *Generator) reclaimStaleMembers(outputs []config.OutputFile) {
	previous := g.previousMergedClaims()
	listed := map[string]bool{}
	for _, rel := range g.previousManifestFiles() {
		listed[filepath.ToSlash(rel)] = true
	}
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir || !output.PartiallyOwned || len(output.MergeClaims) == 0 {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		claims := previous[rel]
		if len(claims) == 0 || !listed[rel] {
			// Not a document ai-rulez wrote whole: what is in it besides its own
			// keys belongs to the user, recorded or not.
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
	for i := range prev {
		claim := prev[i]
		remaining := claim
		covered := false
		for j := range cur {
			now := cur[j]
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
	for i := range claims {
		claim := claims[i]
		if claim.Guarded() || claim.HasElements() {
			guarded = append(guarded, claim)
			continue
		}
		for j := range current {
			now := current[j]
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
		if !g.withinScope(abs) || !g.pathIsFile(abs) {
			continue
		}
		if g.userMode && !g.userMayTouch(abs) {
			continue
		}
		result, err := docmerge.UnmergeWith(g.config.ReadExisting, abs, mergedDocFormat(rel), claims[rel])
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
		if g.localSkipped && !clean {
			// A run that ignores the local inputs must not take back what they wrote.
			prev = slices.DeleteFunc(slices.Clone(prev), func(c jsonmerge.Claim) bool { return c.Local })
		}
		prev = guardClaims(prev, current[rel])
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
	listed := map[string]bool{}
	for _, rel := range g.previousManifestFiles() {
		listed[filepath.ToSlash(rel)] = true
	}
	for _, rel := range mergedDocuments() {
		if _, recorded := previous[rel]; recorded || !listed[rel] {
			// A document no manifest lists as written whole is the user's: the
			// fallback guesses what ai-rulez wrote, and a guess must not delete
			// a rule the user wrote themselves.
			continue
		}
		legacy := withoutPermissionClaims(g.legacyClaims(rel))
		if !clean {
			// An upgrade takes back what an older version wrote and this one no
			// longer does (Claude's servers in .claude/settings.json), and only
			// entries that are exactly that rendering.
			legacy = subtractClaims(legacy, current[rel])
		}
		if len(legacy) > 0 {
			claims[rel] = append(claims[rel], legacy...)
		}
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
	g.log().Warn(msg, args...)
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
			g.log().Warn("Skipped a merged document behind a symlink that leaves the project",
				"path", edit.rel, "error", guardErr)
			continue
		}
		if err := writeFileAtomic(target, []byte(edit.body)); err != nil {
			g.log().Warn("Failed to remove ai-rulez content from merged document",
				"path", edit.rel, "error", oops.Wrapf(err, "write merged document"))
			continue
		}
		g.log().Debug("Removed ai-rulez content from merged document", "path", edit.rel)
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

// withoutPermissionClaims drops the claims on permission rules. The fallback for a
// document with no record rests on a path the committed manifest lists, which
// proves nothing, and a deny or ask rule the user wrote must never leave on that
// evidence.
func withoutPermissionClaims(claims []jsonmerge.Claim) []jsonmerge.Claim {
	return slices.DeleteFunc(slices.Clone(claims), func(c jsonmerge.Claim) bool {
		return slices.ContainsFunc(c.Path, func(seg string) bool {
			seg = strings.ToLower(seg)
			return strings.Contains(seg, "permission") || seg == "deny" || seg == "ask" || seg == "denied"
		})
	})
}
