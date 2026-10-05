package providers

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/generator/presets"
)

// GlobalPaths is the user-scope layout of a provider with every path resolved to
// an absolute, OS-specific path. It is the type Go presets expose too (see
// presets.GlobalOutputProvider).
type GlobalPaths = presets.GlobalPaths

// GlobalPaths resolves the spec's [global] block against the user's home
// directory. getenv looks up home_env (pass os.Getenv). It returns nil when the
// spec declares no user-scope layout at all or home is not an absolute path;
// a relative home_env override is ignored (with a warning) in favour of home.
func (s *ProviderSpec) GlobalPaths(home string, getenv func(string) string) *GlobalPaths {
	if !filepath.IsAbs(home) {
		return nil
	}
	sidecars := make(map[string]string)
	var mcpSidecars map[string]string
	for _, sc := range s.Sidecars {
		if sc == nil {
			continue
		}
		if sc.GlobalPath != "" {
			sidecars[sc.Path] = s.Global.resolve(sc.GlobalPath, home, getenv)
		}
		if sc.GlobalMCPPath != "" {
			if mcpSidecars == nil {
				mcpSidecars = make(map[string]string)
			}
			mcpSidecars[sc.Path] = s.Global.resolve(sc.GlobalMCPPath, home, getenv)
		}
	}
	if s.Global == nil && len(sidecars) == 0 && len(mcpSidecars) == 0 {
		return nil
	}
	g := s.Global
	paths := &GlobalPaths{
		RootFile:    g.resolve(g.field(func(g *GlobalSpec) string { return g.RootFile }), home, getenv),
		SkillsDir:   g.resolve(g.field(func(g *GlobalSpec) string { return g.SkillsDir }), home, getenv),
		AgentsDir:   g.resolve(g.field(func(g *GlobalSpec) string { return g.AgentsDir }), home, getenv),
		CommandsDir: g.resolve(g.field(func(g *GlobalSpec) string { return g.CommandsDir }), home, getenv),
		RulesDir:    g.resolve(g.field(func(g *GlobalSpec) string { return g.RulesDir }), home, getenv),
		Sidecars:    sidecars,
		MCPSidecars: mcpSidecars,
	}
	if g != nil {
		paths.RelocatedHome = presets.HomeOverride(g.HomeEnv, getenv)
		paths.SkillPrecedence = g.SkillPrecedence
		for _, rel := range g.SkillReaders {
			paths.SkillReaders = append(paths.SkillReaders, g.resolve(rel, home, getenv))
		}
	}
	return paths
}

// GlobalOutputPaths makes the generator a presets.GlobalOutputProvider.
func (g *Generator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	return g.Spec.GlobalPaths(home, getenv)
}

// ProjectLayout makes the generator a presets.ProjectLayoutProvider: the root
// file and per-item output directories the spec renders into.
func (g *Generator) ProjectLayout() presets.ProjectLayout {
	layout := presets.ProjectLayout{}
	if g.Spec.Root != nil {
		layout.RootFile = g.Spec.Root.File
	}
	dir := func(typ string) string {
		if out := g.Spec.Outputs[typ]; out != nil && out.Mode == OutputModePerItemFile {
			return out.Dir
		}
		return ""
	}
	layout.SkillsDir, layout.AgentsDir, layout.CommandsDir, layout.RulesDir =
		dir("skills"), dir("agents"), dir("commands"), dir("rules")
	if layout.SkillsDir == "" && g.Spec.Outputs[OutputTypeSkills] == nil && g.Spec.Global != nil {
		// A user-level skills store with no project counterpart: a user-scope run
		// renders the skills at the store's own path (see userOnlyOutput).
		layout.SkillsDir = g.Spec.Global.SkillsDir
	}
	return layout
}

// field reads one path of a possibly nil block.
func (g *GlobalSpec) field(get func(*GlobalSpec) string) string {
	if g == nil {
		return ""
	}
	return get(g)
}

// resolve turns a home-relative path into an absolute one. When home_env is set
// in the environment, the part under home_dir is re-rooted at its value.
func (g *GlobalSpec) resolve(rel, home string, getenv func(string) string) string {
	if g == nil {
		return presets.ResolveHomePath(rel, "", "", home, getenv)
	}
	return presets.ResolveHomePath(rel, g.HomeEnv, g.HomeDir, home, getenv)
}
