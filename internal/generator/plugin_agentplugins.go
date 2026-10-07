package generator

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// AgentPluginFindings builds the Agent Plugins package of a single-plugin project
// in memory and returns what the specification check reports: content a
// conformant client would skip (and `generate --plugin` therefore leaves out),
// and any problem in the files that are written. A project that does not build
// an Agent Plugins package, or a marketplace of members and domain plugins
// (whose members are checked when they are published), has none.
func (g *Generator) AgentPluginFindings(profile string) ([]lint.AgentPluginFinding, error) {
	g.diagnostics()
	if g.config.Plugin == nil {
		return nil, nil
	}
	if mkt := g.config.Marketplace; mkt != nil && (len(mkt.Members) > 0 || mkt.HasDomainPlugins()) {
		return nil, nil
	}
	if g.config.HasLocalInputs() && g.config.ConfigDir != "" && g.config.ConfigFile != "" {
		shared, err := g.sharedPluginView()
		if err != nil {
			return nil, err
		}
		return shared.AgentPluginFindings(profile)
	}
	manifest, err := g.buildPluginManifest(profile)
	if err != nil {
		return nil, err
	}
	if !plugin.UsesAgentPlugins(manifest) {
		return nil, nil
	}
	found, err := plugin.AgentPluginFindings(manifest, g.config.BaseDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return anchorAgentPluginFindings(found, filepath.Join(g.config.BaseDir, "plugin.json")), nil
}

// anchorAgentPluginFindings reports every finding against the package manifest,
// dropping a repeat: the build and the validation of its output can both name
// the same problem.
func anchorAgentPluginFindings(found []agentplugins.Finding, file string) []lint.AgentPluginFinding {
	seen := map[agentplugins.Finding]bool{}
	var out []lint.AgentPluginFinding
	for _, f := range found {
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, lint.AgentPluginFinding{File: file, Finding: f})
	}
	return out
}
