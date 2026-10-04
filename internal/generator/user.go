package generator

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/generator/userscope"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// User scope renders the person's own configuration (not a project's) into the
// per-user directories each harness reads: ~/.claude, ~/.agents/skills,
// ~/.codex, ~/.gemini, ~/.config/opencode, ~/.copilot, ~/.pi/agent. It reuses the
// preset renderers and maps their project-relative output through the closed
// table in package userscope, so only locations a vendor documents are written.
//
// The config directory (default ~/.config/ai-rulez) keeps the generated manifest
// beside the user config, which is what `clean --user` reads. Nothing is written
// unless the caller passes the explicit user opt-in (SetUserScope).

// UserSkip is an output user scope left alone, and why.
type UserSkip struct {
	Path   string
	Reason string
}

// UserPlan is what a user-scope run writes, merges into and skips.
type UserPlan struct {
	Profile string
	// Writes are the files written whole (new or rewritten), absolute, sorted.
	Writes []string
	// Merges are shared documents (settings.json, hooks.json) ai-rulez merges keys into.
	Merges []string
	// Skips are outputs left alone because a file not written by ai-rulez is in the way.
	Skips []UserSkip
	// Stale are files an earlier run wrote that this one no longer produces.
	Stale []string
	// Unmerge are shared documents from which an earlier run's keys are taken back out.
	Unmerge []string
	// Dropped counts project outputs with no documented user-level destination.
	Dropped int
	// Warnings are advisories: skills loaded twice, precedence against a project.
	Warnings []string
}

// SetUserScope switches the Generator to user scope: outputs are mapped into the
// home directory (the config's BaseDir), the manifest lives in the config
// directory, and nothing project-shaped (gitignore, MCP, scopes, local overlays)
// is touched.
func (g *Generator) SetUserScope() {
	g.userMode = true
	g.config.UserScope = true
}

// IsUserScope reports whether the Generator runs in user scope.
func (g *Generator) IsUserScope() bool { return g.userMode }

// SetProjectDir names the project the user runs from, so skills that exist at
// both levels can be reported. Empty disables the check.
func (g *Generator) SetProjectDir(dir string) { g.projectDir = dir }

// collectUserOutputs renders every configured preset and maps the result to the
// user-level destinations under BaseDir (the home directory).
func (g *Generator) collectUserOutputs(profile string) (outputs []config.OutputFile, activeProfile string, dropped int, err error) {
	activeProfile = g.resolveProfile(profile)
	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, "", 0, err
	}
	presets := userPresets(g.config)
	for _, name := range userUnsupportedPresets(g.config) {
		rulefiles.Warn("preset "+name+" has no documented user-level location, so --user writes nothing for it",
			"hint", "supported: "+strings.Join(userscope.Presets(), ", "))
	}
	if g.config.GeneratedAt.IsZero() {
		g.config.GeneratedAt = config.ResolveGenerationTime()
	}

	tempCfg := *g.config
	tempCfg.Content = contentTree
	tempCfg.MCPServers = nil
	tempCfg.MCP = nil
	tempCfg.Scopes = nil
	tempCfg.AgentsMD = false
	tempCfg.UserScope = true
	tempCfg.Presets = presets
	run := config.NewRunState()
	run.SetPreviouslyGenerated(g.previousManifestFiles())
	run.SetPreviousMerged(g.previousMergedClaims())
	tempCfg.Run = run
	tempCfg.SourceHash = computeSourceHash(&tempCfg, contentTree)
	g.config.SourceHash = tempCfg.SourceHash

	rendered, err := config.GeneratePresets(&tempCfg)
	if err != nil {
		return nil, "", 0, oops.Wrapf(err, "generate presets")
	}
	mapped := make(map[string][]config.OutputFile, len(rendered))
	for preset, outs := range rendered {
		for _, output := range outs {
			if output.LocalOnly {
				dropped++
				continue
			}
			rel := filepath.ToSlash(g.convertToRelativePath(output.Path))
			dest, _, ok := userscope.Map(preset, rel)
			if !ok {
				if !output.IsDir {
					dropped++
				}
				continue
			}
			output.Path = filepath.Join(g.config.BaseDir, filepath.FromSlash(dest))
			mapped[preset] = append(mapped[preset], output)
		}
	}
	outputs = flattenPresetOutputs(mapped)
	g.reclaimStaleMembers(outputs)
	return outputs, activeProfile, dropped, nil
}

// userPresets keeps the configured presets that have a user-level destination.
func userPresets(cfg *config.Config) []config.Preset {
	var out []config.Preset
	for _, p := range cfg.Presets {
		if p.IsBuiltIn() && userscope.Supports(p.BuiltIn) {
			out = append(out, p)
		}
	}
	return out
}

// userUnsupportedPresets names the configured presets user scope cannot write
// for (the shared mcp preset is not a harness and is not reported).
func userUnsupportedPresets(cfg *config.Config) []string {
	var out []string
	for _, p := range cfg.Presets {
		name := p.GetName()
		if name == string(config.PresetMCP) || (p.IsBuiltIn() && userscope.Supports(name)) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// guardUserOutputs keeps the outputs user scope may write and lists the rest.
// Three rules apply, in order. Everything must stay below the home directory once
// symlinks are resolved (a ~/.claude symlinked into a dotfiles repository is
// fine, one pointing out of the home directory is refused). A file that exists
// and was not written by ai-rulez is never replaced: it is skipped, and a skill
// directory holding one is skipped whole so a hand-authored skill is not mixed
// with generated resources. A symlinked file is skipped unless it is a shared
// document ai-rulez merges keys into.
func (g *Generator) guardUserOutputs(outputs []config.OutputFile) (kept []config.OutputFile, skips []UserSkip, err error) {
	previous := map[string]bool{}
	for _, rel := range g.previousManifestFiles() {
		previous[rel] = true
	}
	realHome, err := filepath.EvalSymlinks(g.config.BaseDir)
	if err != nil {
		return nil, nil, oops.With("home", g.config.BaseDir).Wrapf(err, "resolve the home directory")
	}

	skippedDirs := map[string]bool{}
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		if !isUnderBaseDir(g.config.BaseDir, abs) {
			return nil, nil, oops.With("path", abs).Errorf("user-level output %s is outside the home directory", abs)
		}
		if err := g.checkSymlinkEscape(realHome, abs); err != nil {
			return nil, nil, err
		}
		if output.IsDir {
			kept = append(kept, output)
			continue
		}
		if dir := skippedSkillDir(skippedDirs, abs); dir != "" {
			skips = append(skips, UserSkip{Path: abs, Reason: "its skill directory holds a file ai-rulez did not write"})
			continue
		}
		if reason := g.userConflict(abs, output, previous); reason != "" {
			skips = append(skips, UserSkip{Path: abs, Reason: reason})
			if filepath.Base(abs) == "SKILL.md" {
				skippedDirs[filepath.Dir(abs)] = true
			}
			continue
		}
		kept = append(kept, output)
	}
	// A skill directory skipped after some of its files were kept drops those too.
	if len(skippedDirs) > 0 {
		kept = slices.DeleteFunc(kept, func(output config.OutputFile) bool {
			abs := g.absOutputPath(output.Path)
			if output.IsDir {
				return skippedDirs[abs]
			}
			for dir := range skippedDirs {
				if isUnderBaseDir(dir, abs) {
					skips = append(skips, UserSkip{Path: abs, Reason: "its skill directory holds a file ai-rulez did not write"})
					return true
				}
			}
			return false
		})
	}
	sort.Slice(skips, func(i, j int) bool { return skips[i].Path < skips[j].Path })
	return kept, skips, nil
}

// skippedSkillDir returns the already skipped skill directory abs lies in, or "".
func skippedSkillDir(skipped map[string]bool, abs string) string {
	for dir := range skipped {
		if isUnderBaseDir(dir, abs) {
			return dir
		}
	}
	return ""
}

// checkSymlinkEscape refuses an output whose nearest existing ancestor resolves
// outside the resolved home directory.
func (g *Generator) checkSymlinkEscape(realHome, abs string) error {
	probe := abs
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return nil
		}
		probe = parent
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return oops.With("path", abs).Wrapf(err, "resolve %s", probe)
	}
	if !isUnderBaseDir(realHome, resolved) {
		return oops.With("path", abs).With("resolves_to", resolved).
			Hint("A symlink below the home directory points out of it; generate --user writes only inside the home directory").
			Errorf("%s resolves outside the home directory", abs)
	}
	return nil
}

// userConflict explains why an existing file at abs must not be written, or "".
func (g *Generator) userConflict(abs string, output config.OutputFile, previous map[string]bool) string {
	info, err := os.Lstat(abs)
	if err != nil {
		return ""
	}
	merged := output.PartiallyOwned || len(output.MergeClaims) > 0
	if info.Mode()&os.ModeSymlink != 0 && !merged {
		return "is a symlink"
	}
	if info.IsDir() {
		return "is a directory"
	}
	if merged {
		return ""
	}
	rel := filepath.ToSlash(g.convertToRelativePath(abs))
	if previous[rel] || g.userFileIsOurs(abs, output) {
		return ""
	}
	return "exists and was not written by ai-rulez"
}

// userFileIsOurs reports whether the file at abs already is what would be written,
// or carries ai-rulez's generated banner.
func (g *Generator) userFileIsOurs(abs string, output config.OutputFile) bool {
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	if output.RawContent != nil {
		return bytes.Equal(data, output.RawContent)
	}
	return string(data) == g.finalContent(output) || hasGeneratedBanner(abs, data)
}

// userStaleOK reports whether a file the manifest lists may still be removed: it
// must not have been replaced by the user since ai-rulez wrote it. Files that
// carry a header must still look generated; headerless ones (JSON, scripts,
// assets) are trusted to the manifest.
func (g *Generator) userStaleOK(abs string) bool {
	info, err := os.Lstat(abs)
	if err != nil || info.IsDir() {
		return false
	}
	if g.config.GetHeaderHashes() == config.HeaderHashesNone || !headerCapable(abs) {
		return true
	}
	return looksGenerated(abs)
}

func headerCapable(abs string) bool {
	return isMarkdownRuleExt(abs) || slices.Contains([]string{".toml", ".yaml", ".yml"}, strings.ToLower(filepath.Ext(abs)))
}

// userStale lists the manifest files an earlier run wrote that this one no
// longer produces and that are still ai-rulez's to remove.
func (g *Generator) userStale(outputs []config.OutputFile) []string {
	var stale []string
	for _, abs := range g.staleManifestFiles(outputs) {
		if g.userStaleOK(abs) {
			stale = append(stale, abs)
		}
	}
	return stale
}

// PlanUser computes what GenerateUser would do without touching the filesystem.
func (g *Generator) PlanUser(profile string) (*UserPlan, error) {
	generateMu.Lock()
	defer generateMu.Unlock()
	plan, _, err := g.planUser(profile)
	return plan, err
}

func (g *Generator) planUser(profile string) (*UserPlan, []config.OutputFile, error) {
	g.beginRun()
	rulefiles.ResetDowngrades()
	defer rulefiles.FlushDowngrades()

	outputs, active, dropped, err := g.collectUserOutputs(profile)
	if err != nil {
		return nil, nil, err
	}
	kept, skips, err := g.guardUserOutputs(outputs)
	if err != nil {
		return nil, nil, err
	}
	plan := &UserPlan{Profile: active, Skips: skips, Dropped: dropped}
	for _, output := range kept {
		if output.IsDir {
			continue
		}
		abs := g.absOutputPath(output.Path)
		if output.PartiallyOwned || len(output.MergeClaims) > 0 {
			plan.Merges = append(plan.Merges, abs)
		} else {
			plan.Writes = append(plan.Writes, abs)
		}
	}
	plan.Stale = g.userStale(kept)
	for _, edit := range g.planUnmerge(kept, false) {
		if edit.delete {
			plan.Stale = append(plan.Stale, edit.abs)
		} else {
			plan.Unmerge = append(plan.Unmerge, edit.abs)
		}
	}
	plan.Warnings = g.userWarnings(kept)
	sort.Strings(plan.Writes)
	sort.Strings(plan.Merges)
	sort.Strings(plan.Stale)
	sort.Strings(plan.Unmerge)
	return plan, kept, nil
}

// GenerateUser writes the user-level outputs and the manifest. It returns the plan
// it carried out; the caller is expected to have shown PlanUser first.
func (g *Generator) GenerateUser(profile string) (*UserPlan, error) {
	generateMu.Lock()
	defer generateMu.Unlock()
	rulefiles.ResetDowngrades()
	defer rulefiles.FlushDowngrades()

	plan, kept, err := g.planUser(profile)
	if err != nil {
		return nil, err
	}
	g.beginRun()
	logger.Info("Generating user-level configuration", "profile", plan.Profile, "home", g.config.BaseDir)

	stale := g.userStale(kept)
	g.removeStaleManifestFiles(stale)
	if err := g.writeOutputs(kept); err != nil {
		return nil, err
	}
	unmerged := g.planUnmerge(kept, false)
	g.applyUnmerge(unmerged)
	g.pruneDirsEmptiedBy(append(stale, deletedPaths(unmerged)...))
	if err := g.writeGeneratedManifest(kept); err != nil {
		return nil, oops.Wrapf(err, "write the user manifest")
	}
	logger.Info("User-level generation complete", "files", len(plan.Writes)+len(plan.Merges))
	return plan, nil
}

// userPruneCandidates lists the directories above removed files that clean may
// remove once empty: only those inside a directory user scope owns the
// content of (~/.claude/skills and below, never ~/.claude itself).
func (g *Generator) userPruneCandidates(removed []string) []string {
	var roots []string
	for _, root := range userscope.Roots() {
		roots = append(roots, filepath.Join(g.config.BaseDir, filepath.FromSlash(root)))
	}
	seen := map[string]bool{}
	var dirs []string
	for _, file := range removed {
		for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
			below := slices.ContainsFunc(roots, func(root string) bool { return isUnderBaseDir(root, dir) })
			if !below {
				break
			}
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// userManaged reports whether clean may remove the generated file at abs.
func (g *Generator) userManaged(abs string, output config.OutputFile) bool {
	info, err := os.Lstat(abs)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return false
	}
	previous := map[string]bool{}
	for _, rel := range g.previousManifestFiles() {
		previous[rel] = true
	}
	return previous[filepath.ToSlash(g.convertToRelativePath(abs))] || g.userFileIsOurs(abs, output)
}

// userWarnings reports skills that would load twice: within one harness that
// reads several of the user-level directories written, and against a project
// that holds a skill of the same name.
func (g *Generator) userWarnings(outputs []config.OutputFile) []string {
	home := g.config.BaseDir
	byDir := map[string][]string{} // user-level skill dir -> skill ids
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if path.Base(rel) != "SKILL.md" {
			continue
		}
		id := path.Base(path.Dir(rel))
		dir := path.Dir(path.Dir(rel))
		byDir[dir] = append(byDir[dir], id)
	}
	configured := map[string]bool{}
	for _, preset := range userPresets(g.config) {
		configured[preset.BuiltIn] = true
	}
	var warnings []string
	for _, harness := range userscope.ReaderNames() {
		if !configured[harness] {
			continue // a harness the user did not configure is not reported on
		}
		var dirs []string
		for _, dir := range userscope.SkillReaders[harness] {
			if len(byDir[dir]) > 0 {
				dirs = append(dirs, dir)
			}
		}
		if len(dirs) < 2 {
			continue
		}
		dup := duplicateIDs(byDir, dirs)
		if len(dup) == 0 {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s reads ~/%s, so these skills load twice: %s",
			harness, strings.Join(dirs, ", ~/"), abbreviate(dup, maxListedSkills)))
	}
	warnings = append(warnings, g.projectOverlapWarnings(home, byDir)...)
	sort.Strings(warnings)
	return slices.Compact(warnings)
}

// maxListedSkills bounds how many skill names a warning spells out.
const maxListedSkills = 5

// abbreviate joins at most limit names and counts the rest.
func abbreviate(names []string, limit int) string {
	if len(names) <= limit {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:limit], ", "), len(names)-limit)
}

// duplicateIDs returns the skill ids that appear in more than one of dirs, sorted.
func duplicateIDs(byDir map[string][]string, dirs []string) []string {
	count := map[string]int{}
	for _, dir := range dirs {
		seen := map[string]bool{}
		for _, id := range byDir[dir] {
			if !seen[id] {
				seen[id] = true
				count[id]++
			}
		}
	}
	var out []string
	for id, n := range count {
		if n > 1 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// projectOverlapWarnings reports user-level skills whose name also exists in the
// project the command runs from, with the vendor's documented precedence.
func (g *Generator) projectOverlapWarnings(home string, byDir map[string][]string) []string {
	if g.projectDir == "" || filepath.Clean(g.projectDir) == filepath.Clean(home) {
		return nil
	}
	var warnings []string
	for _, entry := range userscope.Entries() {
		if entry.Kind != userscope.KindSkills {
			continue
		}
		for _, id := range byDir[entry.To] {
			projectSkill := filepath.Join(g.projectDir, filepath.FromSlash(entry.From), id, "SKILL.md")
			if _, err := os.Stat(projectSkill); err != nil {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("skill %q exists in the project (%s/%s) and at user level (~/%s/%s): %s",
				id, entry.From, id, entry.To, id, userscope.Precedence[entry.Preset]))
		}
	}
	return warnings
}
