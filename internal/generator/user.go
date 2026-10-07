package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/userscope"
)

// User scope renders the person's own configuration (not a project's) into the
// per-user directories each harness reads: ~/.claude, ~/.agents/skills,
// ~/.codex, ~/.gemini, ~/.config/opencode, ~/.copilot, ~/.pi/agent, ... It reuses
// the preset renderers and maps their project-relative output through the layout
// each preset declares (its spec's [global] block, or presets.GlobalOutputProvider),
// so only locations a vendor documents are written. A tool's home variable
// (CODEX_HOME, HERMES_HOME, ...) relocates its directory, and only an absolute
// value is honoured.
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
	// Dropped counts project outputs with no documented user-level destination;
	// Unmapped names them as "preset: project-relative path", sorted.
	Dropped  int
	Unmapped []string
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
	// NewGenerator added the generated-file guard before the scope was known.
	g.config.DropGuardHooks()
}

// IsUserScope reports whether the Generator runs in user scope.
func (g *Generator) IsUserScope() bool { return g.userMode }

// SetUserEnv sets the environment lookup the layouts' home variables are read
// through (default: the host environment); tests use it to relocate a tool's home.
func (g *Generator) SetUserEnv(getenv func(string) string) { g.userGetenv = getenv }

func (g *Generator) userEnv() func(string) string {
	if g.userGetenv != nil {
		return g.userGetenv
	}
	return g.host().GetEnv
}

// resolveUserLayouts resolves the user-scope layout of every built-in preset
// below the home directory, and records the tool homes environment variables
// relocated outside it, for the configured presets. It returns the supported layouts and the reason for each
// unsupported preset.
func (g *Generator) resolveUserLayouts() (map[string]*userscope.Layout, map[string]string, error) {
	layouts, unsupported, err := userscope.AllFor(g.config, g.config.BaseDir, g.userEnv())
	if err != nil {
		return nil, nil, oops.With("home", g.config.BaseDir).
			Hint("The home directory must be an absolute path").Wrapf(err, "resolve the user-level layouts")
	}
	g.userLayouts = layouts
	g.userHomes = nil
	// Only a configured preset may widen the writable scope: a home variable of a
	// tool the user does not generate for says nothing about where to write.
	for _, preset := range userPresets(g.config, layouts) {
		if home := layouts[preset.BuiltIn].RelocatedHome; home != "" && !slices.Contains(g.userHomes, home) {
			g.userHomes = append(g.userHomes, home)
		}
	}
	return layouts, unsupported, nil
}

// withinScope reports whether abs lies where the run may touch files: below the
// base directory (the home directory in user scope) or, in user scope, below a
// tool home an environment variable relocated.
func (g *Generator) withinScope(abs string) bool {
	if isUnderBaseDir(g.config.BaseDir, abs) {
		return true
	}
	return g.userMode && slices.ContainsFunc(g.userHomes, func(home string) bool { return isUnderBaseDir(home, abs) })
}

// SetProjectDir names the project the user runs from, so skills that exist at
// both levels can be reported. Empty disables the check.
func (g *Generator) SetProjectDir(dir string) { g.projectDir = dir }

// collectUserOutputs renders every configured preset and maps the result to the
// user-level destinations of the preset's layout.
//
// The presets render into a scratch directory, not the home directory: a preset
// merges into the existing settings document at the path it renders, and a tool
// whose home variable relocates that document (CODEX_HOME) must merge into the
// relocated file. The scratch directory holds a copy of each such document at its
// project-relative path; the mapped output is then written to the real location.
func (g *Generator) collectUserOutputs(profile string) (outputs []config.OutputFile, activeProfile string, dropped []string, err error) {
	activeProfile = g.resolveProfile(profile)
	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, "", nil, err
	}
	layouts, unsupported, err := g.resolveUserLayouts()
	if err != nil {
		return nil, "", nil, err
	}
	presets := userPresets(g.config, layouts)
	for _, name := range userUnsupportedPresets(g.config, layouts) {
		reason := unsupported[name]
		if reason == "" {
			reason = "it is not a built-in preset"
		}
		g.config.Diag.Warn("preset "+name+" has no documented user-level location, so --user writes nothing for it ("+reason+")",
			"hint", "supported: "+strings.Join(userscope.Supported(layouts), ", "))
	}
	if g.config.GeneratedAt.IsZero() {
		g.config.GeneratedAt = config.ResolveGenerationTimeIn(g.host())
	}

	stage, err := os.MkdirTemp("", "ai-rulez-user-")
	if err != nil {
		return nil, "", nil, oops.Wrapf(err, "create the scratch directory")
	}
	defer func() {
		if err := os.RemoveAll(stage); err != nil {
			g.log().Debug("Could not remove the scratch directory", "path", stage, "error", err)
		}
	}()
	if err := g.stageUserDocuments(stage, presets, layouts); err != nil {
		return nil, "", nil, err
	}

	tempCfg := *g.config
	tempCfg.BaseDir = stage
	tempCfg.Content = contentTree
	tempCfg.MCPServers = nil
	tempCfg.MCP = nil
	tempCfg.Scopes = nil
	tempCfg.AgentsMD = false
	tempCfg.UserScope = true
	tempCfg.Presets = presets
	run := config.NewRunState()
	previous, merged := g.userPreviousAliases(presets, layouts)
	run.SetPreviouslyGenerated(previous)
	run.SetPreviousMerged(merged)
	tempCfg.Run = run
	tempCfg.SourceHash = computeSourceHash(&tempCfg, contentTree)
	g.config.SourceHash = tempCfg.SourceHash

	rendered, err := g.renderUserPresets(&tempCfg, stage, presets)
	if err != nil {
		return nil, "", nil, err
	}
	mapped, dropped, err := g.mapUserOutputs(stage, rendered, layouts)
	if err != nil {
		return nil, "", nil, err
	}
	outputs, err = flattenPresetOutputs(g.config.Diag, g.log(), g.config.ReadExisting, mapped)
	if err != nil {
		return nil, "", nil, err
	}
	g.carryClaimAnnotations(outputs)
	g.reclaimStaleMembers(outputs)
	sort.Strings(dropped)
	return outputs, activeProfile, dropped, nil
}

// userStageDir is the scratch directory one preset renders into. Each preset has
// its own, so two presets that render the same project-relative path (and map it
// to different user-level files) never read each other's staged document.
func userStageDir(stage, preset string) string { return filepath.Join(stage, preset) }

// renderUserPresets renders every preset into its own scratch directory below
// stage. cfg keeps the full preset list, so presets that adapt to the others
// (a shared AGENTS.md) behave as in a combined run.
func (g *Generator) renderUserPresets(cfg *config.Config, stage string, presets []config.Preset) (map[string][]config.OutputFile, error) {
	rendered := make(map[string][]config.OutputFile, len(presets))
	for _, preset := range presets {
		gen, err := g.config.Registry.Generator(preset.BuiltIn)
		if err != nil {
			return nil, oops.With("preset", preset.GetName()).Wrapf(err, "resolve preset")
		}
		presetCfg := *cfg
		presetCfg.BaseDir = userStageDir(stage, preset.BuiltIn)
		outputs, err := gen.Generate(cfg.Content, presetCfg.BaseDir, &presetCfg)
		if err != nil {
			return nil, oops.With("preset", preset.GetName()).Wrapf(err, "generate presets")
		}
		rendered[preset.GetName()] = outputs
		presetCfg.Analysis.Attribute(preset.GetName(), presetCfg.BaseDir, outputs)
	}
	return rendered, nil
}

// mapUserOutputs moves the outputs rendered below stage to their user-level
// destinations and names the ones with none ("preset: project-relative path").
func (g *Generator) mapUserOutputs(stage string, rendered map[string][]config.OutputFile, layouts map[string]*userscope.Layout,
) (mapped map[string][]config.OutputFile, dropped []string, err error) {
	mapped = make(map[string][]config.OutputFile, len(rendered))
	for preset, outs := range rendered {
		layout := layouts[preset]
		for _, output := range outs {
			if output.LocalOnly {
				dropped = append(dropped, preset+": "+filepath.ToSlash(g.convertToRelativePath(output.Path)))
				continue
			}
			rel, relErr := filepath.Rel(userStageDir(stage, preset), output.Path)
			if relErr != nil {
				return nil, nil, oops.With("path", output.Path).Wrapf(relErr, "locate a rendered output")
			}
			dest, _, ok := layout.Map(filepath.ToSlash(rel))
			if !ok {
				if !output.IsDir {
					dropped = append(dropped, preset+": "+filepath.ToSlash(rel))
				}
				continue
			}
			output.Path = dest
			mapped[preset] = append(mapped[preset], output)
		}
	}
	return mapped, dropped, nil
}

// stageUserDocuments copies the existing settings documents the presets merge
// into from their user-level location to the path they render at.
func (g *Generator) stageUserDocuments(stage string, presets []config.Preset, layouts map[string]*userscope.Layout) error {
	for _, preset := range presets {
		for _, row := range layouts[preset.BuiltIn].Rows {
			if row.Kind != userscope.KindSettings || row.To == "" {
				continue
			}
			data, err := g.config.ReadExisting(row.To)
			if err != nil {
				continue // absent, or not readable: the merge starts from nothing, and the guard reports the path
			}
			target := filepath.Join(userStageDir(stage, preset.BuiltIn), filepath.FromSlash(row.From))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return oops.Wrapf(err, "create the scratch directory")
			}
			if err := os.WriteFile(target, data, 0o600); err != nil {
				return oops.Wrapf(err, "stage %s", row.To)
			}
		}
	}
	return nil
}

// userPreviousAliases returns the previous run's generated files and merge claims
// under the project-relative paths the presets render at. The manifest records the
// user-level destinations, and a preset looks its own files up by what it renders.
func (g *Generator) userPreviousAliases(presets []config.Preset, layouts map[string]*userscope.Layout) ([]string, map[string][]jsonmerge.Claim) {
	files := g.previousManifestFiles()
	claims := g.previousMergedClaims()
	generated := slices.Clone(files)
	merged := make(map[string][]jsonmerge.Claim, len(claims))
	for rel, c := range claims {
		merged[rel] = c
	}
	for _, preset := range presets {
		for _, row := range layouts[preset.BuiltIn].Rows {
			if row.To == "" {
				continue
			}
			dest := filepath.ToSlash(g.convertToRelativePath(row.To))
			if dest == row.From {
				continue
			}
			for _, f := range files {
				switch {
				case f == dest:
					generated = append(generated, row.From)
				case row.Dir && strings.HasPrefix(f, dest+"/"):
					generated = append(generated, row.From+strings.TrimPrefix(f, dest))
				}
			}
			if c, ok := claims[dest]; ok {
				merged[row.From] = c
			}
		}
	}
	return generated, merged
}

// userPresets keeps the configured presets that have a user-level layout.
func userPresets(cfg *config.Config, layouts map[string]*userscope.Layout) []config.Preset {
	var out []config.Preset
	for _, p := range cfg.Presets {
		if p.IsBuiltIn() && layouts[p.BuiltIn] != nil {
			out = append(out, p)
		}
	}
	return out
}

// userUnsupportedPresets names the configured presets user scope cannot write
// for (the shared mcp preset is not a harness and is not reported).
func userUnsupportedPresets(cfg *config.Config, layouts map[string]*userscope.Layout) []string {
	var out []string
	for _, p := range cfg.Presets {
		name := p.GetName()
		if name == string(config.PresetMCP) || (p.IsBuiltIn() && layouts[name] != nil) {
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
	if _, err := filepath.EvalSymlinks(g.config.BaseDir); err != nil {
		return nil, nil, oops.With("home", g.config.BaseDir).Wrapf(err, "resolve the home directory")
	}

	skippedDirs := map[string]bool{}
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		if !g.withinScope(abs) {
			return nil, nil, oops.With("path", abs).Errorf("user-level output %s is outside the home directory", abs)
		}
		if err := g.checkSymlinkEscape(abs); err != nil {
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
// outside the resolved root it lies in: the home directory, or a tool home an
// environment variable relocated.
func (g *Generator) checkSymlinkEscape(abs string) error {
	root := g.config.BaseDir
	if !isUnderBaseDir(root, abs) {
		i := slices.IndexFunc(g.userHomes, func(home string) bool { return isUnderBaseDir(home, abs) })
		if i < 0 {
			return nil // withinScope has refused it already
		}
		root = g.userHomes[i]
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root // a relocated home that does not exist yet holds no symlink
	}
	probe := abs
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe || !isUnderBaseDir(root, parent) {
			return nil
		}
		probe = parent
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return oops.With("path", abs).Wrapf(err, "resolve %s", probe)
	}
	if !isUnderBaseDir(realRoot, resolved) {
		return oops.With("path", abs).With("resolves_to", resolved).
			Hint("A symlink below the home directory points out of it; generate --user writes only inside the home directory").
			Errorf("%s resolves outside the home directory", abs)
	}
	return nil
}

// userConflict explains why an existing file at abs must not be written, or "".
func (g *Generator) userConflict(abs string, output config.OutputFile, previous map[string]bool) string {
	info, err := g.config.LstatExisting(abs)
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
	data, err := g.config.ReadExisting(abs)
	if err != nil {
		return false
	}
	if output.RawContent != nil {
		return bytes.Equal(data, output.RawContent)
	}
	return string(data) == g.finalContent(output) || hasGeneratedBanner(abs, data)
}

// userStaleOK reports whether a file the manifest lists may still be removed: it
// must not have been replaced by the user since ai-rulez wrote it. A file that can
// carry a header must still look generated, whatever the header mode. A headerless
// one is trusted to the manifest only where ai-rulez owns it outright: inside a
// content folder a layout owns (a skill's resources), or at the exact file row of
// a layout, where a text file (rules, plugin modules, hook scripts) must still
// carry the generated banner and a JSON document (Copilot's hooks file, which has
// no room for one) rests on the manifest alone.
func (g *Generator) userStaleOK(abs string) bool {
	info, err := g.config.LstatExisting(abs)
	if err != nil || info.IsDir() || !g.userMayTouch(filepath.Dir(abs)) {
		return false
	}
	if headerCapable(abs) {
		return g.looksGenerated(abs)
	}
	if g.userInContentFolder(abs) {
		return true
	}
	if !g.userFileRow(abs) {
		return false
	}
	return strings.EqualFold(filepath.Ext(abs), ".json") || g.looksGenerated(abs)
}

// userFileRow reports whether abs is the destination of a file row of some
// user-level layout.
func (g *Generator) userFileRow(abs string) bool {
	abs = filepath.Clean(abs)
	for _, layout := range g.userLayouts {
		for _, row := range layout.Rows {
			if !row.Dir && row.To != "" && abs == filepath.Clean(row.To) {
				return true
			}
		}
	}
	return false
}

// userMayTouch reports whether abs, which must exist or have an existing ancestor,
// lies in scope and resolves inside the home directory (or a relocated tool home).
func (g *Generator) userMayTouch(abs string) bool {
	if !g.withinScope(abs) {
		return false
	}
	if err := g.checkSymlinkEscape(abs); err != nil {
		g.log().Warn("Skipping a path that resolves outside the home directory", "path", abs)
		return false
	}
	return true
}

// userInContentFolder reports whether abs lies below a content folder (a directory
// row) of any user-level layout.
func (g *Generator) userInContentFolder(abs string) bool {
	return g.userDestination(abs, true)
}

// userDestination reports whether abs is a user-level destination of some layout:
// a file row's path, or anything below a directory row's. dirOnly restricts it to
// directory rows.
func (g *Generator) userDestination(abs string, dirOnly bool) bool {
	abs = filepath.Clean(abs)
	for _, layout := range g.userLayouts {
		for _, row := range layout.Rows {
			if row.To == "" || (dirOnly && !row.Dir) {
				continue
			}
			if abs == filepath.Clean(row.To) && !row.Dir {
				return true
			}
			if row.Dir && abs != filepath.Clean(row.To) && isUnderBaseDir(row.To, abs) {
				return true
			}
		}
	}
	return false
}

// userManifestEntryOK vets a manifest entry before it is trusted for removal. The
// manifest is a plain file in the user's config directory, so an entry is treated
// as untrusted input: it must not climb with ".." (except into a relocated tool
// home, which a manifest records relative to the home directory), and it must be
// a destination some layout writes to.
func (g *Generator) userManifestEntryOK(rel, abs string) bool {
	if slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "..") &&
		!slices.ContainsFunc(g.userHomes, func(home string) bool { return isUnderBaseDir(home, abs) }) {
		g.log().Warn("Ignoring a manifest entry that climbs out of the home directory", "path", rel)
		return false
	}
	if !g.userDestination(abs, false) {
		g.log().Warn("Ignoring a manifest entry that is not a user-level destination", "path", rel)
		return false
	}
	return true
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
	g.mu.Lock()
	defer g.mu.Unlock()
	plan, _, err := g.planUser(profile)
	return plan, err
}

func (g *Generator) planUser(profile string) (*UserPlan, []config.OutputFile, error) {
	g.beginRun()
	d := g.diagnostics()
	d.Reset()
	defer d.Flush()

	outputs, active, dropped, err := g.collectUserOutputs(profile)
	if err != nil {
		return nil, nil, err
	}
	kept, skips, err := g.guardUserOutputs(outputs)
	if err != nil {
		return nil, nil, err
	}
	plan := &UserPlan{Profile: active, Skips: skips, Dropped: len(dropped), Unmapped: dropped}
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
	g.mu.Lock()
	defer g.mu.Unlock()
	plan, kept, err := g.planUser(profile)
	if err != nil {
		return nil, err
	}
	g.beginRun()
	g.log().Info("Generating user-level configuration", "profile", plan.Profile, "home", g.config.BaseDir)

	stale := g.userStale(kept)
	g.noteCreatedUserDirs(kept)
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
	g.log().Info("User-level generation complete", "files", len(plan.Writes)+len(plan.Merges))
	return plan, nil
}

// userPruneCandidates lists the directories above removed files that clean may
// remove once empty. A directory qualifies when it lies inside a content folder
// some preset owns (~/.claude/skills and below), or is deeper than the first level
// below the home directory (~/.copilot/hooks, ~/.config/devin): the directories at
// the first level (~/.claude, ~/.codex, ~/.config), a relocated tool home and the
// user config directory are never candidates. Only empty directories are removed.
func (g *Generator) userPruneCandidates(removed []string) []string {
	var roots []string
	for _, layout := range g.userLayouts {
		for _, root := range layout.Roots() {
			// A root is a content folder; the home directory or a parent of it never is.
			if !isUnderBaseDir(root, g.config.BaseDir) && !slices.Contains(g.userHomes, root) {
				roots = append(roots, root)
			}
		}
	}
	if g.userDirs == nil {
		g.loadUserDirs()
	}
	home := filepath.Clean(g.config.BaseDir)
	keep := func(dir string) bool {
		if slices.Contains(g.userHomes, dir) || dir == home || filepath.Dir(dir) == home {
			return true
		}
		return g.config.ConfigDir != "" && isUnderBaseDir(dir, g.config.ConfigDir)
	}
	seen := map[string]bool{}
	var dirs []string
	for _, file := range removed {
		for dir := filepath.Dir(file); g.withinScope(dir); dir = filepath.Dir(dir) {
			if g.userDirsRecorded && isUnderBaseDir(home, dir) {
				// The manifest says which directories ai-rulez created: those go once
				// empty, wherever they sit, and every other one stays.
				if !g.userDirs[dir] || !g.userDirEligible(dir) {
					break
				}
				if !seen[dir] {
					seen[dir] = true
					dirs = append(dirs, dir)
				}
				continue
			}
			inRoot := slices.ContainsFunc(roots, func(root string) bool { return isUnderBaseDir(root, dir) })
			if keep(dir) && !inRoot {
				break
			}
			if g.config.ConfigDir != "" && isUnderBaseDir(dir, g.config.ConfigDir) {
				break // the user config lives here; never an output folder
			}
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// userDirEligible reports whether dir may be recorded as created or removed as
// such: strictly below the home directory and outside the user config.
func (g *Generator) userDirEligible(dir string) bool {
	home := filepath.Clean(g.config.BaseDir)
	dir = filepath.Clean(dir)
	if dir == home || !isUnderBaseDir(home, dir) || slices.Contains(g.userHomes, dir) {
		return false
	}
	return g.config.ConfigDir == "" || !isUnderBaseDir(g.config.ConfigDir, dir)
}

// userMayRemoveDir reports whether dir may be removed once empty. Below the home
// directory that is only a directory ai-rulez recorded as created, so one that
// existed before (an empty ~/.claude/skills) stays; a manifest that predates the
// record, and a relocated tool home, fall back to the content-folder rules.
func (g *Generator) userMayRemoveDir(dir string) bool {
	if g.userDirs == nil {
		g.loadUserDirs()
	}
	if !g.userDirsRecorded || !isUnderBaseDir(g.config.BaseDir, dir) {
		return true
	}
	return g.userDirs[filepath.Clean(dir)] && g.userDirEligible(dir)
}

// loadUserDirs reads the directories the previous run recorded as created,
// trusting only entries that stay below the home directory.
func (g *Generator) loadUserDirs() {
	g.userDirs, g.userDirsRecorded = map[string]bool{}, false
	recorded := g.readManifest(g.manifestPath()).Dirs
	if recorded == nil {
		return
	}
	g.userDirsRecorded = true
	for _, rel := range *recorded {
		if slices.Contains(strings.Split(rel, "/"), "..") || filepath.IsAbs(filepath.FromSlash(rel)) {
			g.log().Warn("Ignoring a manifest directory that climbs out of the home directory", "path", rel)
			continue
		}
		if abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(rel)); g.userDirEligible(abs) {
			g.userDirs[abs] = true
		}
	}
}

// noteCreatedUserDirs adds to the recorded set every directory the outputs will
// need that does not exist yet: those are ai-rulez's to remove again, and the
// ones that exist already are not.
func (g *Generator) noteCreatedUserDirs(outputs []config.OutputFile) {
	g.loadUserDirs()
	for _, output := range outputs {
		dir := g.absOutputPath(output.Path)
		if !output.IsDir {
			dir = filepath.Dir(dir)
		}
		var missing []string
		for ; g.userDirEligible(dir); dir = filepath.Dir(dir) {
			if _, err := os.Lstat(dir); err == nil {
				break
			}
			missing = append(missing, dir)
		}
		for _, d := range missing {
			g.userDirs[d] = true
		}
	}
	// A manifest from before directories were recorded gets a record from here on.
	g.userDirsRecorded = true
}

// manifestDirs is the directory record the user manifest at path carries: the
// recorded directories that still exist, or nil outside user scope.
func (g *Generator) manifestDirs(path string) *[]string {
	if !g.userMode || path != g.manifestPath() || !g.userDirsRecorded {
		return nil
	}
	dirs := []string{}
	for dir := range g.userDirs {
		if info, err := os.Lstat(dir); err == nil && info.IsDir() {
			if rel, relErr := filepath.Rel(g.config.BaseDir, dir); relErr == nil {
				dirs = append(dirs, filepath.ToSlash(rel))
			}
		}
	}
	sort.Strings(dirs)
	return &dirs
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
	byDir := map[string][]string{} // user-level skill dir (absolute) -> skill ids
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		if output.IsDir || filepath.Base(abs) != "SKILL.md" {
			continue
		}
		dir := filepath.Dir(filepath.Dir(abs))
		byDir[dir] = append(byDir[dir], filepath.Base(filepath.Dir(abs)))
	}
	var warnings []string
	for _, preset := range userPresets(g.config, g.userLayouts) {
		layout := g.userLayouts[preset.BuiltIn]
		var dirs []string
		for _, dir := range layout.SkillReaders {
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
		shown := make([]string, len(dirs))
		for i, dir := range dirs {
			shown[i] = g.displayUserPath(dir)
		}
		warnings = append(warnings, fmt.Sprintf("%s reads %s, so these skills load twice: %s",
			preset.BuiltIn, strings.Join(shown, ", "), abbreviate(dup, maxListedSkills)))
	}
	warnings = append(warnings, g.projectOverlapWarnings(byDir)...)
	sort.Strings(warnings)
	return slices.Compact(warnings)
}

// displayUserPath writes a path below the home directory as ~/relative.
func (g *Generator) displayUserPath(abs string) string {
	if isUnderBaseDir(g.config.BaseDir, abs) {
		if rel, err := filepath.Rel(g.config.BaseDir, abs); err == nil {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return abs
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
func (g *Generator) projectOverlapWarnings(byDir map[string][]string) []string {
	if g.projectDir == "" || filepath.Clean(g.projectDir) == filepath.Clean(g.config.BaseDir) {
		return nil
	}
	var warnings []string
	for _, preset := range userPresets(g.config, g.userLayouts) {
		layout := g.userLayouts[preset.BuiltIn]
		for _, row := range layout.Rows {
			if row.Kind != userscope.KindSkills || row.To == "" {
				continue
			}
			for _, id := range byDir[row.To] {
				projectSkill := filepath.Join(g.projectDir, filepath.FromSlash(row.From), id, "SKILL.md")
				if _, err := os.Stat(projectSkill); err != nil {
					continue
				}
				warnings = append(warnings, fmt.Sprintf("skill %q exists in the project (%s/%s) and at user level (%s/%s): %s",
					id, row.From, id, g.displayUserPath(row.To), id, layout.Precedence()))
			}
		}
	}
	return warnings
}
