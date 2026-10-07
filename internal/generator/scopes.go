package generator

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

// generateScopedOutputs renders the outputs of every [[scopes]] entry. rootContent
// is the content tree the root run rendered and run is its shared state.
func (g *Generator) generateScopedOutputs(activeProfile string, rootContent *config.ContentTree, run *config.RunState,
) ([]config.OutputFile, error) {
	var result []config.OutputFile
	slugs := map[string]string{}
	for _, scope := range g.config.Scopes {
		scopeRun, err := rulefiles.NewScopeRun(g.config.BaseDir, scope.Path)
		if err != nil {
			return nil, err
		}
		if prev, dup := slugs[strings.ToLower(scopeRun.Slug)]; dup {
			return nil, oops.With("scope", scope.Path, "other", prev).
				Hint("rename one scope directory so the two map to different rule file names").
				Errorf("scopes %q and %q share the rule file qualifier %q", prev, scope.Path, scopeRun.Slug)
		}
		slugs[strings.ToLower(scopeRun.Slug)] = scope.Path
		g.warnGeminiOnlyInScope(scope)

		scopeCfg, skip, err := g.scopeConfig(scope, scopeRun, activeProfile, rootContent, run)
		if err != nil {
			return nil, err
		}
		if skip {
			g.log().Debug("Scope has no content beyond the root run, skipping", "scope", scope.Path)
			continue
		}

		// Label the analysis so a cost report keeps scoped roots out of the
		// repository-root totals: a scoped instruction file is loaded only when
		// the agent is working inside that subtree.
		g.config.Analysis.EnterScope(scope.Path)
		outputsByPreset, err := config.GeneratePresets(scopeCfg)
		g.config.Analysis.EnterScope("")
		if err != nil {
			return nil, oops.With("scope", scope.Name).With("path", scope.Path).Wrapf(err, "generate scoped presets")
		}
		applySharedOutputs(outputsByPreset, scopeCfg, scopeCfg.Content)
		flat, err := flattenPresetOutputs(g.config.Diag, g.log(), g.config.ReadExisting, outputsByPreset)
		if err != nil {
			return nil, oops.With("scope", scope.Name).With("path", scope.Path).Wrapf(err, "merge scoped presets")
		}
		result = append(result, flat...)
	}
	return result, nil
}

// scopeConfig builds the config a scope renders with. A scoped file is loaded on
// top of the root file (Claude loads a subdirectory CLAUDE.md; Codex concatenates
// AGENTS.md from the root down), so a scope keeps only its profile's domain
// content, and of those only the domains the root run did not already render:
// rendering a domain twice would load its rules twice. skip is true when that
// leaves a scope with nothing to write.
func (g *Generator) scopeConfig(scope config.ScopeConfig, scopeRun *config.ScopeRun, activeProfile string,
	rootContent *config.ContentTree, run *config.RunState,
) (cfg *config.Config, skip bool, err error) {
	scopeProfile := scope.Profile
	if scopeProfile == "" {
		scopeProfile = activeProfile
	}
	scopeContent, err := g.getContentForProfile(scopeProfile)
	if err != nil {
		return nil, false, oops.With("scope", scope.Name).With("path", scope.Path).Wrapf(err, "resolve scope profile")
	}
	scopeContent.Rules = nil
	scopeContent.Context = nil
	scopeContent.Skills = nil
	scopeContent.Agents = nil
	scopeContent.Commands = nil
	scopeContent.Checks = nil

	domains := make(map[string]*config.Domain, len(scopeContent.Domains))
	dropped := 0
	for name, domain := range scopeContent.Domains {
		if _, inRoot := rootContent.Domains[name]; inRoot {
			dropped++
			continue
		}
		domains[name] = domain
	}
	scopeContent.Domains = domains

	mcpServers := g.collectMCPServersForContent(scopeContent, scopeProfile)
	if dropped > 0 && !hasDomainContent(scopeContent) && len(mcpServers) == 0 {
		return nil, true, nil
	}

	scopeCfg := *g.config
	scopeCfg.BaseDir = filepath.Join(g.config.BaseDir, filepath.FromSlash(scopeRun.Path))
	scopeCfg.Run = run.ForScope(scopeRun)
	scopeCfg.Content = scopeContent
	scopeCfg.MCPServers = mcpServers
	scopeCfg.Presets = scopedPresets(scope.Presets)
	scopeCfg.SourceHash = computeSourceHash(&scopeCfg, scopeContent)
	return &scopeCfg, false, nil
}

// warnGeminiOnlyInScope warns when agents_md is on and gemini is configured for a
// scope but not for the root. Gemini CLI reads the project .gemini/settings.json
// only, and the root run is what points it at AGENTS.md, so the scope's nested
// AGENTS.md is never loaded and GEMINI.md is not written.
func (g *Generator) warnGeminiOnlyInScope(scope config.ScopeConfig) {
	if !g.config.AgentsMD || !slices.Contains(scope.Presets, "gemini") {
		return
	}
	for i := range g.config.Presets {
		if g.config.Presets[i].GetName() == "gemini" {
			return
		}
	}
	g.config.Diag.Warn("agents_md is on and gemini is configured for a scope but not for the root, so Gemini CLI gets no "+
		"instructions for it (the root run points Gemini at AGENTS.md); add gemini to the root presets",
		"scope", scope.Path)
}

func hasDomainContent(tree *config.ContentTree) bool {
	for _, d := range tree.Domains {
		if len(d.Rules)+len(d.Context)+len(d.Skills)+len(d.Agents)+len(d.Commands)+len(d.Checks) > 0 {
			return true
		}
	}
	return false
}

// scopedPresets lists the presets of a scope: the configured names without
// duplicates, or claude and codex by default.
func scopedPresets(names []string) []config.Preset {
	if len(names) == 0 {
		names = []string{"claude", "codex"}
	}
	seen := make(map[string]bool, len(names))
	result := make([]config.Preset, 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, config.Preset{BuiltIn: name})
	}
	return result
}
