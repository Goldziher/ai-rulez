package config

import (
	"regexp"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
	"github.com/samber/oops"
)

// namePrefix is what from_domains may prepend to a domain name: the plugin
// name grammar without the leading-character rule, since the domain follows.
var namePrefix = regexp.MustCompile(`^[a-z0-9._-]*$`)

const (
	fieldPath    = "path"
	fieldRepo    = "repo"
	fieldURL     = "url"
	fieldSkills  = "skills"
	fieldDomains = "domains"
)

// marketplaceSourceKinds maps each supported source kind to its required field.
var marketplaceSourceKinds = map[string]string{
	"directory": fieldPath,
	"github":    fieldRepo,
	"git":       fieldURL,
	"url":       fieldURL,
}

// validateDomainPlugins checks the domain-plugin additions of [marketplace].
func (m *MarketplaceAuthoring) validateDomainPlugins() error {
	if m.OutputDir != "" && isUnsafeProjectPath(m.OutputDir) {
		return oops.With("field", "marketplace.output_dir").With("value", m.OutputDir).
			Hint("Use a project-relative directory that does not contain '..'").
			Errorf("marketplace %q has an unsafe output_dir", m.Name)
	}
	if len(m.Members) > 0 && m.HasDomainPlugins() && m.OutputDir != "" && m.OutputDir != "." {
		return oops.With("field", "marketplace.output_dir").
			Hint("Members are addressed relative to the project root; drop output_dir or the members").
			Errorf("marketplace %q combines members with an output_dir", m.Name)
	}
	if err := m.FromDomains.validate(m.Name); err != nil {
		return err
	}
	seen := make(map[string]bool, len(m.Plugins))
	for i := range m.Plugins {
		if err := m.Plugins[i].validate(); err != nil {
			return err
		}
		if seen[m.Plugins[i].Name] {
			return oops.With("field", "marketplace.plugins").With("value", m.Plugins[i].Name).
				Errorf("marketplace %q lists duplicate plugin %q", m.Name, m.Plugins[i].Name)
		}
		seen[m.Plugins[i].Name] = true
	}
	return m.validateCatalogSkill()
}

// validate checks the [marketplace.from_domains] block; a nil block is valid.
func (fd *DomainPluginsConfig) validate(marketplace string) error {
	if fd == nil {
		return nil
	}
	if !namePrefix.MatchString(fd.NamePrefix) || strings.Contains(fd.NamePrefix, "..") {
		return oops.With("field", "marketplace.from_domains.name_prefix").With("value", fd.NamePrefix).
			Hint("Use lowercase letters, digits, '.', '_' and '-'").
			Errorf("marketplace %q has an invalid name_prefix", marketplace)
	}
	if err := validatePatterns("marketplace.from_domains.include", fd.Include); err != nil {
		return err
	}
	if err := validatePatterns("marketplace.from_domains.exclude", fd.Exclude); err != nil {
		return err
	}
	return fd.validateDefaults(marketplace)
}

func (fd *DomainPluginsConfig) validateDefaults(marketplace string) error {
	return fd.PluginDefaults.validate("marketplace.from_domains", marketplace)
}

func (m *MarketplaceAuthoring) validateCatalogSkill() error {
	c := m.CatalogSkill
	if c == nil || !c.Enabled {
		return nil
	}
	if !pluginName.MatchString(c.SkillName()) || strings.Contains(c.SkillName(), "..") {
		return oops.With("field", "marketplace.catalog_skill.name").With("value", c.SkillName()).
			Errorf("%q is not a valid catalog skill name", c.SkillName())
	}
	if !m.HasDomainPlugins() {
		return oops.With("field", "marketplace.catalog_skill").
			Hint("Enable [marketplace.from_domains] or add [[marketplace.plugins]]").
			Errorf("marketplace %q enables a catalog skill but generates no domain plugins", m.Name)
	}
	return nil
}

func (d *PluginDefaults) validate(field, marketplace string) error {
	seen := make(map[string]bool, len(d.Runtimes))
	for _, r := range d.Runtimes {
		if !isValidPluginRuntime(r) || seen[r] {
			return oops.With("field", field+".runtimes").With("value", r).
				Hint("Valid runtimes: claude, cursor, codex, gemini, kimi, opencode, factory, hermes, agent-plugins; no duplicates").
				Errorf("marketplace %q has an invalid or duplicate runtime %q", marketplace, r)
		}
		seen[r] = true
	}
	return nil
}

func (p *MarketplacePlugin) validate() error {
	if err := validatePluginName(p.Name); err != nil {
		return oops.With("field", "marketplace.plugins.name").Wrapf(err, "marketplace plugin")
	}
	if len(p.Domains) == 0 && len(p.Skills) == 0 && len(p.Commands) == 0 && len(p.Agents) == 0 {
		return oops.With("field", "marketplace.plugins").
			Hint("Select content with domains, skills, commands or agents").
			Errorf("marketplace plugin %q selects no content", p.Name)
	}
	for field, patterns := range map[string][]string{
		fieldDomains: p.Domains, fieldSkills: p.Skills, "commands": p.Commands, "agents": p.Agents,
	} {
		if err := validatePatterns("marketplace.plugins."+field, patterns); err != nil {
			return err
		}
	}
	if err := p.Relevance.validate(p.Name); err != nil {
		return err
	}
	return p.PluginDefaults.validate("marketplace.plugins", p.Name)
}

// Limits Claude Code enforces on relevance values; an entry over a limit is
// rejected by the marketplace.
const (
	relevanceMaxTopic   = 64
	relevanceMaxGlobs   = 10
	relevanceMaxGlobLen = 256
	relevanceMaxCLI     = 10
	relevanceMaxCLILen  = 64
	relevanceMaxHosts   = 20
	relevanceMaxHostLen = 128
	relevanceMaxDeps    = 10
)

func (r *PluginRelevance) validate(plugin string) error {
	if r == nil {
		return nil
	}
	fail := func(format string, args ...any) error {
		return oops.With("field", "marketplace.plugins.relevance").
			Errorf("marketplace plugin %q relevance: "+format, append([]any{plugin}, args...)...)
	}
	sig := r.Signals
	if sig.IsEmpty() {
		return fail("set at least one signal (cwd, cli, hosts, files_read, manifest_deps)")
	}
	if len(r.Topic) > relevanceMaxTopic {
		return fail("topic is longer than %d characters", relevanceMaxTopic)
	}
	for name, check := range map[string]struct {
		values       []string
		maxN, maxLen int
	}{
		"cwd":        {sig.CWD, relevanceMaxGlobs, relevanceMaxGlobLen},
		"cli":        {sig.CLI, relevanceMaxCLI, relevanceMaxCLILen},
		"hosts":      {sig.Hosts, relevanceMaxHosts, relevanceMaxHostLen},
		"files_read": {sig.FilesRead, relevanceMaxGlobs, relevanceMaxGlobLen},
	} {
		if len(check.values) > check.maxN {
			return fail("%s has more than %d entries", name, check.maxN)
		}
		for _, v := range check.values {
			if v == "" || len(v) > check.maxLen {
				return fail("%s has an empty or over-long entry", name)
			}
		}
	}
	for _, host := range sig.Hosts {
		if strings.ContainsAny(host, "/:") {
			return fail("hosts entry %q must be a bare hostname (no scheme, port or path)", host)
		}
	}
	if len(sig.ManifestDeps) > relevanceMaxDeps {
		return fail("manifest_deps has more than %d entries", relevanceMaxDeps)
	}
	for _, dep := range sig.ManifestDeps {
		if dep.File == "" || dep.Pattern == "" {
			return fail("manifest_deps entries need 'file' and 'pattern'")
		}
	}
	return nil
}

// validatePatterns rejects empty and malformed name/glob patterns.
func validatePatterns(field string, patterns []string) error {
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			return oops.With("field", field).Errorf("%s has an empty pattern", field)
		}
		if targetmatch.InvalidGlob(pattern) {
			return oops.With("field", field).With("value", pattern).Errorf("%s has a malformed glob %q", field, pattern)
		}
	}
	return nil
}

// validatePlacement checks the [placement] block.
func (c *Config) validatePlacement() error {
	p := c.Placement
	if p == nil {
		return nil
	}
	if p.Default != "" && p.Default != PlacementCore && p.Default != PlacementPlugin {
		return oops.With("field", "placement.default").With("value", p.Default).
			Hint("Use \"core\" or \"plugin\"").Errorf("placement.default %q is not core or plugin", p.Default)
	}
	if err := validatePatterns("placement.core", p.Core); err != nil {
		return err
	}
	if err := validatePatterns("placement.plugin", p.Plugin); err != nil {
		return err
	}
	for _, core := range p.Core {
		for _, plug := range p.Plugin {
			if targetmatch.Normalize(core) == targetmatch.Normalize(plug) {
				return oops.With("field", "placement").With("value", core).
					Errorf("placement lists %q as both core and plugin", core)
			}
		}
	}
	return nil
}

// validateClaudeSettings checks the [claude.settings] block.
func (c *Config) validateClaudeSettings() error {
	if c.Claude == nil || c.Claude.Settings == nil {
		return nil
	}
	s := c.Claude.Settings
	if !s.Manage {
		return nil
	}
	market := c.Marketplace
	needsMarket := s.RegistersMarketplace() || len(s.EnablePlugins) > 0 || len(s.DisablePlugins) > 0
	if needsMarket && (market == nil || market.Name == "") {
		return oops.With("field", "claude.settings").
			Hint("Add a [marketplace] block with a name, or set register_marketplace = false and list no plugins").
			Errorf("claude.settings needs a [marketplace] name to build plugin ids")
	}
	if err := validatePluginNames("claude.settings.enable_plugins", s.EnablePlugins); err != nil {
		return err
	}
	if err := validatePluginNames("claude.settings.disable_plugins", s.DisablePlugins); err != nil {
		return err
	}
	for _, name := range s.DisablePlugins {
		if slices.Contains(s.EnablePlugins, name) {
			return oops.With("field", "claude.settings").With("value", name).
				Errorf("plugin %q is both enabled and disabled in claude.settings", name)
		}
	}
	return validateMarketplaceSource(s.MarketplaceSource)
}

// validateMarketplaceSource checks an explicit marketplace source object.
func validateMarketplaceSource(src *MarketplaceSource) error {
	if src == nil {
		return nil
	}
	required, ok := marketplaceSourceKinds[src.Source]
	if !ok {
		return oops.With("field", "claude.settings.marketplace_source.source").With("value", src.Source).
			Hint("Use directory, github, git or url").Errorf("unsupported marketplace source %q", src.Source)
	}
	values := map[string]string{fieldPath: src.Path, fieldRepo: src.Repo, fieldURL: src.URL}
	if values[required] == "" {
		return oops.With("field", "claude.settings.marketplace_source."+required).
			Errorf("marketplace source %q requires %q", src.Source, required)
	}
	return nil
}

// validateOutputOptions checks [placement], [claude.settings], [[hooks]] and [permissions].
func (c *Config) validateOutputOptions() error {
	if err := c.validatePlacement(); err != nil {
		return err
	}
	if err := c.validateClaudeSettings(); err != nil {
		return err
	}
	return c.validateSettingsBlocks()
}

func validatePluginNames(field string, names []string) error {
	for _, name := range names {
		if !pluginName.MatchString(name) || strings.Contains(name, "..") {
			return oops.With("field", field).With("value", name).Errorf("%q is not a valid plugin name", name)
		}
	}
	return nil
}
