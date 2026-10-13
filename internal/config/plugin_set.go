package config

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
)

// PluginDefinitions returns every plugin definition of the root, in declaration
// order: the [[marketplace.plugins]] entries, then the [[plugins_authoring]]
// entries (including the ones read from .ai-rulez/plugins/<name>.toml). It is the
// one place that knows both spellings.
func (c *Config) PluginDefinitions() []*MarketplacePlugin {
	if c == nil {
		return nil
	}
	var defs []*MarketplacePlugin
	if c.Marketplace != nil {
		for i := range c.Marketplace.Plugins {
			defs = append(defs, &c.Marketplace.Plugins[i])
		}
	}
	for i := range c.PluginsAuthoring {
		defs = append(defs, &c.PluginsAuthoring[i])
	}
	return defs
}

// HasPluginSet reports whether the root generates a set of plugins rather than
// one [plugin] bundle: monorepo members, domain plugins, plugin definitions or
// marketplace definitions.
func (c *Config) HasPluginSet() bool {
	if c == nil {
		return false
	}
	if m := c.Marketplace; m != nil && (len(m.Members) > 0 || m.HasDomainPlugins()) {
		return true
	}
	return len(c.PluginsAuthoring) > 0 || len(c.MarketplacesAuthoring) > 0
}

// MarketplaceTarget is one marketplace index a root writes.
type MarketplaceTarget struct {
	Name        string
	Description string
	Owner       *Author
	// Plugins names the plugin definitions the index lists; nil lists every plugin
	// of the root.
	Plugins     []string
	OutputDir   string
	CursorIndex bool
	// Classic is the single [marketplace] block when it is the source of the
	// target, which also carries members, from_domains and the catalog skill.
	Classic *MarketplaceAuthoring
}

// MarketplaceTargets returns the indexes the root writes: the single [marketplace]
// block, else each [[marketplaces_authoring]] entry, else (for a root with only
// [[plugins_authoring]]) one index named after the project that lists them all.
func (c *Config) MarketplaceTargets() []MarketplaceTarget {
	if c == nil {
		return nil
	}
	if m := c.Marketplace; m != nil {
		return []MarketplaceTarget{{
			Name: m.Name, Description: m.Description, Owner: m.Owner,
			OutputDir: m.OutputDir, CursorIndex: m.CursorIndex, Classic: m,
		}}
	}
	if len(c.MarketplacesAuthoring) > 0 {
		targets := make([]MarketplaceTarget, 0, len(c.MarketplacesAuthoring))
		for i := range c.MarketplacesAuthoring {
			d := &c.MarketplacesAuthoring[i]
			targets = append(targets, MarketplaceTarget{
				Name: d.Name, Description: d.Description, Owner: d.Owner,
				Plugins: append([]string{}, d.Plugins...), OutputDir: d.OutputDir, CursorIndex: d.CursorIndex,
			})
		}
		return targets
	}
	if len(c.PluginsAuthoring) > 0 {
		name := c.Name
		if name == "" {
			name = "plugins"
		}
		return []MarketplaceTarget{{Name: name, Description: c.Description}}
	}
	return nil
}

// PluginSelectorProblem is a selector of a plugin definition that matches no
// content of the project.
type PluginSelectorProblem struct {
	Plugin  string
	Field   string
	Pattern string
	Message string
}

// PluginSelectorProblems lists the skills, commands, agents and domains selectors
// of the plugin definitions that match nothing in the loaded content, sorted by
// plugin and field. Builtin content is never bundled, so it does not count.
func (c *Config) PluginSelectorProblems() []PluginSelectorProblem {
	if c == nil || c.Content == nil {
		return nil
	}
	tree := c.Content
	var problems []PluginSelectorProblem
	for _, def := range c.PluginDefinitions() {
		fields := []struct {
			name     string
			patterns []string
			names    []string
		}{
			{fieldSkills, def.Skills, bundleableNames(tree.Skills)},
			{commandsDir, def.Commands, bundleableNames(tree.Commands)},
			{agentsDir, def.Agents, bundleableNames(tree.Agents)},
			{fieldDomains, def.Domains, userDomainNames(tree)},
		}
		for _, f := range fields {
			for _, pattern := range f.patterns {
				if slices.ContainsFunc(f.names, func(name string) bool { return targetmatch.Match([]string{pattern}, nil, name) }) {
					continue
				}
				problems = append(problems, PluginSelectorProblem{
					Plugin: def.Name, Field: f.name, Pattern: pattern,
					Message: fmt.Sprintf("plugin %q: %s selector %q matches nothing (available: %s)",
						def.Name, f.name, pattern, strings.Join(f.names, ", ")),
				})
			}
		}
	}
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Plugin != problems[j].Plugin {
			return problems[i].Plugin < problems[j].Plugin
		}
		return problems[i].Field < problems[j].Field
	})
	return problems
}

func bundleableNames(files []ContentFile) []string {
	var names []string
	for i := range files {
		if !strings.HasPrefix(files[i].Path, "builtin://") {
			names = append(names, files[i].Name)
		}
	}
	return names
}

func userDomainNames(tree *ContentTree) []string {
	var names []string
	for name, d := range tree.Domains {
		if d != nil && !d.Builtin {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
