package generator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// localInputRels lists the project-relative machine-local inputs that exist: the
// overlay files and the local/ content tree. They are guarded like outputs, so a
// rule that un-ignores them fails the run instead of exposing their content.
func (g *Generator) localInputRels() []string {
	dir := g.config.ConfigDir
	if dir == "" {
		return nil
	}
	var rels []string
	if matches, err := filepath.Glob(filepath.Join(dir, "config.local.*")); err == nil {
		for _, m := range matches {
			if rel := filepath.ToSlash(g.convertToRelativePath(m)); rel != "" {
				rels = append(rels, rel)
			}
		}
	}
	if info, err := os.Stat(filepath.Join(dir, localSourceDirName)); err == nil && info.IsDir() {
		if rel := filepath.ToSlash(g.convertToRelativePath(filepath.Join(dir, localSourceDirName))); rel != "" {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	return rels
}

// ignoreLocalInputs makes sure the overlay file, its temp files, the local/
// content tree and the local manifest are git-ignored. It runs before any check
// that can refuse the run.
func (g *Generator) ignoreLocalInputs() error {
	patterns := g.localInputPatterns()
	if len(patterns) == 0 {
		return nil
	}
	// The overlay's lock and temp files come and go: ignore them before they exist.
	patterns = append(patterns, g.configDirName()+"/.config.local.*")
	if err := gitignore.EnsureEntries(g.log(), g.config.BaseDir, patterns); err != nil {
		return oops.Wrapf(err, "gitignore the machine-local inputs")
	}
	return nil
}

// localInputPatterns lists the ignore patterns of the machine-local inputs: the
// local/ content tree, the local manifest, the overlay and whichever of its lock
// and temp files exist. It is empty when the project has no local inputs.
func (g *Generator) localInputPatterns() []string {
	if !g.hasLocalGitignoreTargets() {
		return nil
	}
	patterns := []string{
		g.configDirName() + "/" + localSourceDirName + "/",
		g.configDirName() + "/config.local.*",
	}
	if rel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); rel != "" {
		patterns = append(patterns, rel)
	}
	return append(patterns, g.localGitignorePatternsOnDisk()...)
}

// hasLocalGitignoreTargets reports whether machine-local content exists and so
// requires unconditional gitignore entries (the ".local" outputs plus the
// .ai-rulez/local/ source subtree). The local manifest counts: besides local
// outputs it records what ai-rulez merged into hand-authored documents.
func (g *Generator) hasLocalGitignoreTargets() bool {
	return g.config.HasLocalInputs() || len(g.localGitignorePatternsOnDisk()) > 0 || g.localManifestPending
}

// localGitignorePatternsOnDisk lists the machine-local ignore patterns for the
// overlay file, its lock/temp files and the local/ content tree that exist on
// disk. It checks the filesystem rather than the loaded config, so a run that
// skipped local inputs (plugin mode, --no-local) still keeps them ignored.
func (g *Generator) localGitignorePatternsOnDisk() []string {
	dir := g.config.ConfigDir
	if dir == "" {
		return nil
	}
	prefix := g.configDirName() + "/"
	var patterns []string
	for _, p := range []string{"config.local.*", ".config.local.*"} {
		if matches, err := filepath.Glob(filepath.Join(dir, p)); err == nil && len(matches) > 0 {
			patterns = append(patterns, prefix+p)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, localSourceDirName)); err == nil && info.IsDir() {
		patterns = append(patterns, prefix+localSourceDirName+"/")
	}
	return patterns
}

// collectGitignorePaths collects unique patterns to add to .gitignore.
//
// Committed output patterns are only added when config gitignore management is
// enabled. Machine-local ".local" outputs and the .ai-rulez/local/ source
// subtree are added UNCONDITIONALLY (even when gitignore is disabled) because
// committing them would leak machine-local configuration.
func (g *Generator) collectGitignorePaths(outputs []config.OutputFile) map[string]bool {
	paths := make(map[string]bool)
	includeCommitted := g.config.ShouldUpdateGitignore()
	committed := g.committedOutputPaths(outputs)
	for _, output := range outputs {
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if output.LocalOnly {
			if pattern := g.localOutputPattern(relPath); pattern != "" {
				paths[pattern] = true
			}
			continue
		}
		if !includeCommitted {
			continue
		}
		if pattern := g.committedOutputPattern(output, relPath, committed); pattern != "" {
			paths[pattern] = true
		}
	}
	g.addLocalSourcePatterns(paths, includeCommitted)
	if includeCommitted {
		g.addManifestPatterns(paths)
	}

	return paths
}

// committedOutputPattern is the .gitignore pattern for a generated output that
// is not machine-local, or "" when the output must not be ignored.
func (g *Generator) committedOutputPattern(output config.OutputFile, relPath string, committed []string) string {
	// A hand-written file the overwrite guard skipped is the user's, not ours.
	if g.skippedPaths[relPath] {
		return ""
	}
	// A partially owned settings document is hand-authored and tracked apart
	// from the one key ai-rulez writes into it; telling git to ignore it
	// would hide the user's own file (#185).
	if output.PartiallyOwned {
		return ""
	}
	// Check outputs are read by hosted reviewers from the committed tree.
	if output.Committed {
		return ""
	}
	if g.shouldSkipPath(relPath) {
		return ""
	}
	pattern := gitignorePatternForOutput(g.config.RulesDirs, relPath, output.IsDir)
	// A directory pattern that would swallow a committed output (the factory
	// skills dir holding the review-guidelines check) is narrowed to the
	// generated files themselves, since git cannot re-include under it.
	if pattern != "" && strings.HasSuffix(pattern, "/") && coversCommitted(pattern, committed) {
		if output.IsDir {
			return ""
		}
		return relPath
	}
	return pattern
}

// addLocalSourcePatterns adds the patterns for machine-local inputs to paths:
// the .ai-rulez/local/ source subtree and the outputs an earlier run recorded.
func (g *Generator) addLocalSourcePatterns(paths map[string]bool, includeCommitted bool) {
	// The .ai-rulez/local/ source subtree holds machine-local override content
	// and must never be committed. Ignore it unconditionally, bypassing the
	// config-dir skip that normally protects .ai-rulez/.
	for _, p := range g.localInputPatterns() {
		paths[p] = true
	}
	// With managed ignores on, the directory is covered even before it exists:
	// `telemetry record` creates files in it between two generates, and they
	// must not show up as untracked.
	if includeCommitted && !g.userMode {
		paths[g.configDirName()+"/"+localSourceDirName+"/"] = true
	}

	// A run that skipped the local inputs on purpose (--no-local) writes no
	// local outputs, but the ones an earlier run left are still there and must
	// stay ignored.
	if g.localSkipped {
		for _, rel := range g.readManifest(g.localManifestPath()).Files {
			if pattern := g.skippedLocalPattern(rel); pattern != "" {
				paths[pattern] = true
			}
		}
	}
}

// addManifestPatterns adds the generated manifest and the machine-local record
// to paths. The manifest sits inside the config dir and is rewritten on every
// `generate`; including it explicitly lets the managed fence cover it.
func (g *Generator) addManifestPatterns(paths map[string]bool) {
	if manifestRel := filepath.ToSlash(g.convertToRelativePath(g.manifestPath())); manifestRel != "" {
		paths[manifestRel] = true
	}
	// The machine-local record holds this machine's claims and digests.
	if localRel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); localRel != "" &&
		(g.localManifestPending || pathIsFile(g.localManifestPath())) {
		paths[localRel] = true
	}
}

// committedOutputPaths lists the relative paths of outputs that must stay tracked.
func (g *Generator) committedOutputPaths(outputs []config.OutputFile) []string {
	var paths []string
	for _, output := range outputs {
		if output.Committed && !output.IsDir {
			paths = append(paths, filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path))))
		}
	}
	return paths
}

// coversCommitted reports whether a directory pattern contains a committed path.
func coversCommitted(dirPattern string, committed []string) bool {
	dir := strings.TrimPrefix(dirPattern, "./")
	for _, rel := range committed {
		if strings.HasPrefix(rel, dir) || strings.Contains(rel, "/"+dir) {
			return true
		}
	}
	return false
}

// skippedLocalPattern is the managed-.gitignore pattern for a machine-local file
// an earlier run recorded, for a run that did not render local inputs. Files
// named after local content are excluded per clone and stay in .git/info/exclude,
// which such a run leaves alone; only outside a repository do they need the block.
func (g *Generator) skippedLocalPattern(rel string) string {
	if stableLocalName(rel) || g.git().InfoExcludePathContext(g.ctx, g.config.BaseDir) == "" {
		return localGitignorePattern(g.config.RulesDirs, rel)
	}
	return ""
}

// localGitignorePattern maps a machine-local output to its ignore pattern. Local
// rule files share a rules folder with committed rules, so they get the stable
// "<rulesdir>/*.local.*" pattern instead of one entry per file; that keeps the
// block identical for teammates and covers rules added later.
func localGitignorePattern(rd *config.RulesDirSet, relPath string) string {
	if rd.In(relPath) {
		return relPath[:strings.LastIndex(relPath, "/")] + "/*.local.*"
	}
	return relPath
}

// convertToRelativePath converts an absolute path to relative, or returns the original path
func (g *Generator) convertToRelativePath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	relPath, err := g.relativeToBase(path)
	if err != nil {
		// Never the base name alone: that would name an unrelated file below the
		// base directory. The whole path cannot match one either.
		return path
	}
	return relPath
}

// relativeToBase makes abs relative to the base directory, failing when no
// relative form exists (a different volume on Windows).
func (g *Generator) relativeToBase(abs string) (string, error) {
	rel, err := filepath.Rel(g.config.BaseDir, abs)
	if err != nil {
		return "", oops.With("path", abs).Wrapf(err, "express the path relative to %s", g.config.BaseDir)
	}
	return rel, nil
}

// shouldSkipPath checks if a path should be skipped for .gitignore
func (g *Generator) shouldSkipPath(relPath string) bool {
	configDir := g.configDirName()
	return relPath == configDir ||
		hasPrefix(relPath, configDir+"/") ||
		hasPrefix(relPath, configDir+"\\")
}

func (g *Generator) configDirName() string {
	if g.config.ConfigDirName != "" {
		return g.config.ConfigDirName
	}
	return ".ai-rulez"
}

// updateGitignore updates .gitignore with generated file paths using a fenced block
func (g *Generator) updateGitignore(outputs []config.OutputFile) error {
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")

	// Patterns git does not already cover: not ignored by a user rule, and not
	// deliberately un-ignored by one.
	paths, overridden := g.neededGitignorePatterns(outputs)
	for _, o := range overridden {
		g.log().Warn("A .gitignore rule un-ignores a machine-local or secret output; ai-rulez will not re-ignore it",
			"path", o.Pattern, "rule", o.Rule, "source", o.Source)
	}

	// Read existing .gitignore content
	existingData, err := gitutil.ReadIgnoreFileOrEmpty(g.log(), gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "read .gitignore")
	}
	existingContent := string(existingData)

	// Git does not read a symlinked .gitignore, and writing through the link would
	// change a file that lives elsewhere: keep every entry in .git/info/exclude.
	// The link's patterns protect nothing, so none may be dropped as user-covered.
	if gitignore.IsSymlink(g.config.BaseDir) {
		return gitignore.ReplaceViaExclude(g.log(), g.config.BaseDir, paths) //nolint:wrapcheck // already contextual
	}

	sortedPaths := dropUserPatterns(paths, existingContent)

	if len(sortedPaths) == 0 {
		g.log().Debug("No paths to add to .gitignore")
		return g.dropGitignoreBlock(gitignorePath, existingContent)
	}

	newContent := gitignoreWithBlock(existingContent, sortedPaths)

	gitignorePath, _, err = g.guardWrite(gitignorePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil { //nolint:gosec // path from config, not user input
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "write .gitignore")
	}

	g.log().Debug("Updated .gitignore", "entries", len(sortedPaths))
	return nil
}

// dropGitignoreBlock removes the managed block an earlier run left in
// existingContent, rather than leaving an empty fence behind when nothing is
// left to add.
func (g *Generator) dropGitignoreBlock(gitignorePath, existingContent string) error {
	if !contains(existingContent, gitignore.BeginMarker) && !contains(existingContent, gitignore.OldHeader) {
		return nil
	}
	safePath, _, guardErr := g.guardWrite(gitignorePath)
	if guardErr != nil {
		return guardErr
	}
	return dropManagedBlock(safePath, existingContent)
}

// gitignoreWithBlock returns existingContent with the fenced block listing paths
// replacing an earlier block, or appended when there is none.
func gitignoreWithBlock(existingContent string, paths []string) string {
	var fencedBlock strings.Builder
	fencedBlock.WriteString(gitignore.BeginMarker + "\n")
	for _, p := range paths {
		fencedBlock.WriteString(p + "\n")
	}
	fencedBlock.WriteString(gitignore.EndMarker + "\n")

	switch {
	case contains(existingContent, gitignore.BeginMarker):
		return gitignore.ReplaceFencedBlock(existingContent, fencedBlock.String())
	case contains(existingContent, gitignore.OldHeader):
		return gitignore.ReplaceOldHeaderBlock(existingContent, fencedBlock.String())
	case existingContent == "":
		return fencedBlock.String()
	}
	newContent := existingContent
	if !hasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	return newContent + "\n" + fencedBlock.String()
}

// dropUserPatterns drops entries the user already has outside the managed fence,
// which avoids duplicating lines like ".cursor/" that pre-existed in the file.
func dropUserPatterns(paths []string, existingContent string) []string {
	outside := gitignore.PatternsOutsideFence(existingContent)
	outsidePatterns := make([]string, 0, len(outside))
	for pattern := range outside {
		outsidePatterns = append(outsidePatterns, pattern)
	}
	var kept []string
	for _, p := range paths {
		if !isIgnored(p, outsidePatterns) {
			kept = append(kept, p)
		}
	}
	return kept
}
