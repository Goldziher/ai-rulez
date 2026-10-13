package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"
)

// noPluginHint lists every way a marketplace gets plugins.
const noPluginHint = "add [[marketplace.plugins]] or [[plugins_authoring]] definitions, `members`, " +
	"[marketplace.from_domains], or name the definitions in a [[marketplaces_authoring]] entry's `plugins`"

// validatePluginSet checks the roots that define several plugins or marketplaces:
// [[plugins_authoring]] and [[marketplaces_authoring]], and the rules that tie
// them to [plugin], [marketplace] and each other.
func (c *Config) validatePluginSet() error {
	if err := c.checkPluginSpellings(); err != nil {
		return err
	}
	if err := c.validatePluginDefinitions(); err != nil {
		return err
	}
	return c.validateMarketplaceDefinitions()
}

// checkPluginSpellings rejects mixing the single-block spellings with the array
// ones: each pair would define the same thing twice.
func (c *Config) checkPluginSpellings() error {
	if c.Plugin != nil && len(c.PluginsAuthoring) > 0 {
		return oops.With("field", "plugins_authoring").
			Hint(fmt.Sprintf("Move [plugin] %q into a [[plugins_authoring]] entry that selects the root content "+
				"(skills = [\"*\"], commands = [\"*\"], agents = [\"*\"]), or keep [plugin] and drop [[plugins_authoring]]", c.Plugin.Name)).
			Errorf("[plugin] and [[plugins_authoring]] cannot be combined: [plugin] bundles the whole root, "+
				"[[plugins_authoring]] entries bundle what they select (migrate %q into an entry)", c.Plugin.Name)
	}
	if c.Marketplace != nil && len(c.MarketplacesAuthoring) > 0 {
		return oops.With("field", "marketplaces_authoring").
			Hint(fmt.Sprintf("Move [marketplace] %q into a [[marketplaces_authoring]] entry, or drop [[marketplaces_authoring]]", c.Marketplace.Name)).
			Errorf("[marketplace] and [[marketplaces_authoring]] cannot be combined (migrate %q into an entry)", c.Marketplace.Name)
	}
	return nil
}

// validatePluginDefinitions checks every [[plugins_authoring]] entry and that no
// plugin name is defined twice across it and [[marketplace.plugins]].
func (c *Config) validatePluginDefinitions() error {
	where := map[string]string{}
	if c.Marketplace != nil {
		for i := range c.Marketplace.Plugins {
			where[c.Marketplace.Plugins[i].Name] = "[[marketplace.plugins]]"
		}
	}
	servers := c.mcpServerNames()
	for i := range c.PluginsAuthoring {
		p := &c.PluginsAuthoring[i]
		if err := p.validate("plugins_authoring"); err != nil {
			return err
		}
		if first, dup := where[p.Name]; dup {
			spelling := "[[plugins_authoring]]"
			return oops.With("field", "plugins_authoring").With("value", p.Name).
				Hint("Keep the plugin in one place").
				Errorf("plugin %q is defined in %s and %s", p.Name, first, spelling)
		}
		where[p.Name] = "[[plugins_authoring]]"
	}
	for _, p := range c.PluginDefinitions() {
		for _, name := range p.MCPServers {
			if !slices.Contains(servers, name) {
				return oops.With("field", "mcp_servers").With("value", name).
					Hint("Define the server in [[mcp_servers]]; defined: "+strings.Join(servers, ", ")).
					Errorf("plugin %q bundles MCP server %q, which is not defined (defined: %s)", p.Name, name, strings.Join(servers, ", "))
			}
		}
		if err := c.validateHookGroups(p.Name, p.Hooks); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) mcpServerNames() []string {
	var names []string
	for i := range c.MCPServersRaw {
		names = append(names, c.MCPServersRaw[i].Name)
	}
	for name := range c.MCPServers {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// validateMarketplaceDefinitions checks the [[marketplaces_authoring]] entries:
// unique names and output directories, safe paths, and that each lists known
// plugins.
func (c *Config) validateMarketplaceDefinitions() error {
	defined := map[string]bool{}
	for _, p := range c.PluginDefinitions() {
		defined[p.Name] = true
	}
	names := make([]string, 0, len(defined))
	for name := range defined {
		names = append(names, name)
	}
	slices.Sort(names)

	byName := map[string]int{}
	byDir := map[string]string{}
	for i := range c.MarketplacesAuthoring {
		m := &c.MarketplacesAuthoring[i]
		if err := validateMarketplaceName(m.Name); err != nil {
			return err
		}
		if first, dup := byName[m.Name]; dup {
			return oops.With("field", "marketplaces_authoring").With("value", m.Name).
				Hint("Merge the entries, or rename one").
				Errorf("marketplace %q is defined twice ([[marketplaces_authoring]] entries #%d and #%d)", m.Name, first+1, i+1)
		}
		byName[m.Name] = i
		if err := validateOutputDir(m.Name, m.OutputDir); err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Clean(m.OutputDir))
		if other, dup := byDir[dir]; dup {
			return oops.With("field", "marketplaces_authoring.output_dir").With("value", m.OutputDir).
				Hint("Give each marketplace its own output_dir").
				Errorf("marketplaces %q and %q write to the same output_dir %q", other, m.Name, dir)
		}
		byDir[dir] = m.Name
		if len(m.Plugins) == 0 {
			return oops.With("field", "marketplaces_authoring.plugins").
				Hint(noPluginHint).
				Errorf("marketplace %q lists no plugin: %s", m.Name, noPluginHint)
		}
		seen := map[string]bool{}
		for _, name := range m.Plugins {
			if seen[name] {
				return oops.With("field", "marketplaces_authoring.plugins").With("value", name).
					Errorf("marketplace %q lists plugin %q twice", m.Name, name)
			}
			seen[name] = true
			if !defined[name] {
				return oops.With("field", "marketplaces_authoring.plugins").With("value", name).
					Hint("Define it with [[plugins_authoring]] or [[marketplace.plugins]]; defined: "+strings.Join(names, ", ")).
					Errorf("marketplace %q lists plugin %q, which no plugin definition provides (defined: %s)", m.Name, name, strings.Join(names, ", "))
			}
		}
	}
	return nil
}

func validateMarketplaceName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return oops.With("field", "marketplaces_authoring.name").With("value", name).
			Hint("Use a name without path separators").
			Errorf("marketplace has an invalid name %q", name)
	}
	return nil
}

func validateOutputDir(marketplace, dir string) error {
	if dir == "" {
		return nil
	}
	if isUnsafeProjectPath(dir) {
		return oops.With("field", "marketplaces_authoring.output_dir").With("value", dir).
			Hint("Use a project-relative directory that does not contain '..'").
			Errorf("marketplace %q has an unsafe output_dir", marketplace)
	}
	if clean := filepath.ToSlash(filepath.Clean(dir)); clean != "." {
		if err := ValidateOutputPath("marketplaces_authoring.output_dir", dir); err != nil {
			return oops.With("field", "marketplaces_authoring.output_dir").With("value", dir).
				Hint("Use a project-relative directory outside .git/ and .ai-rulez/").
				Wrapf(err, "marketplace %q has an unsafe output_dir", marketplace)
		}
	}
	return nil
}

// validateClassicMarketplaceHasPlugins rejects a [marketplace] block that no
// plugin source feeds: no [plugin], members, from_domains or definitions. Its
// index would list nothing.
func (c *Config) validateClassicMarketplaceHasPlugins() error {
	m := c.Marketplace
	if m == nil || c.Plugin != nil || len(m.Members) > 0 || m.HasDomainPlugins() || len(c.PluginsAuthoring) > 0 {
		return nil
	}
	return oops.With("field", "marketplace").
		Hint(noPluginHint).
		Errorf("marketplace %q lists no plugin: %s", m.Name, noPluginHint)
}
