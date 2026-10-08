package generator

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// CleanOptions controls Clean behavior.
type CleanOptions struct {
	// DryRun computes the plan without deleting anything.
	DryRun bool
	// KeepGitignore leaves the ai-rulez managed block in .gitignore in place.
	KeepGitignore bool
	// KeepManifest leaves the generated manifest (.generated-manifest.json) in place.
	KeepManifest bool
	// RemoveEdited also deletes generated files whose body was edited by hand
	// (their Content-Hash no longer matches); without it they are kept and reported.
	RemoveEdited bool
}

// CleanPlan describes what Clean removed (or, in dry-run, would remove).
type CleanPlan struct {
	Profile      string
	Files        []string // absolute paths of generated files to remove
	Dirs         []string // absolute generated dirs, deepest-first (removed only if empty)
	ManifestPath string   // absolute manifest path; "" when absent or kept
	// LocalManifestPath is the absolute path of the machine-local manifest; ""
	// when absent or kept.
	LocalManifestPath string
	GitignoreEdited   bool // the managed .gitignore block will be / was stripped
	// Unmerged lists the absolute paths of hand-authored JSON documents (such as
	// .claude/settings.json) that ai-rulez merged keys into; clean removes those
	// keys and keeps the rest. A document left with nothing else is in Files.
	Unmerged []string
	// Restored lists the absolute paths of imported commands generate retired in
	// favor of a generated skill; clean writes them back as it removes the skill.
	Restored []string
}

// Empty reports whether the plan would remove nothing at all.
func (p *CleanPlan) Empty() bool {
	return len(p.Files) == 0 && len(p.Dirs) == 0 && len(p.Unmerged) == 0 && len(p.Restored) == 0 &&
		p.ManifestPath == "" && p.LocalManifestPath == "" && !p.GitignoreEdited
}

// Clean removes the files and directories that Generate produced for the given
// profile — the inverse of Generate. It never touches the .ai-rulez/ source
// tree, since those paths are not among the generated outputs. Files are removed
// first, then now-empty generated directories deepest-first; the generated
// manifest and the ai-rulez managed .gitignore block are removed too unless the
// corresponding Keep* option is set. With DryRun the plan is computed but nothing
// is deleted.
func (g *Generator) Clean(profile string, opts CleanOptions) (*CleanPlan, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	// Rendering the outputs to learn their paths also runs the generate-time
	// advice (a context.fileName that misses a name, ...), which says nothing
	// useful while the outputs are being removed.
	defer g.diagnostics().SetSink(func(msg string, _ ...any) { g.log().Debug(msg) })()

	g.lenientMCP = true // clean needs paths and claims, never secret values
	defer func() { g.lenientMCP = false }()
	outputs, activeProfile, err := g.collectForClean(profile)
	if err != nil {
		return nil, err
	}

	g.localSkipped = false // the local manifest's files go with it, loaded or not
	plan := &CleanPlan{Profile: activeProfile}
	g.previousFiles = nil
	defer func() { g.previousFiles = nil }()

	dirs := g.collectCleanTargets(outputs, plan, opts)
	pluginFiles, pluginDirs := g.pluginCleanTargets(profile)
	plan.Files = append(plan.Files, pluginFiles...)
	dirs = append(dirs, pluginDirs...)

	// Include files recorded in the manifest from earlier runs that the current
	// profile no longer emits (e.g. a preset was removed): the exact set generate
	// itself cleans up as stale.
	plan.Files = append(plan.Files, g.cleanStaleFiles(outputs)...)
	// Merged documents shared with the user lose only what ai-rulez wrote into
	// them; one that holds nothing else goes with the other generated files.
	edits := g.planUnmerge(outputs, true)
	plan.Files = append(plan.Files, deletedPaths(edits)...)
	plan.Unmerged = editedPaths(edits)
	plan.Files = existingSortedUnique(plan.Files)

	// Scoped rule files sit in subfolders of the root rules folders that are not
	// outputs themselves; the folders this removal empties are part of the plan.
	dirs = existingDirs(dirs)
	removing := plan.Files
	if g.userMode {
		dirs = slices.DeleteFunc(dirs, func(dir string) bool { return !g.userMayRemoveDir(dir) })
		// A generated folder that holds no file (~/.cursor/commands) empties its
		// parent too, so the parents of the folders count as removed.
		removing = append(slices.Clone(plan.Files), dirs...)
	}
	var generated []string
	if !g.userMode {
		// A generated folder that ends up empty (.agents/skills with no skill)
		// empties the folder generate created implicitly above it (.agents).
		generated = dirs
	}
	dirs = append(dirs, g.emptiedDirs(removing, generated...)...)
	// Deepest-first so children are removed before their parents.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] > dirs[j] })
	plan.Dirs = slices.DeleteFunc(slices.Compact(dirs), func(dir string) bool { return holdsAny(dir, plan.Restored) })

	if !opts.KeepManifest {
		if mp := g.manifestPath(); pathIsFile(mp) {
			plan.ManifestPath = mp
		}
		plan.LocalManifestPath = g.removableLocalManifest()
	}
	plan.GitignoreEdited = !opts.KeepGitignore && !g.userMode && g.gitignoreHasManagedBlock()

	if opts.DryRun {
		return plan, nil
	}

	if err := g.restoreRetiredCommands(plan.Restored); err != nil {
		return nil, err
	}
	g.applyUnmerge(rewrites(edits))
	for _, f := range plan.Files {
		g.removeStaleFile(f)
	}
	for _, d := range plan.Dirs {
		g.removeEmptyDir(d)
	}
	if plan.ManifestPath != "" {
		g.removeStaleFile(plan.ManifestPath)
	}
	if plan.LocalManifestPath != "" {
		g.removeStaleFile(plan.LocalManifestPath)
	}
	g.cleanGitignore(opts, plan)

	return plan, nil
}

// removableLocalManifest returns the machine-local manifest when clean may delete
// it. A tracked one is the repository's, not machine state: deleting it would
// become a tracked change, so it is kept with a warning, as generate refuses to
// write it.
func (g *Generator) removableLocalManifest() string {
	mp := g.localManifestPath()
	if !pathIsFile(mp) {
		return ""
	}
	if g.git().IsTrackedContext(g.ctx, mp) {
		g.warnOnce("Not removing "+g.localManifestRel()+": git tracks it, and it must stay machine-local",
			"fix", "git rm --cached "+g.localManifestRel())
		return ""
	}
	return mp
}

// collectCleanTargets adds the generated files of outputs to plan.Files and
// returns the generated directories.
func (g *Generator) collectCleanTargets(outputs []config.OutputFile, plan *CleanPlan, opts CleanOptions) []string {
	var dirs []string
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		if !g.withinScope(abs) {
			g.log().Warn("Skipping generated path outside project", "path", output.Path)
			continue
		}
		if g.cleanLeavesMerged(output, abs) {
			continue
		}
		if output.IsDir {
			dirs = append(dirs, abs)
			continue
		}
		if g.cleanKeepsFile(output, abs, opts) {
			continue
		}
		if !g.cleanRetiredCommand(abs, plan) {
			continue
		}
		plan.Files = append(plan.Files, abs)
	}
	return dirs
}

// cleanLeavesMerged reports whether output is a merged document clean must not
// delete whole.
func (g *Generator) cleanLeavesMerged(output config.OutputFile, abs string) bool {
	// A merged document that also holds hand-authored content is not ours to
	// delete; removing it would take the user's own settings with it.
	if output.PartiallyOwned {
		return true
	}
	// A merged document is taken apart by claim (planUnmerge) and deleted only
	// when this machine recorded writing it whole; a hand-written file that
	// merely equals the rendering is not proof.
	return !g.userMode && !output.IsDir && (len(output.MergeClaims) > 0 || g.isMergedDocument(abs))
}

// cleanKeepsFile reports whether clean must keep the generated file at abs,
// warning when the reason is worth telling the user.
func (g *Generator) cleanKeepsFile(output config.OutputFile, abs string, opts CleanOptions) bool {
	if !g.userMode && isSymlink(abs) {
		g.warnOnce("Keeping " + g.relSlash(abs) + ": it is a symlink, and links are the user's to remove")
		return true
	}
	if !g.removalConfined(abs) {
		g.warnOnce("Not removing " + output.Path + ": it is behind a symlink that leaves the project")
		return true
	}
	if g.cleanKeepsConvertedOriginal(abs) {
		return true
	}
	// A generated file someone edited holds work ai-rulez cannot recreate.
	if !opts.RemoveEdited && output.RawContent == nil && g.editedGenerated(abs) {
		g.warnOnce("Keeping "+output.Path+": its body was edited by hand", "hint", "pass --include-edited to remove it anyway")
		return true
	}
	// A hand-written file in a shared rules folder is not ours to delete.
	if output.RawContent == nil && g.isUnmanagedRuleFile(abs, g.finalContent(output)) {
		return true
	}
	// In user scope a file is removed only when ai-rulez wrote it.
	if g.userMode && !g.userManaged(abs, output) {
		return true
	}
	// In project scope the same holds: a file at a generated path may be the
	// user's own (a CLAUDE.md that `convert` imported and generate has not yet
	// replaced), and being at the path proves nothing.
	if !g.userMode && !g.projectFileIsOurs(abs, output) {
		if _, err := os.Lstat(abs); err != nil {
			return true // already gone (an earlier run removed it): nothing to keep, nothing to warn about
		}
		g.warnOnce("Keeping "+g.relSlash(abs)+": nothing shows ai-rulez wrote it",
			"hint", g.keepReason(abs)+"; delete it by hand if it is not needed")
		return true
	}
	return false
}

// cleanKeepsConvertedOriginal reports whether abs is the generated file that
// replaced an original `convert` imported, which is the only copy of it left at
// that path: removing it would leave neither.
func (g *Generator) cleanKeepsConvertedOriginal(abs string) bool {
	if g.userMode || !g.convertedOriginal(g.relSlash(abs)) {
		return false
	}
	why := "it replaced a file `convert` imported, and the content now lives under the config directory"
	if data, rerr := g.config.ReadExisting(abs); rerr == nil && g.adoptable(g.relSlash(abs), data) {
		why = "it is the file `convert` imported, and nothing has replaced it yet"
	}
	g.warnOnce("Keeping "+g.relSlash(abs)+": "+why,
		"hint", "delete it by hand once you no longer want it in the repository")
	return true
}

// cleanRetiredCommand handles a skill that replaced a command `convert`
// imported: it goes only when the command can come back in its place. It
// reports whether abs may be removed, recording the command to restore in plan.
func (g *Generator) cleanRetiredCommand(abs string, plan *CleanPlan) bool {
	cmd, ok := g.retiredCommand(abs)
	if !ok {
		return true
	}
	if !g.restorableCommand(cmd) {
		g.warnOnce("Keeping "+g.relSlash(abs)+": it replaced "+cmd+", which `convert` imported, "+
			"and the copy under the config directory no longer holds the imported bytes",
			"hint", "delete it by hand once you no longer want it in the repository")
		return false
	}
	plan.Restored = append(plan.Restored, filepath.Join(g.config.BaseDir, filepath.FromSlash(cmd)))
	return true
}

// pluginCleanTargets lists what `generate --plugin` wrote for a project with a
// [plugin] block or a plugin marketplace: the bundle files whose bytes are still
// exactly what it renders, and its directories (removed only when empty). The
// bundle has no manifest entry, so that equality is the proof; an edited file is
// kept with a warning.
func (g *Generator) pluginCleanTargets(profile string) (files, dirs []string) {
	mkt := g.config.Marketplace
	if g.userMode || (g.config.Plugin == nil && (mkt == nil || (len(mkt.Members) == 0 && !mkt.HasDomainPlugins()))) {
		return nil, nil
	}
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		g.warnOnce("Keeping the plugin bundle: it cannot be rendered to tell which files generate --plugin wrote",
			"error", err)
		return nil, nil
	}
	for i := range outputs {
		output := &outputs[i]
		abs := g.absOutputPath(output.Path)
		if output.IsDir {
			dirs = append(dirs, abs)
			continue
		}
		if !g.withinScope(abs) || isSymlink(abs) || !g.removalConfined(abs) {
			continue
		}
		data, rerr := g.config.ReadExisting(abs)
		if rerr != nil {
			continue
		}
		want := output.RawContent
		if want == nil {
			want = []byte(output.Content)
		}
		if !bytes.Equal(data, want) {
			g.warnOnce("Keeping "+g.relSlash(abs)+": it differs from what generate --plugin writes",
				"hint", "delete it by hand if it is no longer needed")
			continue
		}
		files = append(files, abs)
	}
	return files, dirs
}

// projectFileIsOurs reports whether clean may remove the project file at abs
// because ai-rulez provably wrote it: it carries a Content-Hash, its bytes equal
// what a generate would write, or the generated manifest lists it and it carries a
// generated banner or the digest the local manifest recorded. With no manifest
// and no header nothing is provable, so nothing is removed.
func (g *Generator) projectFileIsOurs(abs string, output config.OutputFile) bool {
	data, err := g.config.ReadExisting(abs)
	if err != nil {
		return false
	}
	if output.RawContent != nil {
		if bytes.Equal(data, output.RawContent) {
			return true
		}
	} else if string(data) == g.finalContent(output) {
		return true
	}
	rel := filepath.ToSlash(g.convertToRelativePath(abs))
	return g.outputProvenance(abs, rel, data, g.headerlessOutput(abs, output)) == provenStrong
}

// isMergedDocument reports whether abs is one of the documents ai-rulez merges
// keys into, however many of them this run still claims.
func (g *Generator) isMergedDocument(abs string) bool {
	return isMergedDocumentPath(mergedDocuments(), g.relSlash(abs))
}

// editedGenerated reports whether the file at abs carries a Content-Hash its body
// no longer matches.
func (g *Generator) editedGenerated(abs string) bool {
	data, err := os.ReadFile(abs)
	return err == nil && bodyEdited(string(data), abs)
}

// gitignoreHasManagedBlock reports whether <BaseDir>/.gitignore contains the
// ai-rulez fenced block.
func (g *Generator) gitignoreHasManagedBlock() bool {
	data, err := gitutil.ReadIgnoreFileOrEmpty(g.log(), filepath.Join(g.config.BaseDir, ".gitignore"))
	if err != nil {
		return false
	}
	return contains(string(data), gitignore.BeginMarker)
}

// stripGitignoreManagedBlock removes the ai-rulez fenced block from .gitignore,
// leaving any user-authored entries intact. If nothing remains, the .gitignore
// file is deleted rather than left empty.
func (g *Generator) stripGitignoreManagedBlock() error {
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")
	data, err := gitutil.ReadIgnoreFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, gitutil.ErrNotRegular) || errors.Is(err, gitutil.ErrTooLarge) {
			return nil
		}
		return oops.With("path", gitignorePath).Wrapf(err, "read .gitignore")
	}

	// Machine-local files (the config.local.* overlay, its lock, the local/ tree)
	// stay ignored: clean removes generated outputs, not the secrets beside them.
	keep := ""
	if patterns := g.localGitignorePatternsOnDisk(); len(patterns) > 0 && contains(string(data), gitignore.BeginMarker) {
		keep = gitignore.BeginMarker + "\n" + strings.Join(patterns, "\n") + "\n" + gitignore.EndMarker + "\n"
	}
	stripped := strings.TrimRight(gitignore.ReplaceFencedBlock(string(data), keep), "\n")
	if strings.TrimSpace(stripped) == "" {
		if err := os.Remove(gitignorePath); err != nil {
			return oops.With("path", gitignorePath).Wrapf(err, "remove empty .gitignore")
		}
		return nil
	}
	safePath, _, err := g.guardWrite(gitignorePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(safePath, []byte(stripped+"\n"), 0o644); err != nil { //nolint:gosec // path from config, not user input
		return oops.With("path", gitignorePath).Wrapf(err, "write .gitignore")
	}
	return nil
}

// existingSortedUnique filters to on-disk files, deduplicates, and sorts.
func existingSortedUnique(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if seen[p] || !pathIsFile(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// holdsAny reports whether dir is an ancestor of any of paths.
func holdsAny(dir string, paths []string) bool {
	for _, p := range paths {
		if isUnderBaseDir(dir, p) && filepath.Clean(dir) != filepath.Clean(p) {
			return true
		}
	}
	return false
}

// existingDirs filters to directories that exist on disk, preserving order.
func existingDirs(paths []string) []string {
	var out []string
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// pruneDirsEmptiedBy removes the directories that deleting removed left empty.
// Generate's stale pass deletes files a narrower profile no longer emits but used
// to leave their directories standing, and an empty `<id>/` under skills/ reads —
// to a human and to tooling that lists the directory — as a live skill that has
// lost its body.
//
// Blast radius is bounded three ways. Only the ancestors of a file ai-rulez wrote
// are candidates, so the walk never leaves the generated output roots. It stops
// below the project root and refuses the .ai-rulez source tree. And a directory
// holding any entry at all survives: a skill's hand-authored references/, scripts/
// or assets/ file keeps both that subdirectory and the skill directory above it.
func (g *Generator) pruneDirsEmptiedBy(removed []string) {
	dirs := g.pruneCandidates(removed)
	// Deepest-first, so emptying a child lets its parent go in the same pass.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] > dirs[j] })
	for _, dir := range dirs {
		g.removeEmptyDir(dir)
	}
}

// emptiedDirs returns the directories that removing the given files would leave
// empty, deepest first, without touching the filesystem. generatedDirs are
// directories clean removes when empty: each is checked like a candidate, and
// so are the directories between it and its output root.
func (g *Generator) emptiedDirs(removed []string, generatedDirs ...string) []string {
	gone := make(map[string]bool, len(removed))
	for _, file := range removed {
		gone[filepath.Clean(file)] = true
	}
	candidates := g.pruneCandidates(removed)
	for _, dir := range append(g.generatedDirParents(generatedDirs), generatedDirs...) {
		if !slices.Contains(candidates, dir) && g.isPrunableDir(dir) {
			candidates = append(candidates, dir)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] > candidates[j] })
	var emptied []string
dirLoop:
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !gone[filepath.Join(dir, entry.Name())] {
				continue dirLoop
			}
		}
		gone[dir] = true
		emptied = append(emptied, dir)
	}
	return emptied
}

// generatedDirParents lists the directories above each generated directory up to
// its output root (.agents above .agents/skills): generate created them
// implicitly. A generated directory outside any hidden root has none.
func (g *Generator) generatedDirParents(dirs []string) []string {
	var parents []string
	for _, dir := range dirs {
		stop := g.outputRoot(dir)
		for cur := dir; cur != stop; {
			cur = filepath.Dir(cur)
			if !g.isPrunableDir(cur) {
				break
			}
			parents = append(parents, cur)
		}
	}
	return parents
}

// pruneCandidates lists the directories above the removed files that may be
// removed once empty. The walk up from a file stops at the generated output root:
// the outermost hidden directory on its path (.claude, .github, ...), or just
// the file's own directory when there is none. Visible directories above that,
// such as a monorepo scope directory, hold sources and are never candidates.
func (g *Generator) pruneCandidates(removed []string) []string {
	if g.userMode {
		return g.userPruneCandidates(removed)
	}
	seen := make(map[string]bool, len(removed))
	var dirs []string
	for _, file := range removed {
		dir := filepath.Dir(file)
		stop := g.outputRoot(dir)
		for g.isPrunableDir(dir) {
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
			if dir == stop {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
	return dirs
}

// outputRoot returns the outermost hidden directory at or above dir below the
// project root, or dir itself when none of its path components is hidden.
func (g *Generator) outputRoot(dir string) string {
	base, absErr := filepath.Abs(g.config.BaseDir)
	if absErr != nil {
		return dir
	}
	rel, err := filepath.Rel(base, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return dir
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ".") && part != "." {
			return filepath.Join(append([]string{base}, parts[:i+1]...)...)
		}
	}
	return dir
}

// isPrunableDir reports whether dir is a directory the generator may remove: one
// strictly inside the project and outside the .ai-rulez configuration tree. It
// bounds the upward walk in pruneDirsEmptiedBy, which stops at the first
// directory this rejects.
func (g *Generator) isPrunableDir(dir string) bool {
	clean := filepath.Clean(dir)
	if !isUnderBaseDir(g.config.BaseDir, clean) {
		return false
	}
	if base, err := filepath.Abs(g.config.BaseDir); err == nil {
		if abs, absErr := filepath.Abs(clean); absErr == nil && abs == base {
			return false
		}
	}
	// A scope directory holds sources, however empty its generated outputs leave it.
	for _, scope := range g.config.Scopes {
		if filepath.Clean(filepath.Join(g.config.BaseDir, filepath.FromSlash(scope.Path))) == clean {
			return false
		}
	}
	// isUnderBaseDir is true for the directory itself, so this rejects the config
	// dir along with everything in it.
	return g.config.ConfigDir == "" || !isUnderBaseDir(g.config.ConfigDir, clean)
}

// removeEmptyDir is removeDirIfEmpty, which in user scope first refuses a
// directory that resolves outside the home directory (or a relocated tool home).
func (g *Generator) removeEmptyDir(dir string) {
	if g.userMode && (!g.userMayTouch(dir) || !g.userMayRemoveDir(dir)) {
		return
	}
	if !g.removalConfined(dir) {
		return
	}
	g.removeDirIfEmpty(dir)
}

// removeDirIfEmpty removes a directory only when it holds no entries, so
// user-authored files inside a generated directory are never destroyed.
func (g *Generator) removeDirIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) > 0 {
		return
	}
	if err := os.Remove(dir); err != nil {
		g.log().Debug("Kept non-removable directory", "path", dir, "error", err)
	} else {
		g.log().Debug("Removed empty directory", "path", dir)
	}
}

// pathIsFile reports whether path exists and is a regular (non-directory) file.
func pathIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// collectForClean collects the outputs clean plans against: the project's, or in
// user scope the ones mapped into the home directory.
func (g *Generator) collectForClean(profile string) ([]config.OutputFile, string, error) {
	if !g.userMode {
		return g.collectOutputs(profile)
	}
	outputs, active, _, err := g.collectUserOutputs(profile)
	return outputs, active, err
}

// cleanStaleFiles lists the files the manifest records that the current outputs
// no longer include. In user scope a file the user has since replaced stays.
func (g *Generator) cleanStaleFiles(outputs []config.OutputFile) []string {
	if g.userMode {
		return g.userStale(outputs)
	}
	return g.staleManifestFiles(outputs)
}

// cleanGitignore strips the managed .gitignore block and the project's block in
// .git/info/exclude. User scope has neither: it never touches the home directory's
// git configuration.
func (g *Generator) cleanGitignore(opts CleanOptions, plan *CleanPlan) {
	if g.userMode {
		return
	}
	if plan.GitignoreEdited {
		if err := g.stripGitignoreManagedBlock(); err != nil {
			g.log().Warn("Failed to strip .gitignore managed block", "error", err)
			plan.GitignoreEdited = false
		}
	}
	if !opts.KeepGitignore {
		// This project's block in .git/info/exclude names machine-local outputs
		// that were just removed; no paths means the block is dropped.
		if err := g.syncMachineExcludes(nil); err != nil {
			g.log().Warn("Failed to remove .git/info/exclude block", "error", err)
		}
	}
}
