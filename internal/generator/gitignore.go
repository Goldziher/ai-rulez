package generator

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// gitignoreFileName is the ignore file ai-rulez keeps its managed block in.
const gitignoreFileName = ".gitignore"

// localInputRels lists the project-relative machine-local inputs that exist: the
// overlay files and the local/ content tree. They are guarded like outputs, so a
// rule that un-ignores them fails the run instead of exposing their content.
func (g *Generator) localInputRels() []string {
	dir := g.config.ConfigDir
	if dir == "" {
		return nil
	}
	var rels []string
	if matches, err := filepath.Glob(filepath.Join(dir, g.localOverlayGlob())); err == nil {
		for _, m := range matches {
			if rel := filepath.ToSlash(g.convertToRelativePath(m)); rel != "" {
				rels = append(rels, rel)
			}
		}
	}
	if info, err := os.Stat(filepath.Join(dir, localSourceDirName)); err == nil && info.IsDir() && !g.config.Contentless() {
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
	patterns = append(patterns, g.localLockGlob())
	if g.gitignoreSink != nil {
		if err := g.gitignoreSink.Add(g.log(), g.gitignorePrefix, patterns); err != nil {
			return oops.Wrapf(err, "gitignore the machine-local inputs")
		}
		return nil
	}
	// Roots of a recursive run share their repository's .gitignore: the read and
	// the rewrite are one critical section, or a write drops another root's
	// entries and, on Windows, a read fails with a sharing violation.
	unlock := g.excludeLocks.lock(gitignore.EnsureTarget(g.config.BaseDir))
	defer unlock()
	if err := gitignore.EnsureEntries(g.log(), g.config.BaseDir, patterns); err != nil {
		return oops.Wrapf(err, "gitignore the machine-local inputs")
	}
	return nil
}

// localInputPatterns lists the ignore patterns of the machine-local inputs: the
// local/ content tree, the local manifest, the overlay and whichever of its lock
// and temp files exist. It is empty when the project has no local inputs.
func (g *Generator) localInputPatterns() []string {
	if !g.hasLocalInputTargets() {
		return nil
	}
	patterns := []string{
		g.configDirName() + "/" + localSourceDirName + "/",
		g.configDirName() + "/config.local.*",
	}
	if g.config.Contentless() {
		patterns = []string{g.localOverlayGlob()}
	}
	if rel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); rel != "" {
		patterns = append(patterns, rel)
	}
	return append(patterns, g.localGitignorePatternsOnDisk()...)
}

// fixedLocalPatterns lists the machine-local ignore patterns a managed block
// always carries: the local/ content tree, the overlay and its lock and temp
// files, and the machine-local manifest. They depend on the layout alone.
func (g *Generator) fixedLocalPatterns() []string {
	var patterns []string
	if g.config.Contentless() {
		patterns = []string{g.localOverlayGlob(), g.localLockGlob()}
	} else {
		dir := g.configDirName()
		patterns = []string{dir + "/" + localSourceDirName + "/", dir + "/" + g.localOverlayGlob(), g.localLockGlob()}
	}
	if rel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); rel != "" {
		patterns = append(patterns, rel)
	}
	return patterns
}

// hasLocalGitignoreTargets reports whether machine-local content exists and so
// requires unconditional gitignore entries (the ".local" outputs plus the
// .ai-rulez/local/ source subtree). The local manifest counts: besides local
// outputs it records what ai-rulez merged into hand-authored documents.
func (g *Generator) hasLocalGitignoreTargets() bool {
	return g.hasLocalInputTargets() || g.localManifestIgnorable()
}

// hasLocalInputTargets is hasLocalGitignoreTargets without a manifest that only
// sits on disk: output ownership claims and digests imply no overlay or local/ tree. ~keep
func (g *Generator) hasLocalInputTargets() bool {
	return g.config.HasLocalInputs() || len(g.localGitignorePatternsOnDisk()) > 0
}

// manifestExcluded reports whether the machine-local manifest is the only
// machine-local thing to ignore and the project does not manage a .gitignore
// block: it then goes to .git/info/exclude, so a project that sets
// gitignore = false never has its tracked .gitignore written (#497).
func (g *Generator) manifestExcluded() bool {
	return !g.userMode && !g.config.ShouldUpdateGitignore() && !g.hasLocalInputTargets() &&
		g.git().InfoExcludePathContext(g.ctx, g.config.BaseDir) != ""
}

// localManifestIgnorable reports whether the machine-local manifest is written
// this run or already on disk and so needs a .gitignore entry. Either way it
// holds this machine's claims and digests and is ignored, whatever `gitignore`
// says; with gitignore off and nothing else local, .git/info/exclude holds it
// instead (see manifestExcluded). A manifest that only sits on disk is ignored
// inside a repository only: elsewhere there is nothing to stage, and a
// .gitignore would be noise.
func (g *Generator) localManifestIgnorable() bool {
	if g.manifestExcluded() {
		return false
	}
	return g.localManifestPending ||
		(pathIsFile(g.localManifestPath()) && g.git().IsRepoContext(g.ctx, g.config.BaseDir))
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
	overlayNames := []string{"config.local.*", ".config.local.*"}
	if g.config.Contentless() {
		// The project root is not a config directory: its patterns carry no prefix.
		prefix, overlayNames = "", []string{g.localOverlayGlob(), ".ai-rulez-local.*"}
	}
	var patterns []string
	for _, p := range overlayNames {
		if matches, err := filepath.Glob(filepath.Join(dir, p)); err == nil && len(matches) > 0 {
			patterns = append(patterns, prefix+p)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, localSourceDirName)); err == nil && info.IsDir() && !g.config.Contentless() {
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
			if pattern := g.localOutputPattern(relPath, output.IsDir); pattern != "" {
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
		if len(g.config.GitignoreExclude) > 0 {
			g.dropExcludedPatterns(paths, g.protectedGitignorePatterns(outputs))
		}
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
	// The manifest alone does not make .ai-rulez/local/ or the overlay targets.
	if rel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); rel != "" && g.localManifestIgnorable() {
		paths[rel] = true
	}
	// With managed ignores on, the machine-local inputs are listed whether or not
	// they exist: the committed block must not depend on which local files one
	// machine has, or on whether this run loaded them (--no-local). The local
	// directory is covered before it exists too: `telemetry record` creates
	// files in it between two generates.
	if includeCommitted && !g.userMode {
		for _, p := range g.fixedLocalPatterns() {
			paths[p] = true
		}
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
	if g.git().InfoExcludePathContext(g.ctx, g.config.BaseDir) == "" {
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
	if g.config.Contentless() {
		// The project root is the config directory: only the config file itself is off limits.
		return relPath == config.RootConfigFilename
	}
	configDir := g.configDirName()
	return relPath == configDir ||
		strings.HasPrefix(relPath, configDir+"/") ||
		strings.HasPrefix(relPath, configDir+"\\")
}

// localOverlayGlob is the glob (relative to the config directory) that matches
// the machine-local overlay file.
func (g *Generator) localOverlayGlob() string {
	if g.config.Contentless() {
		return "ai-rulez.local.*"
	}
	return "config.local.*"
}

// localLockGlob is the ignore pattern of the overlay's lock and temp files.
func (g *Generator) localLockGlob() string {
	if g.config.Contentless() {
		return ".ai-rulez-local.*"
	}
	return g.configDirName() + "/.config.local.*"
}

func (g *Generator) configDirName() string {
	if g.config.ConfigDirName != "" {
		return g.config.ConfigDirName
	}
	return ".ai-rulez"
}

// updateGitignore updates .gitignore with generated file paths using a fenced block
func (g *Generator) updateGitignore(outputs []config.OutputFile) error {
	// A recursive run collects every root's patterns into one block at the
	// repository toplevel instead of writing a .gitignore beside each config.
	if g.gitignoreSink != nil {
		return g.collectToGitignoreSink(outputs)
	}
	gitignorePath := filepath.Join(g.config.BaseDir, gitignoreFileName)
	unlock := g.excludeLocks.lock(gitignore.WriteTarget(g.config.BaseDir))
	defer unlock()

	// Patterns git does not already cover: not ignored by a user rule, and not
	// deliberately un-ignored by one.
	paths, overridden := g.neededGitignorePatterns(outputs)
	g.warnUnignoredProtected(overridden)

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
	warnTrackedByBlock(g.warnOnce, trackedBlockWarnings(g.ctx, g.git(), g.config.BaseDir, newContent))
	return nil
}

// gitignoreDriftNote names the managed block in a drift report.
const gitignoreDriftNote = "managed block"

// gitignoreDrift is updateGitignore without the write: it reports the managed
// block as drifted when a generate run would add, change or remove it. A
// recursive run only collects this root's patterns into the shared sink, which
// judges the one toplevel block once every root is in (GitignoreSink.Drift).
func (g *Generator) gitignoreDrift(outputs []config.OutputFile) (Drift, bool) {
	if g.userMode {
		return Drift{}, false
	}
	managed := g.config.ShouldUpdateGitignore() || g.hasLocalGitignoreTargets()
	if g.gitignoreSink != nil {
		if managed {
			paths, _ := g.neededGitignorePatterns(outputs)
			g.gitignoreSink.Collect(g.gitignorePrefix, paths)
		}
		return Drift{}, false
	}
	if gitignore.IsSymlink(g.config.BaseDir) {
		// Git does not read it, and generate writes the exclude file instead.
		var paths []string
		if managed {
			paths, _ = g.neededGitignorePatterns(outputs)
		}
		return excludeBlockDrift(g.config.BaseDir, paths)
	}
	existing, err := gitutil.ReadIgnoreFileOrEmpty(g.log(), filepath.Join(g.config.BaseDir, gitignoreFileName))
	if err != nil && !os.IsNotExist(err) {
		return Drift{}, false
	}
	var paths []string
	if managed {
		needed, _ := g.neededGitignorePatterns(outputs)
		paths = dropUserPatterns(needed, string(existing))
	}
	return managedBlockDrift(gitignoreFileName, string(existing), paths)
}

// excludeFileName is the file that stands in for a symlinked .gitignore.
const excludeFileName = ".git/info/exclude"

// excludeBlockDrift judges the project's block in .git/info/exclude, which
// generate keeps instead of a symlinked .gitignore, against paths.
func excludeBlockDrift(baseDir string, paths []string) (Drift, bool) {
	hasBlock, exact, inRepo := gitignore.ExcludeBlockStatus(baseDir, paths)
	switch {
	case !inRepo || exact:
		return Drift{}, false
	case !hasBlock:
		return Drift{Path: excludeFileName, Kind: DriftMissing, Note: gitignoreDriftNote}, true
	}
	return Drift{Path: excludeFileName, Kind: DriftStale, Note: gitignoreDriftNote}, true
}

// managedBlockDrift compares content with the managed block holding paths. No
// paths means the block should not be there; a block that differs from paths in
// either direction (a missing entry, or one generate would remove), or none at
// all, is drift.
func managedBlockDrift(rel, content string, paths []string) (Drift, bool) {
	hasBlock := strings.Contains(content, gitignore.BeginMarker) || strings.Contains(content, gitignore.OldHeader)
	if len(paths) == 0 {
		if !hasBlock {
			return Drift{}, false
		}
		return Drift{Path: rel, Kind: DriftStale, Note: gitignoreDriftNote}, true
	}
	if !hasBlock {
		return Drift{Path: rel, Kind: DriftMissing, Note: gitignoreDriftNote}, true
	}
	if strings.Contains(content, gitignore.BeginMarker) && blockEquals(content, paths) {
		return Drift{}, false
	}
	return Drift{Path: rel, Kind: DriftStale, Note: gitignoreDriftNote}, true
}

// blockEquals reports whether the managed block of content lists exactly paths.
// generate rewrites the whole block, so a line it would drop is drift too.
func blockEquals(content string, paths []string) bool {
	inBlock := map[string]bool{}
	in := false
	for _, line := range strings.Split(content, "\n") {
		switch trimmed := strings.TrimSpace(line); trimmed {
		case gitignore.BeginMarker:
			in = true
		case gitignore.EndMarker:
			in = false
		default:
			if in && trimmed != "" {
				inBlock[trimmed] = true
			}
		}
	}
	want := make(map[string]bool, len(paths))
	for _, p := range paths {
		want[p] = true
	}
	return maps.Equal(inBlock, want)
}

// warnUnignoredProtected reports the protected patterns a user rule
// deliberately un-ignores: they stay out of the block, but the user should know
// the machine-local or secret output is now committable.
func (g *Generator) warnUnignoredProtected(overridden []unignoredProtected) {
	for _, o := range overridden {
		if !o.Protected {
			// A negation the user wrote on purpose; naming the pattern in
			// gitignore_exclude says the same and silences this.
			g.warnOnce("A .gitignore rule un-ignores "+o.Pattern+", which the managed block would list; ai-rulez leaves it out",
				"rule", o.Rule, "at", fmt.Sprintf("%s:%d", o.Source, o.Line),
				"fix", fmt.Sprintf("add %q to gitignore_exclude to commit it on purpose", o.Pattern))
			continue
		}
		g.log().Warn("A .gitignore rule un-ignores a machine-local or secret output; ai-rulez will not re-ignore it",
			"path", o.Pattern, "rule", o.Rule, "source", o.Source)
	}
}

// collectToGitignoreSink adds this root's needed patterns to the shared
// recursive-run block. It does not write a .gitignore beside the config, so a
// nested root that had none gains no file; the sink writes the toplevel block
// as patterns arrive.
func (g *Generator) collectToGitignoreSink(outputs []config.OutputFile) error {
	paths, overridden := g.neededGitignorePatterns(outputs)
	g.warnUnignoredProtected(overridden)
	if len(paths) == 0 {
		return nil
	}
	if err := g.gitignoreSink.Add(g.log(), g.gitignorePrefix, paths); err != nil {
		return oops.Wrapf(err, "add patterns to the shared .gitignore block")
	}
	return nil
}

// dropGitignoreBlock removes the managed block an earlier run left in
// existingContent, rather than leaving an empty fence behind when nothing is
// left to add.
func (g *Generator) dropGitignoreBlock(gitignorePath, existingContent string) error {
	if !strings.Contains(existingContent, gitignore.BeginMarker) && !strings.Contains(existingContent, gitignore.OldHeader) {
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
	case strings.Contains(existingContent, gitignore.BeginMarker):
		return gitignore.ReplaceFencedBlock(existingContent, fencedBlock.String())
	case strings.Contains(existingContent, gitignore.OldHeader):
		return gitignore.ReplaceOldHeaderBlock(existingContent, fencedBlock.String())
	case existingContent == "":
		return fencedBlock.String()
	}
	newContent := existingContent
	if !strings.HasSuffix(newContent, "\n") {
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
