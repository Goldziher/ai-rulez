package generator

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

const (
	claudeMDFile       = "CLAUDE.md"
	agentsOverrideName = "AGENTS.override.md"
	copilotRootFile    = ".github/copilot-instructions.md"
)

// skipMatcher decides, from a slash path relative to a config's base dir,
// whether skip_outputs excludes it. A nil matcher skips nothing.
type skipMatcher struct {
	cfg *config.Config
	// roots are the root instruction files presets write, relative to the base
	// dir (".rules", ".github/copilot-instructions.md", GEMINI.md, ...). A scope
	// root is one of these directly inside a configured scope path.
	roots map[string]bool
	// commandSkills are the directory names under a skills folder that hold a
	// command a harness runs as a skill, not a skill: a path alone cannot tell.
	commandSkills map[string]bool
}

// skipOutputsMatcher returns the matcher for cfg, or nil when skip_outputs is empty.
func skipOutputsMatcher(cfg *config.Config) *skipMatcher {
	if !cfg.HasSkipOutputs() {
		return nil
	}
	m := &skipMatcher{cfg: cfg, roots: map[string]bool{
		agentsOverrideName: true, copilotRootFile: true,
		claudeMDFile: true, string(config.SharedAgentsMD): true, "GEMINI.md": true,
		"CLAUDE.local.md": true, "AGENTS.local.md": true, "GEMINI.local.md": true,
	}}
	m.commandSkills = commandSkillIDs(cfg.Content)
	if cfg.Registry != nil {
		cfg.Registry.Each(func(_ string, gen config.PresetGenerator) {
			m.addPresetRoots(gen)
		})
	}
	return m
}

// commandSkillIDs lists the ids under which commands are written as skills: the
// sanitized command names, less the ones a skill already has (the skill wins and
// the command is not written).
func commandSkillIDs(content *config.ContentTree) map[string]bool {
	if content == nil {
		return nil
	}
	skills, commands := content.Skills, content.Commands
	for _, d := range content.Domains {
		if d != nil {
			skills, commands = append(skills, d.Skills...), append(commands, d.Commands...)
		}
	}
	ids := make(map[string]bool, len(commands))
	for i := range commands {
		ids[rulefiles.ID(commands[i].Name)] = true
	}
	for i := range skills {
		delete(ids, config.SkillID(skills[i]))
	}
	return ids
}

// addPresetRoots records the root instruction files of one preset: the spec's
// root file of a provider, its machine-local counterpart, the root file a shared
// AGENTS.md replaces, and any single-segment markdown path a hand-written preset
// reports.
func (m *skipMatcher) addPresetRoots(gen config.PresetGenerator) {
	if provider, ok := gen.(*providers.Generator); ok && provider.Spec.Root != nil && provider.Spec.Root.File != "" {
		m.roots[filepath.ToSlash(provider.Spec.Root.File)] = true
	}
	if local, ok := gen.(config.LocalRootProvider); ok && local.LocalRootFile() != "" {
		m.roots[filepath.ToSlash(local.LocalRootFile())] = true
	}
	for _, p := range gen.GetOutputPaths("") {
		rel := filepath.ToSlash(p)
		if !strings.Contains(rel, "/") && config.InferOutputKind(rel, "") == config.OutputKindRoot {
			m.roots[rel] = true
		}
	}
}

// isRoot reports whether rel is a root instruction file (or a machine-local
// variant of one), at the base dir or in a scope below it.
func (m *skipMatcher) isRoot(rel string) bool {
	for root := range m.roots {
		if rel == root {
			return true
		}
		// A scope root sits directly in a configured scope path. A file with a
		// root's name anywhere else (a skill resource, a rule, a docs page) is not
		// a root file.
		if dir, ok := strings.CutSuffix(rel, "/"+root); ok && m.isScopePath(dir) {
			return true
		}
	}
	return !strings.Contains(rel, "/") && config.InferOutputKind(rel, "") == config.OutputKindRoot
}

// isScopePath reports whether dir (slash separated, relative to the base dir) is
// the path of a configured scope.
func (m *skipMatcher) isScopePath(dir string) bool {
	return slices.ContainsFunc(m.cfg.Scopes, func(scope config.ScopeConfig) bool {
		return path.Clean(filepath.ToSlash(scope.Path)) == dir
	})
}

// skips reports whether skip_outputs excludes the output at rel (a path relative
// to the base dir, slash separated). isDir marks directory outputs.
func (m *skipMatcher) skips(rel string, isDir bool) bool {
	if m == nil {
		return false
	}
	rel = path.Clean(filepath.ToSlash(rel))
	if m.isRoot(rel) {
		return m.skipsRoot(path.Base(rel))
	}
	probe := rel
	if isDir {
		probe += "/_"
	}
	if m.skipsSharedSkills(probe) {
		return true
	}
	return m.skipsKind(m.outputKind(probe))
}

// outputKind is config.InferOutputKind, except that a skill output holding a
// command rendered as a skill is a command.
func (m *skipMatcher) outputKind(probe string) config.OutputKind {
	kind := config.InferOutputKind(probe, "")
	if kind != config.OutputKindSkill && kind != config.OutputKindResource {
		return kind
	}
	segments := strings.Split(probe, "/")
	for i, segment := range segments[:len(segments)-1] {
		if segment == "skills" {
			if m.commandSkills[segments[i+1]] {
				return config.OutputKindCommand
			}
			break
		}
	}
	return kind
}

// skipsRoot reports whether skip_outputs excludes the root instruction file whose
// base name is base.
func (m *skipMatcher) skipsRoot(base string) bool {
	switch {
	case m.cfg.SkipsOutput(config.SkipInstructions):
		return true
	case m.cfg.SkipsOutput(config.SkipClaudeMD) && base == claudeMDFile:
		return true
	case m.cfg.SkipsOutput(config.SkipAgentsMD) && (base == string(config.SharedAgentsMD) || base == agentsOverrideName):
		return true
	}
	return false
}

// skipsSharedSkills reports whether probe lies in the shared .agents/skills tree
// that skip_outputs = ["agents_md"] leaves out.
func (m *skipMatcher) skipsSharedSkills(probe string) bool {
	shared := string(config.SharedAgentSkills) + "/"
	return m.cfg.SkipsOutput(config.SkipAgentsMD) && (strings.HasPrefix(probe, shared) || strings.Contains(probe, "/"+shared))
}

// skipsKind reports whether skip_outputs excludes outputs of the given kind.
func (m *skipMatcher) skipsKind(kind config.OutputKind) bool {
	switch kind {
	case config.OutputKindRuleFile:
		return m.cfg.SkipsOutput(config.SkipRules)
	case config.OutputKindSkill, config.OutputKindResource:
		return m.cfg.SkipsOutput(config.SkipSkills)
	case config.OutputKindAgent:
		return m.cfg.SkipsOutput(config.SkipAgents)
	case config.OutputKindCommand:
		return m.cfg.SkipsOutput(config.SkipCommands)
	case config.OutputKindUnknown, config.OutputKindRoot, config.OutputKindLocalRoot,
		config.OutputKindSidecar, config.OutputKindDirectory:
	}
	return false
}

// dropSkippedKinds removes, from every preset, the outputs skip_outputs excludes,
// so the manifest, the gitignore block, drift checks, the plan, the lock digests
// and clean all see the same list. Output paths are relative to cfg.BaseDir.
func (g *Generator) dropSkippedKinds(allOutputs map[string][]config.OutputFile, cfg *config.Config) {
	matcher := skipOutputsMatcher(cfg)
	if matcher == nil {
		return
	}
	for name, outputs := range allOutputs {
		kept := outputs[:0:0]
		for _, output := range outputs {
			rel := output.Path
			if filepath.IsAbs(rel) {
				if r, err := filepath.Rel(cfg.BaseDir, rel); err == nil {
					rel = r
				}
			}
			if matcher.skips(rel, output.IsDir) {
				continue
			}
			kept = append(kept, output)
		}
		allOutputs[name] = kept
	}
}

// keepSkipped removes from files (absolute paths ai-rulez would delete) the ones
// skip_outputs now excludes. They stay on disk and leave the manifest: a file an
// earlier run generated is reported once as unmanaged, never deleted.
func (g *Generator) keepSkipped(files []string) []string {
	if !g.config.HasSkipOutputs() {
		return files
	}
	matcher := skipOutputsMatcher(g.config)
	return slices.DeleteFunc(files, func(abs string) bool {
		rel := g.relSlash(abs)
		if !matcher.skips(rel, false) {
			return false
		}
		g.noticeUnmanaged(rel)
		return true
	})
}

// noticeUnmanaged reports once that rel, generated by an earlier run, is now
// excluded by skip_outputs. A generate records the notice in the machine-local
// manifest (see unmanagedRecord), so later runs of generate, --check and clean
// stay quiet about the same file.
func (g *Generator) noticeUnmanaged(rel string) {
	if slices.Contains(g.readManifest(g.localManifestPath()).Unmanaged, rel) || slices.Contains(g.noticed, rel) {
		return
	}
	g.noticed = append(g.noticed, rel)
	g.config.Diag.Warn(rel+" was generated by an earlier run and is now excluded by skip_outputs: it is left in place and no longer managed by ai-rulez",
		"hint", "edit it by hand, or delete it if it is no longer needed")
}

// recordedUnmanaged is the notices the machine-local manifest holds. It does not
// cache the manifest as readManifest does: a write follows, and the readers after
// it must see the file as written.
func (g *Generator) recordedUnmanaged() []string {
	manifestPath := g.localManifestPath()
	if manifest, ok := g.manifests[manifestPath]; ok {
		return slices.Clone(manifest.Unmanaged)
	}
	if g.localManifestUntrusted() {
		return nil
	}
	return readManifestFile(g.log(), g.config.ReadExisting, manifestPath).Unmanaged
}

// unmanagedRecord is the notices the machine-local manifest keeps after this
// run: the earlier ones whose file is still there, and this run's.
func (g *Generator) unmanagedRecord() []string {
	var record []string
	for _, rel := range append(g.recordedUnmanaged(), g.noticed...) {
		if g.pathIsFile(filepath.Join(g.config.BaseDir, filepath.FromSlash(rel))) && !slices.Contains(record, rel) {
			record = append(record, rel)
		}
	}
	return record
}
