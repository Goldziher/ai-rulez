package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/oops"
)

// Plugin layouts an installed_plugins entry can force with `format`.
const (
	PluginFormatClaude       = "claude"
	PluginFormatCodex        = "codex"
	PluginFormatGemini       = "gemini"
	PluginFormatCursor       = "cursor"
	PluginFormatAgentPlugins = "agent-plugins"
)

// PluginFormats lists the values of `format`.
func PluginFormats() []string {
	return []string{PluginFormatClaude, PluginFormatCodex, PluginFormatGemini, PluginFormatCursor, PluginFormatAgentPlugins}
}

var pluginNamePrefix = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateInstalledMarketplaces validates the installed_marketplaces section:
// required fields, unique names, and a source, ref and path that are safe to
// hand to git or an HTTP client.
func ValidateInstalledMarketplaces(list []InstalledMarketplaceConfig) error {
	seen := map[string]bool{}
	for i := range list {
		m := &list[i]
		bad := func(field, reason string) error {
			return oops.With("field", fmt.Sprintf("installed_marketplaces[%d].%s", i, field)).With("marketplace_name", m.Name).
				Errorf("installed marketplace %q: %s %s", m.Name, field, reason)
		}
		switch {
		case m.Name == "":
			return bad("name", "is required")
		case m.Source == "":
			return bad("source", "is required: a git URL, owner/repo, a local path or an https URL of a marketplace.json")
		case seen[m.Name]:
			return bad("name", "is a duplicate")
		}
		seen[m.Name] = true
		if reason := pluginSourceProblem(m.Source); reason != "" {
			return bad("source", reason)
		}
		if reason := skillRefProblem(m.Ref); reason != "" && !IsVersionConstraintRef(m.Ref) {
			return bad("ref", reason)
		}
		if reason := skillPathProblem(m.Path); reason != "" {
			return bad("path", reason)
		}
		if err := checkTrust(m.Trust, bad); err != nil {
			return err
		}
		if kind := ClassifyPluginSource(m.Source); (m.Ref != "" || m.Version != "") && (kind == PluginSourceLocal || kind == PluginSourceIndexURL) {
			return bad("ref", "and version need a git source; a local directory or a marketplace.json URL has no revisions")
		}
	}
	return nil
}

func checkTrust(trust string, bad func(field, reason string) error) error {
	switch trust {
	case "", TrustError, TrustWarn:
		return nil
	}
	return bad("trust", fmt.Sprintf("has an invalid trust level %q (use 'error', the default, or 'warn')", trust))
}

// ValidateInstalledPlugins validates the installed_plugins section against the
// declared marketplaces.
func ValidateInstalledPlugins(plugins []InstalledPluginConfig, marketplaces []InstalledMarketplaceConfig) error {
	known := map[string]bool{}
	for i := range marketplaces {
		known[marketplaces[i].Name] = true
	}
	seen := map[string]bool{}
	for i := range plugins {
		p := &plugins[i]
		if p.Name == "" {
			return oops.With("field", fmt.Sprintf("installed_plugins[%d].name", i)).
				Errorf("installed plugin at index %d missing required field 'name'", i)
		}
		if seen[p.Name] {
			return oops.With("field", "installed_plugins").With("plugin_name", p.Name).
				Errorf("duplicate installed plugin name: %q", p.Name)
		}
		seen[p.Name] = true
		if err := validateInstalledPlugin(i, p, known); err != nil {
			return err
		}
	}
	return nil
}

func validateInstalledPlugin(i int, p *InstalledPluginConfig, known map[string]bool) error {
	bad := func(field, reason string) error {
		return oops.With("field", fmt.Sprintf("installed_plugins[%d].%s", i, field)).With("plugin_name", p.Name).
			Errorf("installed plugin %q: %s %s", p.Name, field, reason)
	}
	if err := validatePluginOrigin(p, known, bad); err != nil {
		return err
	}
	if err := checkTrust(p.Trust, bad); err != nil {
		return err
	}
	if p.Format != "" && !slices.Contains(PluginFormats(), p.Format) {
		return bad("format", fmt.Sprintf("must be one of %s", strings.Join(PluginFormats(), ", ")))
	}
	for _, list := range []struct {
		field string
		kinds []string
	}{{"include", p.Include}, {"exclude", p.Exclude}} {
		for _, k := range list.kinds {
			if !slices.Contains(PluginKinds(), k) {
				return bad(list.field, fmt.Sprintf("has the unknown component kind %q (use %s)", k, strings.Join(PluginKinds(), ", ")))
			}
		}
	}
	if p.NamePrefix != "" && !pluginNamePrefix.MatchString(p.NamePrefix) {
		return bad("name_prefix", "must start with a letter or digit and hold only letters, digits, '.', '_' and '-'")
	}
	return nil
}

// validatePluginOrigin checks where the plugin comes from: exactly one of a
// marketplace or a direct source, each with the fields that belong to it.
func validatePluginOrigin(p *InstalledPluginConfig, known map[string]bool, bad func(field, reason string) error) error {
	switch {
	case p.Marketplace != "" && p.Source != "":
		return bad("source", "cannot be combined with marketplace: set one of them")
	case p.Marketplace == "" && p.Source == "":
		return bad("source", "is required when no marketplace is set: a git URL, owner/repo or a local path")
	case p.Marketplace != "":
		return validateMarketplacePlugin(p, known, bad)
	}
	return validateSourcePlugin(p, bad)
}

// validateMarketplacePlugin checks a plugin installed from a marketplace, which
// follows the marketplace's own ref and version.
func validateMarketplacePlugin(p *InstalledPluginConfig, known map[string]bool, bad func(field, reason string) error) error {
	if !known[p.Marketplace] {
		return bad("marketplace", fmt.Sprintf("names %q, which is not an installed_marketplaces entry", p.Marketplace))
	}
	for _, kv := range [][2]string{{"path", p.Path}, {"ref", p.Ref}, {"version", p.Version}, {"tag_prefix", p.TagPrefix}, {"min_release_age", p.MinReleaseAge}} {
		if kv[1] != "" {
			return bad(kv[0], "applies to a plugin installed from source; a marketplace plugin follows the marketplace's own ref and version")
		}
	}
	if p.IncludePrerelease {
		return bad("include_prerelease", "applies to a plugin installed from source")
	}
	return nil
}

// validateSourcePlugin checks a plugin installed from a repository or directory.
func validateSourcePlugin(p *InstalledPluginConfig, bad func(field, reason string) error) error {
	if p.Plugin != "" {
		return bad("plugin", "names a marketplace entry and needs marketplace")
	}
	if kind := ClassifyPluginSource(p.Source); kind == PluginSourceIndexURL {
		return bad("source", "must be a plugin repository or directory, not a marketplace.json URL")
	}
	if reason := pluginSourceProblem(p.Source); reason != "" {
		return bad("source", reason)
	}
	if reason := skillRefProblem(p.Ref); reason != "" && !IsVersionConstraintRef(p.Ref) {
		return bad("ref", reason)
	}
	if reason := skillPathProblem(p.Path); reason != "" {
		return bad("path", reason)
	}
	if ClassifyPluginSource(p.Source) == PluginSourceLocal && (p.Ref != "" || p.Version != "") {
		return bad("ref", "and version need a git source")
	}
	return nil
}

// validatePluginVersionKeys checks the version keys of every marketplace and plugin.
func (c *Config) validatePluginVersionKeys() error {
	for i := range c.InstalledMarketplaces {
		m := &c.InstalledMarketplaces[i]
		if err := validateVersionKeys("installed_marketplaces", m.Name, m.Ref, m.Version, m.TagPrefix, m.IncludePrerelease, m.MinReleaseAge); err != nil {
			return err
		}
	}
	for i := range c.InstalledPlugins {
		p := &c.InstalledPlugins[i]
		if err := validateVersionKeys("installed_plugins", p.Name, p.Ref, p.Version, p.TagPrefix, p.IncludePrerelease, p.MinReleaseAge); err != nil {
			return err
		}
	}
	return nil
}

// validateInstalledPlugins validates the marketplace and plugin sections.
func (c *Config) validateInstalledPlugins() error {
	if err := c.validatePluginVersionKeys(); err != nil {
		return err
	}
	if err := ValidateInstalledMarketplaces(c.InstalledMarketplaces); err != nil {
		return err
	}
	return ValidateInstalledPlugins(c.InstalledPlugins, c.InstalledMarketplaces)
}
