package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers" // Register DSL-backed preset generators (overrides legacy registrations where they overlap)
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles" // Register remaining legacy preset generators
)

// keepForRole marks the previously generated files a role run must not clean as
// still wanted. A role renders one person's slice, not the project, and the
// committed OKF bundle documents the whole project.
func (g *Generator) keepForRole(previous []string, next map[string]bool) {
	if g.role == nil && !rulefiles.InScope(g.config) {
		return
	}
	if g.role != nil && g.config.OKFEnabled() {
		if dir := strings.Trim(filepath.ToSlash(g.config.OKFDir()), "/"); dir != "" && dir != "." {
			for _, rel := range previous {
				if strings.HasPrefix(rel, dir+"/") {
					next[rel] = true
				}
			}
		}
	}
	// The llms.txt files document the whole project and a role or scope run
	// does not write them; keeping them out of `next` would delete them.
	llmsDir := strings.Trim(filepath.ToSlash(g.config.LLMsTxtDir()), "/")
	for _, name := range []string{presets.LLMsTxtFileName, presets.LLMsTxtFullFileName} {
		rel := name
		if llmsDir != "" && llmsDir != "." {
			rel = llmsDir + "/" + name
		}
		if slices.Contains(previous, rel) {
			next[rel] = true
		}
	}
}

// staleManifestFiles returns the absolute paths the previous manifest lists that
// this run no longer generates and that are provably ai-rulez's to delete.
func (g *Generator) staleManifestFiles(outputs []config.OutputFile) []string {
	previous := g.previousManifestFiles()
	if len(previous) == 0 {
		return nil
	}

	next := g.currentManifestSet(outputs)

	// Never delete a merged settings document from a manifest entry. The manifest
	// holds bare paths with no record of what the document contained, and one
	// written by an older ai-rulez lists these files even when the consumer
	// hand-authored them — so the flag on OutputFile cannot be consulted here.
	// Leaving a wholly generated .mcp.json behind is the cost; the alternative
	// deletes a tracked file full of the user's own settings (#185).
	//
	// Both sources are needed: provider sidecar specs cover .claude/settings.json,
	// .mcp.json and .amp/settings.json, while .gemini/settings.json and
	// .agents/settings.json are merged by preset generators that have no spec.
	// Those two are also the ones emitted only when MCP servers are declared, so
	// they are exactly the paths a render omits while the manifest still lists them.
	merged := append(providers.MergedSidecarPaths(), presets.MergedDocumentPaths()...)

	// The gitignored local manifest lists only whole files ai-rulez wrote from
	// machine-local inputs (never a partially owned document), so it may remove
	// a merged document too: a secret-bearing .mcp.json must not outlive the
	// overlay that produced it.
	local := g.localManifestSet()

	g.keepForRole(previous, next)

	digests := g.manifestDigestSet()
	var matcher *outputMatcher
	matcherFor := func() *outputMatcher {
		if matcher == nil {
			matcher = g.outputMatcher(outputs)
		}
		return matcher
	}
	var stale, unknown, unverified []string
	for _, relPath := range previous {
		if next[relPath] || (isMergedDocumentPath(merged, relPath) && !local[relPath]) {
			continue
		}
		absPath := filepath.Join(g.config.BaseDir, filepath.FromSlash(relPath))
		switch g.staleVerdictFor(relPath, absPath, digests, matcherFor) {
		case staleRemove:
			stale = append(stale, absPath)
		case staleUnknown:
			unknown = append(unknown, relPath)
		case staleUnverified:
			unverified = append(unverified, relPath)
		case staleKeep:
		}
	}
	sort.Strings(unknown)
	sort.Strings(unverified)
	g.warnStaleKept(unknown, unverified)
	sort.Strings(stale)
	return stale
}

// currentManifestSet is the set of relative paths this run generates, plus the
// ones only the shared baseline produces, which are never this machine's to delete.
func (g *Generator) currentManifestSet(outputs []config.OutputFile) map[string]bool {
	next := make(map[string]bool)
	if g.plan != nil {
		for _, rel := range g.plan.suppressed {
			next[rel] = true
		}
	}
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		next[filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))] = true
	}
	return next
}

// staleVerdict is what staleManifestFiles decides for one manifest entry.
type staleVerdict int

const (
	// staleKeep leaves the entry alone.
	staleKeep staleVerdict = iota
	// staleRemove marks the entry as a generated file to delete.
	staleRemove
	// staleUnknown keeps an entry no preset writes at its path.
	staleUnknown
	// staleUnverified keeps an entry nothing proves ai-rulez generated.
	staleUnverified
)

// staleVerdictFor decides what to do with the manifest entry relPath at absPath.
// matcherFor returns the output matcher, built on first use.
func (g *Generator) staleVerdictFor(relPath, absPath string, digests map[string]string, matcherFor func() *outputMatcher) staleVerdict {
	if !g.withinScope(absPath) {
		g.log().Warn("Skipping generated manifest path outside project", "path", relPath)
		return staleKeep
	}
	if g.userMode && !g.userManifestEntryOK(relPath, absPath) {
		return staleKeep
	}
	if !g.removalConfined(absPath) {
		g.warnOnce("Stale file not removed: " + relPath + " is behind a symlink that leaves the project")
		return staleKeep
	}
	if _, err := g.config.StatExisting(absPath); err != nil {
		return staleKeep
	}
	if !g.userMode && !matcherFor().matches(relPath) {
		return staleUnknown
	}
	if !g.provablyGenerated(relPath, absPath, digests) {
		return staleUnverified
	}
	// A rules folder is shared with hand-written rules: delete only a file that
	// still looks generated, even when a manifest lists it.
	if g.config.InRulesDir(relPath) && !g.looksGenerated(absPath) {
		g.log().Debug("Keeping manifest-listed rule file without a generated marker", "path", relPath)
		return staleKeep
	}
	return staleRemove
}

// warnStaleKept explains the stale entries that were left in place. An upgrade
// can meet dozens of leftovers an older version listed: one line per reason,
// naming the files, rather than one warning per file.
func (g *Generator) warnStaleKept(unknown, unverified []string) {
	if len(unknown) > 0 {
		g.warnOnce(fmt.Sprintf("Stale files not removed: %d file(s) the previous manifest lists are not at a path any preset writes",
			len(unknown)), "files", unknown, "hint", "delete them by hand if they are no longer needed")
	}
	if len(unverified) > 0 {
		g.warnOnce(fmt.Sprintf("Stale files not removed: cannot verify that ai-rulez generated %d file(s) the previous manifest lists",
			len(unverified)), "files", unverified,
			"hint", "they carry no Content-Hash and no digest was recorded for them; delete them by hand if they are no longer needed")
	}
}

// localManifestSet is the set of paths the machine-local manifest authorizes
// deleting; empty when the run deliberately ignores local inputs.
func (g *Generator) localManifestSet() map[string]bool {
	set := map[string]bool{}
	if g.localSkipped {
		return set
	}
	for _, f := range g.readManifest(g.localManifestPath()).Files {
		set[f] = true
	}
	return set
}

// looksGenerated reports whether the file at absPath, read from the project's
// workspace, carries stored hashes or a generated banner.
func (g *Generator) looksGenerated(absPath string) bool {
	if contentHash, _, _ := g.scanHashes(absPath); contentHash != "" {
		return true
	}
	data, err := g.config.ReadExisting(absPath)
	return err == nil && hasGeneratedBanner(absPath, data)
}

// pathIsFile is pathIsFile on the project's workspace.
func (g *Generator) pathIsFile(path string) bool {
	info, err := g.config.StatExisting(path)
	return err == nil && !info.IsDir()
}

// isMergedDocumentPath reports whether relPath names one of the merged settings
// documents. The registries hold paths relative to a config's own base dir
// (".mcp.json"), while a manifest entry is relative to the root config, so a
// scope's document appears as "packages/api/.mcp.json". Matching on the tail --
// the same rule presets.isRegisteredMergedDocument applies -- is what keeps a
// scoped document out of the stale set; an exact match protected only the root
// one and deleted every scope's, which is the #185 data loss this guard exists
// to prevent.
func isMergedDocumentPath(merged []string, relPath string) bool {
	slashed := filepath.ToSlash(relPath)
	for _, candidate := range merged {
		if slashed == candidate || strings.HasSuffix(slashed, "/"+candidate) {
			return true
		}
	}
	return false
}

func isUnderBaseDir(baseDir, path string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func (g *Generator) removeStaleManifestFiles(files []string) {
	for _, file := range files {
		g.removeStaleFile(file)
	}
}

// removalConfined reports whether removing path stays inside the project: the
// directory holding it, with every symlink resolved, must lie under a root the run
// may write to, exactly as guardWrite requires of a write. Without it a generated
// folder replaced by a link (.cursor/commands -> ../shared) would carry a removal
// out of the checkout. User scope vets its paths with userMayTouch instead.
func (g *Generator) removalConfined(path string) bool {
	if g.userMode {
		return true
	}
	resolved, _, err := resolveWriteTarget(filepath.Dir(filepath.Clean(path)), new(int))
	if err != nil {
		return false
	}
	for _, root := range g.writeRoots() {
		realRoot, _, rerr := resolveWriteTarget(root, new(int))
		if rerr != nil {
			realRoot = root
		}
		if isUnderBaseDir(realRoot, resolved) {
			return true
		}
	}
	return false
}

// removeStaleFile removes a single stale file.
func (g *Generator) removeStaleFile(filePath string) {
	// In user scope a file is removed only when the directory holding it still
	// resolves inside the home directory: a symlinked folder must not carry the
	// removal out of it.
	if g.userMode && filePath != g.manifestPath() && filePath != g.localManifestPath() &&
		!g.userMayTouch(filepath.Dir(filePath)) {
		return
	}
	if !g.userMode && filePath != g.manifestPath() && filePath != g.localManifestPath() && isSymlink(filePath) {
		g.log().Warn("Not removing a symlink: links are the user's to remove", "path", filePath)
		return
	}
	if !g.removalConfined(filePath) {
		g.log().Warn("Not removing a generated file behind a symlink that leaves the project", "path", filePath)
		return
	}
	if err := os.Remove(filePath); err != nil {
		g.log().Warn("Failed to remove stale file", "path", filePath, "error", err)
	} else {
		g.log().Debug("Removed stale file", "path", filePath)
	}
}
