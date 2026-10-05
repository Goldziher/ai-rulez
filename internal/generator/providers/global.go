package providers

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// GlobalPaths is the user-scope layout of a provider with every path resolved to
// an absolute, OS-specific path. A field is empty when the tool has no user-scope
// counterpart of that output.
type GlobalPaths struct {
	RootFile    string
	SkillsDir   string
	AgentsDir   string
	CommandsDir string
	RulesDir    string
	// Sidecars maps a sidecar's project path (SidecarSpec.Path) to its user-scope
	// path, for the sidecars that declare a global_path.
	Sidecars map[string]string
	// MCPSidecars maps a sidecar's project path to the user-scope file holding its
	// MCP servers, for the sidecars whose servers do not live at their Sidecars
	// path (global_mcp_path). A user-scope run writes the MCP member there and the
	// rest of the sidecar to Sidecars. Nil when no sidecar declares one.
	MCPSidecars map[string]string
}

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
	return &GlobalPaths{
		RootFile:    g.resolve(g.field(func(g *GlobalSpec) string { return g.RootFile }), home, getenv),
		SkillsDir:   g.resolve(g.field(func(g *GlobalSpec) string { return g.SkillsDir }), home, getenv),
		AgentsDir:   g.resolve(g.field(func(g *GlobalSpec) string { return g.AgentsDir }), home, getenv),
		CommandsDir: g.resolve(g.field(func(g *GlobalSpec) string { return g.CommandsDir }), home, getenv),
		RulesDir:    g.resolve(g.field(func(g *GlobalSpec) string { return g.RulesDir }), home, getenv),
		Sidecars:    sidecars,
		MCPSidecars: mcpSidecars,
	}
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
	if rel == "" {
		return ""
	}
	if g != nil && g.HomeEnv != "" && getenv != nil {
		override := getenv(g.HomeEnv)
		if override != "" && !filepath.IsAbs(override) {
			logger.Warn("ignoring relative home override; using the home directory", "env", g.HomeEnv, "value", override)
			override = ""
		}
		if override != "" && isUnder(rel, g.HomeDir) {
			rest := strings.TrimPrefix(strings.TrimPrefix(rel, strings.TrimSuffix(g.HomeDir, "/")), "/")
			return filepath.Join(override, filepath.FromSlash(path.Clean(rest)))
		}
	}
	return filepath.Join(home, filepath.FromSlash(rel))
}
