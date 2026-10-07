package presets

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// GlobalPaths is the user-scope layout of a preset with every path resolved to
// an absolute, OS-specific path. A field is empty when the tool has no user-scope
// counterpart of that output. Provider specs expose the same type through
// ProviderSpec.GlobalPaths, so a --global run treats DSL and Go presets alike.
type GlobalPaths struct {
	RootFile    string
	SkillsDir   string
	AgentsDir   string
	CommandsDir string
	RulesDir    string
	// Sidecars maps a document's project path (base-relative, slash-separated)
	// to its user-scope path.
	Sidecars map[string]string
	// MCPSidecars maps a document's project path to the user-scope file holding
	// its MCP servers, for tools whose servers do not live at the Sidecars path.
	MCPSidecars map[string]string
	// SkillReaders lists every user-level skill directory the tool reads (its own
	// SkillsDir included), absolute. Two of them holding the same skill name load
	// it twice. Empty when the tool is only known to read SkillsDir.
	SkillReaders []string
	// RelocatedHome is the absolute directory the tool's home environment variable
	// (HomeEnv) points at, when it is set; paths below HomeDir were re-rooted there.
	// Empty when the tool lives under the user's home.
	RelocatedHome string
	// IgnoredHomeEnv and IgnoredHomeValue name a home environment variable that
	// was set to a relative path and so ignored; the caller reports it.
	IgnoredHomeEnv, IgnoredHomeValue string
	// SkillPrecedence says which copy runs when a skill of the same name exists at
	// user and project level, as the vendor documents it. Empty means the vendor
	// documents none.
	SkillPrecedence string
}

// ProjectLayout is where a preset writes the project-level counterparts of the
// GlobalPaths fields, relative to the project root and slash-separated. User
// scope maps an output below one of these onto the matching GlobalPaths field.
type ProjectLayout struct {
	RootFile, SkillsDir, AgentsDir, CommandsDir, RulesDir string
}

// ProjectLayoutProvider is implemented by every preset generator that can be
// mapped into user scope, next to GlobalOutputProvider.
type ProjectLayoutProvider interface {
	ProjectLayout() ProjectLayout
}

// ConfiguredProjectLayoutProvider is implemented by a preset whose project layout
// depends on the config (codex_skills_dir). It takes precedence over ProjectLayout.
type ConfiguredProjectLayoutProvider interface {
	ProjectLayoutFor(cfg *config.Config) ProjectLayout
}

// GlobalOutputProvider is implemented by a preset generator that knows its
// user-scope layout. home must be absolute; getenv looks up the layout's HomeEnv
// (pass os.Getenv). It returns nil when the tool has no user scope.
type GlobalOutputProvider interface {
	GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths
}

// GlobalLayout declares a Go preset's user-scope layout. Every path is relative
// to the home directory and slash-separated, like providers.GlobalSpec.
type GlobalLayout struct {
	// HomeEnv relocates the part of a path under HomeDir (CODEX_HOME).
	HomeEnv, HomeDir string

	RootFile, SkillsDir, AgentsDir, CommandsDir, RulesDir string

	Sidecars, MCPSidecars map[string]string

	// SkillReaders and SkillPrecedence are described on GlobalPaths; SkillReaders
	// are home-relative here.
	SkillReaders    []string
	SkillPrecedence string
}

// Resolve turns the layout into absolute paths under home.
func (l GlobalLayout) Resolve(home string, getenv func(string) string) *GlobalPaths {
	if !filepath.IsAbs(home) {
		return nil
	}
	resolve := func(rel string) string { return ResolveHomePath(rel, l.HomeEnv, l.HomeDir, home, getenv) }
	paths := &GlobalPaths{
		RelocatedHome: HomeOverride(l.HomeEnv, getenv),
		RootFile:      resolve(l.RootFile),
		SkillsDir:     resolve(l.SkillsDir),
		AgentsDir:     resolve(l.AgentsDir),
		CommandsDir:   resolve(l.CommandsDir),
		RulesDir:      resolve(l.RulesDir),
		Sidecars:      map[string]string{},

		SkillPrecedence: l.SkillPrecedence,
	}
	if ignored := IgnoredHomeOverride(l.HomeEnv, getenv); ignored != "" {
		paths.IgnoredHomeEnv, paths.IgnoredHomeValue = l.HomeEnv, ignored
	}
	for _, rel := range l.SkillReaders {
		paths.SkillReaders = append(paths.SkillReaders, resolve(rel))
	}
	for project, rel := range l.Sidecars {
		paths.Sidecars[project] = resolve(rel)
	}
	if len(l.MCPSidecars) > 0 {
		paths.MCPSidecars = map[string]string{}
		for project, rel := range l.MCPSidecars {
			paths.MCPSidecars[project] = resolve(rel)
		}
	}
	return paths
}

// ResolveHomePath turns a home-relative path into an absolute one. When homeEnv
// is set in the environment, the part under homeDir is re-rooted at its value.
// An empty rel stays empty.
func ResolveHomePath(rel, homeEnv, homeDir, home string, getenv func(string) string) string {
	if rel == "" {
		return ""
	}
	if override := HomeOverride(homeEnv, getenv); override != "" && underDir(rel, homeDir) {
		rest := strings.TrimPrefix(strings.TrimPrefix(rel, strings.TrimSuffix(homeDir, "/")), "/")
		return filepath.Join(override, filepath.FromSlash(path.Clean(rest)))
	}
	return filepath.Join(home, filepath.FromSlash(rel))
}

// HomeOverride returns the absolute directory the home-relocating variable homeEnv
// is set to, or "" when it is unset, empty or not absolute (a relative value is
// ignored with a warning: it would depend on the working directory).
func HomeOverride(homeEnv string, getenv func(string) string) string {
	if homeEnv == "" || getenv == nil {
		return ""
	}
	override := getenv(homeEnv)
	if override == "" {
		return ""
	}
	if !filepath.IsAbs(override) {
		return ""
	}
	return filepath.Clean(override)
}

// IgnoredHomeOverride returns the value homeEnv is set to when HomeOverride
// ignored it for being a relative path (it would depend on the working
// directory); "" otherwise.
func IgnoredHomeOverride(homeEnv string, getenv func(string) string) string {
	if homeEnv == "" || getenv == nil {
		return ""
	}
	if override := getenv(homeEnv); override != "" && !filepath.IsAbs(override) {
		return override
	}
	return ""
}

// underDir reports whether p is dir or lies inside it (slash-separated, relative).
func underDir(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}
