package presets

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
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
}

// Resolve turns the layout into absolute paths under home.
func (l GlobalLayout) Resolve(home string, getenv func(string) string) *GlobalPaths {
	if !filepath.IsAbs(home) {
		return nil
	}
	resolve := func(rel string) string { return ResolveHomePath(rel, l.HomeEnv, l.HomeDir, home, getenv) }
	paths := &GlobalPaths{
		RootFile:    resolve(l.RootFile),
		SkillsDir:   resolve(l.SkillsDir),
		AgentsDir:   resolve(l.AgentsDir),
		CommandsDir: resolve(l.CommandsDir),
		RulesDir:    resolve(l.RulesDir),
		Sidecars:    map[string]string{},
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
	if homeEnv != "" && getenv != nil {
		override := getenv(homeEnv)
		if override != "" && !filepath.IsAbs(override) {
			logger.Warn("ignoring relative home override; using the home directory", "env", homeEnv, "value", override)
			override = ""
		}
		if override != "" && underDir(rel, homeDir) {
			rest := strings.TrimPrefix(strings.TrimPrefix(rel, strings.TrimSuffix(homeDir, "/")), "/")
			return filepath.Join(override, filepath.FromSlash(path.Clean(rest)))
		}
	}
	return filepath.Join(home, filepath.FromSlash(rel))
}

// underDir reports whether p is dir or lies inside it (slash-separated, relative).
func underDir(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}
