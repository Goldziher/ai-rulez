package config

import "context"

// Plugin component kinds an installed plugin entry can include or exclude.
const (
	PluginKindSkills   = "skills"
	PluginKindAgents   = "agents"
	PluginKindCommands = "commands"
	PluginKindHooks    = "hooks"
	PluginKindMCP      = "mcp"
	PluginKindRules    = "rules"
)

// PluginKinds lists the component kinds `include` and `exclude` accept.
func PluginKinds() []string {
	return []string{PluginKindSkills, PluginKindAgents, PluginKindCommands, PluginKindHooks, PluginKindMCP, PluginKindRules}
}

// HookBuiltinPlugin marks a HookGroup an installed plugin delivered. Like the
// guard group it is a runtime marker and is never written back to config.toml.
const HookBuiltinPlugin = "installed-plugin"

// InstalledMarketplaceConfig is a plugin marketplace the project reads: a git
// repository, a local directory or an https URL holding a marketplace index
// (.claude-plugin/marketplace.json, .agents/plugins/marketplace.json or
// .cursor-plugin/marketplace.json). It installs nothing by itself; an
// [[installed_plugins]] entry names it as its `marketplace`.
type InstalledMarketplaceConfig struct {
	Name string `yaml:"name" json:"name" toml:"name"`
	// Source is a git URL, a github shorthand (owner/repo), a local path inside
	// the project, or an https URL of a marketplace.json file.
	Source string `yaml:"source" json:"source" toml:"source"`
	// Path is the directory of the repository that holds the marketplace index.
	Path string `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Ref  string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	// Trust is the scan level of the plugins read through this marketplace.
	Trust string `yaml:"trust,omitempty" json:"trust,omitempty" toml:"trust,omitempty"`
	// Version, TagPrefix, IncludePrerelease and MinReleaseAge are as on IncludeConfig.
	Version           string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	TagPrefix         string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty" toml:"tag_prefix,omitempty"`                         //nolint:tagliatelle
	IncludePrerelease bool   `yaml:"include_prerelease,omitempty" json:"include_prerelease,omitempty" toml:"include_prerelease,omitempty"` //nolint:tagliatelle
	MinReleaseAge     string `yaml:"min_release_age,omitempty" json:"min_release_age,omitempty" toml:"min_release_age,omitempty"`          //nolint:tagliatelle
}

// VersionSpec returns the version constraint of the marketplace.
func (m *InstalledMarketplaceConfig) VersionSpec() VersionSpec {
	return resolveVersionSpec(m.Ref, m.Version, m.TagPrefix, m.IncludePrerelease, m.MinReleaseAge)
}

// RequestedRef is what the lock records as the requested ref.
func (m *InstalledMarketplaceConfig) RequestedRef() string {
	return requestedRef(m.Ref, m.VersionSpec())
}

// InstalledPluginConfig is a plugin whose skills, agents, commands, hooks and
// MCP servers the project imports. The plugin comes from an installed
// marketplace (Marketplace) or straight from a repository or directory
// (Source, Path, Ref).
type InstalledPluginConfig struct {
	Name string `yaml:"name" json:"name" toml:"name"`
	// Marketplace names an [[installed_marketplaces]] entry; the plugin is the
	// marketplace entry called Plugin (default: Name).
	Marketplace string `yaml:"marketplace,omitempty" json:"marketplace,omitempty" toml:"marketplace,omitempty"`
	Plugin      string `yaml:"plugin,omitempty" json:"plugin,omitempty" toml:"plugin,omitempty"`
	// Source, Path and Ref name a plugin repository or directory directly, as
	// on installed_skills. Exclusive with Marketplace.
	Source string `yaml:"source,omitempty" json:"source,omitempty" toml:"source,omitempty"`
	Path   string `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	// Format forces the plugin layout (claude, codex, gemini, cursor,
	// agent-plugins); empty detects it.
	Format string `yaml:"format,omitempty" json:"format,omitempty" toml:"format,omitempty"`
	// Include keeps only these component kinds (skills, agents, commands, hooks,
	// mcp, rules); Exclude drops them and wins.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// NamePrefix is prepended to every imported skill, agent, command and MCP server
	// name, and to the output directory of a skill.
	NamePrefix string `yaml:"name_prefix,omitempty" json:"name_prefix,omitempty" toml:"name_prefix,omitempty"` //nolint:tagliatelle
	// Domain and Profiles scope the imported content as on installed_skills.
	Domain   string   `yaml:"domain,omitempty" json:"domain,omitempty" toml:"domain,omitempty"`
	Profiles []string `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
	// Trust is the scan level of the imported content: "error" (default) or "warn".
	Trust string `yaml:"trust,omitempty" json:"trust,omitempty" toml:"trust,omitempty"`
	// EnableHooks lets the plugin's hooks run. Off by default: a hook runs a
	// command on every machine that uses the project.
	EnableHooks bool `yaml:"enable_hooks,omitempty" json:"enable_hooks,omitempty" toml:"enable_hooks,omitempty"` //nolint:tagliatelle
	// EnableMCP imports the plugin's MCP servers; on unless set to false.
	EnableMCP *bool `yaml:"enable_mcp,omitempty" json:"enable_mcp,omitempty" toml:"enable_mcp,omitempty"` //nolint:tagliatelle
	// Version, TagPrefix, IncludePrerelease and MinReleaseAge are as on
	// IncludeConfig; they apply to a plugin installed from Source (a plugin of a
	// marketplace follows the marketplace's own pin).
	Version           string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	TagPrefix         string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty" toml:"tag_prefix,omitempty"`                         //nolint:tagliatelle
	IncludePrerelease bool   `yaml:"include_prerelease,omitempty" json:"include_prerelease,omitempty" toml:"include_prerelease,omitempty"` //nolint:tagliatelle
	MinReleaseAge     string `yaml:"min_release_age,omitempty" json:"min_release_age,omitempty" toml:"min_release_age,omitempty"`          //nolint:tagliatelle
}

// VersionSpec returns the version constraint of the plugin.
func (p *InstalledPluginConfig) VersionSpec() VersionSpec {
	return resolveVersionSpec(p.Ref, p.Version, p.TagPrefix, p.IncludePrerelease, p.MinReleaseAge)
}

// RequestedRef is what the lock records as the requested ref.
func (p *InstalledPluginConfig) RequestedRef() string { return requestedRef(p.Ref, p.VersionSpec()) }

// EntryName is the marketplace entry the plugin installs.
func (p *InstalledPluginConfig) EntryName() string {
	if p.Plugin != "" {
		return p.Plugin
	}
	return p.Name
}

// MCPEnabled reports whether the plugin's MCP servers are imported.
func (p *InstalledPluginConfig) MCPEnabled() bool { return p.EnableMCP == nil || *p.EnableMCP }

// Wants reports whether the entry imports a component kind.
func (p *InstalledPluginConfig) Wants(kind string) bool {
	for _, k := range p.Exclude {
		if k == kind {
			return false
		}
	}
	if len(p.Include) == 0 {
		return true
	}
	for _, k := range p.Include {
		if k == kind {
			return true
		}
	}
	return false
}

// ImportedScanFile is a file or generated text of an installed plugin that the
// import-time security scan reads besides the content files: a hook command,
// a hook script, an MCP server definition. Runtime-only.
type ImportedScanFile struct {
	Path    string
	Content string
	Trust   string
}

// PluginResolution is what resolving the installed plugins of a configuration
// delivered. Content files carry their entry's scope (Profiles, Domain, Trust).
type PluginResolution struct {
	Skills   []ContentFile
	Agents   []ContentFile
	Commands []ContentFile
	Rules    []ContentFile
	Hooks    []HookGroup
	MCP      []MCPServer
	Scan     []ImportedScanFile
}

// ResolveInstalledPluginsCallback resolves the installed marketplaces and
// plugins of a loaded configuration.
type ResolveInstalledPluginsCallback func(ctx context.Context, cfg *Config) (*PluginResolution, error)

// PluginTrust is the scan level of an installed plugin: its own, else the one of
// the marketplace it is installed from, else "" (the default).
func (c *Config) PluginTrust(p *InstalledPluginConfig) string {
	if p.Trust != "" || p.Marketplace == "" {
		return p.Trust
	}
	for i := range c.InstalledMarketplaces {
		if c.InstalledMarketplaces[i].Name == p.Marketplace {
			return c.InstalledMarketplaces[i].Trust
		}
	}
	return ""
}
