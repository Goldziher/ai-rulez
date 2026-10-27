package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/oops"
)

// Claude Code marketplace source types, as [[enabled_plugins]] marketplace.type
// and [marketplace] sources name them.
const (
	MarketplaceTypeGitHub    = "github"
	MarketplaceTypeGit       = "git"
	MarketplaceTypeURL       = "url"
	MarketplaceTypeDirectory = "directory"
)

// EnabledPlugin is one [[enabled_plugins]] entry: a plugin from a marketplace
// the project does not author, switched on for every harness that can express
// it. Claude Code gets enabledPlugins (and the marketplace registration), Codex
// gets a [plugins."<id>"] table, and OpenCode gets its npm package in `plugin`.
type EnabledPlugin struct {
	// ID is the fully qualified plugin id, <name>@<marketplace>.
	ID string `yaml:"id" json:"id" toml:"id"`
	// Marketplace registers the marketplace the plugin comes from for Claude
	// Code. Leave it out when the harness already knows the marketplace.
	Marketplace *EnabledPluginMarketplace `yaml:"marketplace,omitempty" json:"marketplace,omitempty" toml:"marketplace,omitempty"`
	// OpenCode is the npm package of the plugin for OpenCode, which has no
	// marketplaces. Without it OpenCode is left alone.
	OpenCode string `yaml:"opencode,omitempty" json:"opencode,omitempty" toml:"opencode,omitempty"`
}

// EnabledPluginMarketplace is the marketplace of an enabled plugin.
type EnabledPluginMarketplace struct {
	// Name is the marketplace name; it must match the id's marketplace part.
	Name string `yaml:"name" json:"name" toml:"name"`
	// Source is where the marketplace lives: owner/repo for github, a repository
	// or manifest URL for git and url, a project-relative directory for directory.
	Source string `yaml:"source" json:"source" toml:"source"`
	// Type is the kind of Source: github, git, url or directory.
	Type string `yaml:"type" json:"type" toml:"type"`
	// Ref pins a git ref (branch or tag) for github and git sources.
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
}

var (
	// githubRepoSource is owner/repo.
	githubRepoSource = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)
	// scpGitSource is the scp-like ssh form git@host:path.
	scpGitSource = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[A-Za-z0-9._/~-]+$`)
	// gitRefName is a branch or tag name without whitespace or shell metacharacters.
	gitRefName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	// npmPackageSpec is an npm package name with an optional version or tag.
	npmPackageSpec = regexp.MustCompile(`^(@[a-z0-9~][a-z0-9._~-]*/)?[a-z0-9~][a-z0-9._~-]*(@[A-Za-z0-9._^~<>=*+-]+)?$`)
)

// pluginIDParts splits name@marketplace; ok is false for any other shape.
func pluginIDParts(id string) (name, market string, ok bool) {
	name, market, found := strings.Cut(id, "@")
	if !found || name == "" || market == "" || strings.Contains(market, "@") {
		return "", "", false
	}
	return name, market, true
}

// HasEnabledPlugins reports whether [[enabled_plugins]] is declared.
func (c *Config) HasEnabledPlugins() bool {
	return c != nil && len(c.EnabledPlugins) > 0
}

// RendersEnabledPlugins reports whether [[enabled_plugins]] is written in this
// run. A [[scopes]] subdirectory renders none: enabling a plugin is project-wide.
func (c *Config) RendersEnabledPlugins() bool {
	return c.HasEnabledPlugins() && (c.Run == nil || c.Run.Scope == nil)
}

// ClaudeSource is the marketplace as a Claude Code source object.
func (m *EnabledPluginMarketplace) ClaudeSource() *MarketplaceSource {
	src := &MarketplaceSource{Source: m.Type}
	switch m.Type {
	case MarketplaceTypeGitHub:
		src.Repo = m.Source
		src.Ref = m.Ref
	case MarketplaceTypeGit:
		src.URL = m.Source
		src.Ref = m.Ref
	case MarketplaceTypeURL:
		src.URL = m.Source
	default:
		src.Path = m.Source
	}
	return src
}

// QualifiedPluginID returns a fully qualified id as it is, and qualifies a bare
// plugin name with the [marketplace] name of this project.
func (c *Config) QualifiedPluginID(name string) string {
	if strings.Contains(name, "@") || c == nil || c.Marketplace == nil || c.Marketplace.Name == "" {
		return name
	}
	return name + "@" + c.Marketplace.Name
}

// validateEnabledPlugins checks [[enabled_plugins]] against allowlisted formats
// and warns when no active preset can express an entry.
func (c *Config) validateEnabledPlugins() error {
	seen := map[string]bool{}
	markets := map[string]*EnabledPluginMarketplace{}
	for i := range c.EnabledPlugins {
		p := &c.EnabledPlugins[i]
		field := fmt.Sprintf("enabled_plugins[%d]", i)
		if err := validateEnabledPluginID(field, p.ID); err != nil {
			return err
		}
		if seen[p.ID] {
			return oops.With("field", field+".id").With("value", p.ID).Errorf("plugin %q is listed twice in enabled_plugins", p.ID)
		}
		seen[p.ID] = true
		if err := validateEnabledPluginMarketplace(field, p, markets); err != nil {
			return err
		}
		if err := c.validateEnabledPluginScope(field, p); err != nil {
			return err
		}
		if p.OpenCode != "" && !npmPackageSpec.MatchString(p.OpenCode) {
			return oops.With("field", field+".opencode").With("value", p.OpenCode).
				Hint("Use an npm package name, optionally with @version, such as basemind-opencode or @scope/pkg@1.2.3").
				Errorf("%q is not an npm package", p.OpenCode)
		}
	}
	if err := c.validateEnabledPluginsVsClaudeSettings(); err != nil {
		return err
	}
	c.warnEnabledPluginsUnrendered()
	return nil
}

// validateEnabledPluginScope rejects what cannot be written where the entry
// renders: the user settings file is read from any working directory, so a
// project-relative directory marketplace would resolve against the wrong place,
// and a marketplace named like the project's registered own one would replace it.
func (c *Config) validateEnabledPluginScope(field string, p *EnabledPlugin) error {
	m := p.Marketplace
	if m == nil {
		return nil
	}
	if c.UserScope && m.Type == MarketplaceTypeDirectory {
		return oops.With("field", field+".marketplace.type").With("value", m.Source).
			Hint("Use a github, git or url marketplace in the user config, or declare the plugin in the project config").
			Errorf("%s cannot use a directory marketplace in the user config: %q is relative to a project", field, m.Source)
	}
	if c.RegistersLocalMarketplace() && c.Marketplace != nil && m.Name == c.Marketplace.Name {
		return oops.With("field", field+".marketplace.name").With("value", m.Name).
			Hint("Leave out marketplace in this entry: the marketplace of this project is already registered").
			Errorf("marketplace %q is the marketplace of this project and would replace its registration", m.Name)
	}
	return nil
}

// validateEnabledPluginsVsClaudeSettings compares [[enabled_plugins]] with the
// [claude.settings] plugin lists, after qualifying bare names with the local
// marketplace. Enabling and disabling one id is a conflict; listing it in both
// enable lists is redundant and only warned about.
func (c *Config) validateEnabledPluginsVsClaudeSettings() error {
	if !c.ManagesClaudeSettings() {
		return nil
	}
	s := c.Claude.Settings
	disabled := map[string]bool{}
	for _, name := range s.DisablePlugins {
		disabled[c.QualifiedPluginID(name)] = true
	}
	enabled := map[string]bool{}
	for _, name := range s.EnablePlugins {
		enabled[c.QualifiedPluginID(name)] = true
	}
	for i := range c.EnabledPlugins {
		id := c.EnabledPlugins[i].ID
		if disabled[id] {
			return oops.With("field", fmt.Sprintf("enabled_plugins[%d].id", i)).With("value", id).
				Hint("Remove the id from one of the two lists").
				Errorf("plugin %q is enabled in enabled_plugins and disabled in claude.settings.disable_plugins", id)
		}
		if enabled[id] {
			c.Warn(fmt.Sprintf("plugin %q is listed in enabled_plugins and in claude.settings.enable_plugins; list it once", id),
				"field", "enabled_plugins")
		}
	}
	return nil
}

func validateEnabledPluginID(field, id string) error {
	if id == "" {
		return oops.With("field", field+".id").Hint(`Set id = "<name>@<marketplace>"`).Errorf("%s.id is required", field)
	}
	name, market, ok := pluginIDParts(id)
	if !ok {
		return oops.With("field", field+".id").With("value", id).
			Hint(`Use the fully qualified id, for example "basemind@basemind"`).
			Errorf("%q is not name@marketplace", id)
	}
	for _, part := range []string{name, market} {
		if !pluginName.MatchString(part) || strings.Contains(part, "..") {
			return oops.With("field", field+".id").With("value", id).
				Hint("Use lowercase letters, digits, '.', '_' and '-' on both sides of the '@'").
				Errorf("%q is not a valid plugin id", id)
		}
	}
	return nil
}

func validateEnabledPluginMarketplace(field string, p *EnabledPlugin, markets map[string]*EnabledPluginMarketplace) error {
	m := p.Marketplace
	if m == nil {
		return nil
	}
	_, idMarket, _ := pluginIDParts(p.ID)
	where := field + ".marketplace"
	if m.Name != idMarket {
		return oops.With("field", where+".name").With("value", m.Name).
			Errorf("marketplace name %q does not match the marketplace of %q", m.Name, p.ID)
	}
	if _, ok := marketplaceSourceKinds[m.Type]; !ok {
		return oops.With("field", where+".type").With("value", m.Type).
			Hint("Use github, git, url or directory").Errorf("unsupported marketplace type %q", m.Type)
	}
	if err := validateEnabledMarketplaceSource(where, m); err != nil {
		return err
	}
	if m.Ref != "" && (m.Type == MarketplaceTypeURL || m.Type == MarketplaceTypeDirectory || !gitRefName.MatchString(m.Ref) || strings.Contains(m.Ref, "..")) {
		return oops.With("field", where+".ref").With("value", m.Ref).
			Hint("ref is a branch or tag of a github or git source").Errorf("marketplace %q has an invalid ref %q", m.Name, m.Ref)
	}
	if prior, dup := markets[m.Name]; dup && *prior != *m {
		return oops.With("field", where).With("value", m.Name).
			Errorf("marketplace %q is declared twice with different sources", m.Name)
	}
	markets[m.Name] = m
	return nil
}

func validateEnabledMarketplaceSource(where string, m *EnabledPluginMarketplace) error {
	bad := func(hint string) error {
		return oops.With("field", where+".source").With("value", m.Source).Hint(hint).
			Errorf("marketplace %q has an invalid %s source: %s", m.Name, m.Type, hint)
	}
	if m.Source == "" {
		return oops.With("field", where+".source").Errorf("marketplace %q needs a source", m.Name)
	}
	switch m.Type {
	case MarketplaceTypeGitHub:
		if !githubRepoSource.MatchString(m.Source) {
			return bad("Use owner/repo")
		}
	case MarketplaceTypeGit:
		if !strings.HasPrefix(m.Source, "https://") && !strings.HasPrefix(m.Source, "ssh://") && !scpGitSource.MatchString(m.Source) {
			return bad("Use an https:// or ssh:// URL, or git@host:path; http:// and git:// are rejected")
		}
	case MarketplaceTypeURL:
		if !strings.HasPrefix(m.Source, "https://") {
			return bad("Use an https:// URL; http:// and git:// are rejected")
		}
	default:
		if isUnsafeProjectPath(m.Source) {
			return bad("Use a project-relative directory without '..'")
		}
	}
	if strings.ContainsAny(m.Source, " \t\r\n\"'`") {
		return bad("A source cannot contain whitespace or quotes")
	}
	return nil
}

// warnEnabledPluginsUnrendered warns when no active preset renders the
// declaration: a harness that cannot express it skips it, never approximates it.
func (c *Config) warnEnabledPluginsUnrendered() {
	if len(c.EnabledPlugins) == 0 || len(c.Presets) == 0 {
		return
	}
	// The presets that render [[enabled_plugins]].
	for _, preset := range c.Presets {
		if slices.Contains([]string{HarnessClaude, HarnessCodex, HarnessOpencode}, preset.BuiltIn) {
			return
		}
	}
	c.Warn("enabled_plugins renders nothing: no active preset is claude, codex or opencode",
		"field", "enabled_plugins")
}
