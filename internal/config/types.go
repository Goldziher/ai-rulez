package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/builtins"
)

// Config represents the configuration format
type Config struct {
	Schema          string                 `yaml:"$schema,omitempty" json:"$schema,omitempty" toml:"schema,omitempty"`
	Version         string                 `yaml:"version" json:"version" toml:"version"`
	Name            string                 `yaml:"name" json:"name" toml:"name"`
	Description     string                 `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Presets         []Preset               `yaml:"presets,omitempty" json:"presets,omitempty" toml:"presets,omitempty"`
	Default         string                 `yaml:"default,omitempty" json:"default,omitempty" toml:"default,omitempty"`
	Profiles        map[string][]string    `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
	Gitignore       *bool                  `yaml:"gitignore,omitempty" json:"gitignore,omitempty" toml:"gitignore,omitempty"`
	Includes        []IncludeConfig        `yaml:"includes,omitempty" json:"includes,omitempty" toml:"includes,omitempty"`
	InstalledSkills []InstalledSkillConfig `yaml:"installed_skills,omitempty" json:"installed_skills,omitempty" toml:"installed_skills,omitempty"` //nolint:tagliatelle
	Header          *HeaderConfig          `yaml:"header,omitempty" json:"header,omitempty" toml:"header,omitempty"`
	Defaults        *DefaultsConfig        `yaml:"defaults,omitempty" json:"defaults,omitempty" toml:"defaults,omitempty"`
	Builtins        *BuiltinsConfig        `yaml:"builtins,omitempty" json:"builtins,omitempty" toml:"builtins,omitempty"`
	Compact         *bool                  `yaml:"compact,omitempty" json:"compact,omitempty" toml:"compact,omitempty"`
	AgentsMD        bool                   `yaml:"agents_md,omitempty" json:"agents_md,omitempty" toml:"agents_md,omitempty"` //nolint:tagliatelle
	// BundleExclude adds patterns to DefaultBundleExcludes: skill and command
	// resources matching one are not listed in SKILL.md or copied.
	BundleExclude []string `yaml:"bundle_exclude,omitempty" json:"bundle_exclude,omitempty" toml:"bundle_exclude,omitempty"` //nolint:tagliatelle
	// CodexSkillsDir is where the codex preset writes skills, relative to the
	// output base dir. Empty means the documented ".agents/skills"; set
	// ".codex/skills" to keep the pre-4.24 location.
	CodexSkillsDir string              `yaml:"codex_skills_dir,omitempty" json:"codex_skills_dir,omitempty" toml:"codex_skills_dir,omitempty"` //nolint:tagliatelle
	Plugins        []PluginConfig      `yaml:"plugins,omitempty" json:"plugins,omitempty" toml:"plugins,omitempty"`
	Marketplaces   []MarketplaceConfig `yaml:"marketplaces,omitempty" json:"marketplaces,omitempty" toml:"marketplaces,omitempty"`
	Scopes         []ScopeConfig       `yaml:"scopes,omitempty" json:"scopes,omitempty" toml:"scopes,omitempty"`
	MCP            *MCPConfig          `yaml:"mcp,omitempty" json:"mcp,omitempty" toml:"mcp,omitempty"`
	Rules          *RulesConfig        `yaml:"rules,omitempty" json:"rules,omitempty" toml:"rules,omitempty"`
	Lint           *LintConfig         `yaml:"lint,omitempty" json:"lint,omitempty" toml:"lint,omitempty"`
	// Verifiers declares deterministic repo checks run by `ai-rulez verifiers run`.
	Verifiers []VerifierConfig `yaml:"verifiers,omitempty" json:"verifiers,omitempty" toml:"verifiers,omitempty"`
	Usage     *UsageConfig     `yaml:"usage,omitempty" json:"usage,omitempty" toml:"usage,omitempty"`

	// Plugin / Marketplace are the *authoring* (producer) side: they describe a
	// distributable plugin bundle and its marketplace index. Distinct from the
	// consumer Plugins/Marketplaces fields above.
	Plugin      *PluginAuthoring      `yaml:"plugin,omitempty" json:"plugin,omitempty" toml:"plugin,omitempty"`
	Marketplace *MarketplaceAuthoring `yaml:"marketplace,omitempty" json:"marketplace,omitempty" toml:"marketplace,omitempty"`

	// Placement decides whether skills and commands are generated into
	// .claude/skills (core) or shipped only through a plugin.
	Placement *PlacementConfig `yaml:"placement,omitempty" json:"placement,omitempty" toml:"placement,omitempty"`
	// Claude holds Claude Code specific output options.
	Claude *ClaudeConfig `yaml:"claude,omitempty" json:"claude,omitempty" toml:"claude,omitempty"`
	// Codex groups Codex specific options; see CodexConfig.
	Codex *CodexConfig `yaml:"codex,omitempty" json:"codex,omitempty" toml:"codex,omitempty"`

	// Hooks declares lifecycle hooks rendered into each harness's native project
	// settings (.claude/settings.json, .codex/hooks.json, .cursor/hooks.json,
	// .gemini/settings.json, .github/hooks/ai-rulez.json), outside any plugin.
	Hooks []HookGroup `yaml:"hooks,omitempty" json:"hooks,omitempty" toml:"hooks,omitempty"`
	// Permissions declares allow/ask/deny rules for .claude/settings.json and, translated,
	// for every harness with a native permission surface (docs/permissions.md).
	Permissions *Permissions `yaml:"permissions,omitempty" json:"permissions,omitempty" toml:"permissions,omitempty"`

	// Runtime fields (populated during load)
	BaseDir       string `yaml:"-" json:"-" toml:"-"`
	ConfigDir     string `yaml:"-" json:"-" toml:"-"`
	ConfigDirName string `yaml:"-" json:"-" toml:"-"`
	// Run is the state of the generation in progress; nil outside one.
	Run        *RunState    `yaml:"-" json:"-" toml:"-"`
	ConfigFile string       `yaml:"-" json:"-" toml:"-"` // Actual config filename (e.g. "config.toml")
	Content    *ContentTree `yaml:"-" json:"-" toml:"-"`
	// LocalContent holds machine-local override content scanned from
	// .ai-rulez/local/ (rules + context only). It is kept strictly separate
	// from Content so it never lands in committed output; it is emitted only to
	// the per-preset ".local" root variants (CLAUDE.local.md, AGENTS.local.md,
	// ...) and is always gitignored.
	LocalContent *ContentTree `yaml:"-" json:"-" toml:"-"`
	// IncludeMemo caches fetched include and skill sources for the lifetime of
	// one generate run, so loading a second view of the same project (the
	// shared baseline used by the drift guard) never fetches twice. It is owned
	// by the includes resolver; nil means "not created yet".
	IncludeMemo any `yaml:"-" json:"-" toml:"-"`
	// LocalOverlay is set when a config.local.* overlay was merged into this
	// configuration. Such a config is a merged view and is never written back.
	LocalOverlay  *LocalOverlay         `yaml:"-" json:"-" toml:"-"`
	MCPServers    map[string]*MCPServer `yaml:"-" json:"-" toml:"-"`
	MCPServersRaw []MCPServer           `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty" toml:"mcp_servers,omitempty"`

	// SourceHash is a blake3 hash over all sources that contribute to generated
	// output for the active profile (config metadata, content tree, MCP servers,
	// and the generator schema version). Computed once per generation and embedded
	// in every output file's header so that subsequent runs can detect "nothing
	// changed in sources" without re-rendering. Format: "blake3:<hex>".
	SourceHash string `yaml:"-" json:"-" toml:"-"`

	// GeneratedAt is the single timestamp a generate run stamps into every header
	// it writes, resolved once so sibling outputs (CLAUDE.md and AGENTS.md, which
	// are otherwise byte-identical) can never disagree because their renders
	// landed in different seconds. Zero means "not resolved yet"; HeaderTimestamp
	// falls back to the wall clock for callers that render a preview without a
	// generation run. Only read when [header] timestamp opts the line back in.
	GeneratedAt time.Time `yaml:"-" json:"-" toml:"-"`

	// UserScope is set while rendering for `generate --user`: outputs are mapped
	// into the person's home config directories, so renderers leave out keys that
	// only make sense inside a project (MCP servers, plugin registration,
	// machine-local context names).
	UserScope bool `yaml:"-" json:"-" toml:"-"`

	// MCPEnvOverrides are generation-time KEY=VALUE overrides used to resolve
	// MCP env placeholders. They are intentionally not serialized.
	MCPEnvOverrides map[string]string `yaml:"-" json:"-" toml:"-"`

	// MCPEnvFiles are generation-time dotenv files used to resolve MCP env
	// placeholders. Empty means "load .env from BaseDir when present".
	MCPEnvFiles []string `yaml:"-" json:"-" toml:"-"`

	// Analysis, when non-nil, makes the rendering layer record how each output
	// was classified and what each of its sections contained. Only the `tokens`
	// command sets it; generation leaves it nil and pays nothing.
	Analysis *AnalysisCollector `yaml:"-" json:"-" toml:"-"`
}

// HasPluginAuthoring reports whether the configuration produces a plugin or a
// multi-plugin marketplace. Consumer-side [[plugins]] declarations do not count.
func (c *Config) HasPluginAuthoring() bool {
	return c.Plugin != nil || (c.Marketplace != nil && (len(c.Marketplace.Members) > 0 || c.Marketplace.HasDomainPlugins()))
}

// ScopeConfig configures an additional scoped output root for directory-aware
// assistants such as Codex and Claude Code.
type ScopeConfig struct {
	Name    string   `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	Path    string   `yaml:"path" json:"path" toml:"path"`
	Profile string   `yaml:"profile,omitempty" json:"profile,omitempty" toml:"profile,omitempty"`
	Presets []string `yaml:"presets,omitempty" json:"presets,omitempty" toml:"presets,omitempty"`
}

// MCPConfig holds project-level MCP generation options, as opposed to the
// individual server definitions in [[mcp_servers]].
type MCPConfig struct {
	// SelfServer, when true, makes generation add ai-rulez's own MCP server
	// (`npx -y ai-rulez@<version> mcp`) to the project .mcp.json. The entry is
	// merged into the file, so hand-authored servers beside it survive, and no
	// other MCP output (.claude/settings.json, ...) is touched.
	SelfServer bool `yaml:"self_server,omitempty" json:"self_server,omitempty" toml:"self_server,omitempty"`

	// SelfServerVersion pins the ai-rulez version the entry runs. Empty means
	// the version of the running binary, or "latest" for a dev build.
	SelfServerVersion string `yaml:"self_server_version,omitempty" json:"self_server_version,omitempty" toml:"self_server_version,omitempty"`

	// SelfServerCommand replaces the whole launch command (executable followed
	// by its arguments), for installs that do not run through npx, for example
	// ["ai-rulez", "mcp"]. SelfServerVersion is ignored when it is set.
	SelfServerCommand []string `yaml:"self_server_command,omitempty" json:"self_server_command,omitempty" toml:"self_server_command,omitempty"`
}

// HasSelfServer reports whether generation should add the ai-rulez MCP server.
func (c *Config) HasSelfServer() bool {
	return c != nil && c.MCP != nil && c.MCP.SelfServer
}

// SelfMCPServerName is the key of the ai-rulez entry in generated MCP files.
const SelfMCPServerName = "ai-rulez"

// SelfMCPServerEntry builds the .mcp.json entry for ai-rulez's own MCP server.
// binaryVersion is the running binary's version; it is used unless the config
// pins one, and "dev"/empty resolve to "latest".
func (c *Config) SelfMCPServerEntry(binaryVersion string) map[string]any {
	if c.MCP != nil && len(c.MCP.SelfServerCommand) > 0 {
		entry := map[string]any{"type": "stdio", "command": c.MCP.SelfServerCommand[0]}
		if args := c.MCP.SelfServerCommand[1:]; len(args) > 0 {
			entry["args"] = append([]string(nil), args...)
		}
		return entry
	}
	version := ""
	if c.MCP != nil {
		version = c.MCP.SelfServerVersion
	}
	if version == "" {
		version = binaryVersion
	}
	if version == "" || version == "dev" {
		version = "latest"
	}
	return map[string]any{
		"type":    "stdio",
		"command": "npx",
		"args":    []string{"-y", "ai-rulez@" + version, "mcp"},
	}
}

// headerStyleMinimal is the default header style for generated files.
const headerStyleMinimal = "minimal"

// HeaderConfig represents header style configuration for generated files
type HeaderConfig struct {
	Style string `yaml:"style,omitempty" json:"style,omitempty" toml:"style,omitempty"` // "detailed", "compact", or "minimal"
	// Text, when non-empty, replaces the prose generated by Style with an
	// author-supplied banner. It is written verbatim (the generator still wraps
	// it in the output's comment syntax and appends the Content-Hash /
	// Source-Hash freshness lines, subject to Hashes), so a project can point at its own toolchain
	// — a pinned runner, mise, a package manager other than npx — without the
	// predefined style's assumptions. Multi-line TOML via `text = """..."""`.
	Text string `yaml:"text,omitempty" json:"text,omitempty" toml:"text,omitempty"`
	// Timestamp controls whether the "Generated:" line is emitted. Nil means
	// disabled: generated output is byte-reproducible by default, so it can be
	// verified by content hash and two sibling files rendered from the same
	// sources cannot disagree. Set it to true to opt the line back in; pin it
	// with SOURCE_DATE_EPOCH if reproducibility still matters.
	Timestamp *bool `yaml:"timestamp,omitempty" json:"timestamp,omitempty" toml:"timestamp,omitempty"`
	// Hashes selects which freshness lines generated headers carry: "full"
	// (default) writes Content-Hash and Source-Hash, "content" keeps only the
	// per-file Content-Hash, "none" writes neither. Source-Hash covers the whole
	// source set, so under "full" one edit rewrites a line in every generated
	// file; "content" and "none" keep committed output free of that churn.
	Hashes string `yaml:"hashes,omitempty" json:"hashes,omitempty" toml:"hashes,omitempty"`
}

// Header hash modes accepted by [header] hashes.
const (
	HeaderHashesFull    = "full"
	HeaderHashesContent = "content"
	HeaderHashesNone    = "none"
)

// GetHeaderHashes returns the header hash mode, defaulting to "full".
func (h *HeaderConfig) GetHeaderHashes() string {
	if h == nil || h.Hashes == "" {
		return HeaderHashesFull
	}
	return h.Hashes
}

// GetHeaderStyle returns the header style, defaulting to "minimal"
func (h *HeaderConfig) GetHeaderStyle() string {
	if h == nil || h.Style == "" {
		return headerStyleMinimal
	}
	return h.Style
}

// GetCustomHeader returns the author-supplied header text, or "" when unset.
func (h *HeaderConfig) GetCustomHeader() string {
	if h == nil {
		return ""
	}
	return h.Text
}

// ShowTimestamp reports whether generated headers include the "Generated:"
// line. Defaults to false, so generation is reproducible unless a project asks
// for the line.
func (h *HeaderConfig) ShowTimestamp() bool {
	if h == nil || h.Timestamp == nil {
		return false
	}
	return *h.Timestamp
}

// DefaultsConfig represents top-level defaults that propagate into generated outputs
// when individual content files do not override them.
type DefaultsConfig struct {
	// Effort is the default reasoning effort applied across providers that support it.
	// Accepted values: low, medium, high, xhigh, max, inherit. Empty string means "not set".
	// Each preset maps the value to its own vocabulary (see internal/generator/presets/effort.go).
	Effort string `yaml:"effort,omitempty" json:"effort,omitempty" toml:"effort,omitempty"`

	// EffortByPreset overrides Effort for specific presets. Keys are preset names (e.g. "codex",
	// "claude", "devin"). Per-agent frontmatter still wins over this map where the preset
	// supports per-agent effort.
	EffortByPreset map[string]string `yaml:"effort_by_preset,omitempty" json:"effort_by_preset,omitempty" toml:"effort_by_preset,omitempty"`

	// ModelByPreset overrides the agent `model` per preset. Keys are preset names (e.g.
	// "claude", "copilot", "cursor"). Per-agent `<preset>_model` frontmatter still wins
	// over this map; the legacy `model` field is the lowest-priority fallback. Presets
	// that do not emit a per-agent model frontmatter ignore entries for their preset.
	ModelByPreset map[string]string `yaml:"model_by_preset,omitempty" json:"model_by_preset,omitempty" toml:"model_by_preset,omitempty"`

	// OmitAgentFields lists agent frontmatter fields to suppress for every preset,
	// so an agent stays loadable in a tool where a field would be invalid (an
	// unconfigured model/provider, a tool name the tool does not recognize, ...).
	// Recognized values: "model", "effort", "tools", "description". Omitted fields
	// are simply not written; nothing else changes.
	OmitAgentFields []string `yaml:"omit_agent_fields,omitempty" json:"omit_agent_fields,omitempty" toml:"omit_agent_fields,omitempty"` //nolint:tagliatelle
}

// OmitsAgentField reports whether the named agent frontmatter field is configured
// to be omitted.
func (c *Config) OmitsAgentField(field string) bool {
	if c == nil || c.Defaults == nil {
		return false
	}
	for _, f := range c.Defaults.OmitAgentFields {
		if f == field {
			return true
		}
	}
	return false
}

// Rules output modes. In split mode each rule is written as its own file in
// the tool's native rules folder; in inline mode rules are embedded in the
// root file.
const (
	RulesModeSplit  = "split"
	RulesModeInline = "inline"
)

// defaultRulesMode is the mode used when neither rules.mode_by_preset nor
// rules.mode is set.
const defaultRulesMode = RulesModeSplit

// validRulesModes lists the accepted values for rules.mode and rules.mode_by_preset.
var validRulesModes = []string{RulesModeSplit, RulesModeInline}

// RulesConfig controls how rules are written to generated outputs.
type RulesConfig struct {
	// Mode is the default rules output mode: "split" or "inline".
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty" toml:"mode,omitempty"`

	// ModeByPreset overrides Mode for specific presets. Keys are preset names.
	ModeByPreset map[string]string `yaml:"mode_by_preset,omitempty" json:"mode_by_preset,omitempty" toml:"mode_by_preset,omitempty"` //nolint:tagliatelle

	// BazScoped says where the baz preset puts path-scoped rules and context:
	// "nested" (the default) writes them to the AGENTS.md of the directory the
	// globs point into, "root" keeps them in the root AGENTS.md with an
	// "Applies to" line.
	BazScoped string `yaml:"baz_scoped,omitempty" json:"baz_scoped,omitempty" toml:"baz_scoped,omitempty"` //nolint:tagliatelle
}

// Values of rules.baz_scoped.
const (
	BazScopedNested = "nested"
	BazScopedRoot   = "root"
)

// validBazScoped lists the accepted values for rules.baz_scoped.
var validBazScoped = []string{BazScopedNested, BazScopedRoot}

// BazScopedRules returns where the baz preset puts path-scoped items:
// BazScopedNested unless rules.baz_scoped says BazScopedRoot.
func (c *Config) BazScopedRules() string {
	if c != nil && c.Rules != nil && c.Rules.BazScoped != "" {
		return c.Rules.BazScoped
	}
	return BazScopedNested
}

// HasBuiltInPreset reports whether the built-in preset is configured.
func (c *Config) HasBuiltInPreset(name string) bool {
	if c == nil {
		return false
	}
	for i := range c.Presets {
		if c.Presets[i].IsBuiltIn() && c.Presets[i].BuiltIn == name {
			return true
		}
	}
	return false
}

// RulesModeFor returns the rules output mode for a preset:
// rules.mode_by_preset[preset] > rules.mode > the built-in default.
func (c *Config) RulesModeFor(preset string) string {
	if c != nil && c.Rules != nil {
		if v := c.Rules.ModeByPreset[preset]; v != "" {
			return v
		}
		if c.Rules.Mode != "" {
			return c.Rules.Mode
		}
	}
	return defaultRulesMode
}

// RulesModeExplicitFor reports whether the preset's rules mode is set
// explicitly through rules.mode_by_preset.
func (c *Config) RulesModeExplicitFor(preset string) bool {
	if c == nil || c.Rules == nil {
		return false
	}
	return c.Rules.ModeByPreset[preset] != ""
}

// BuiltinsConfig represents the builtins field which can be:
//   - boolean true: enable all builtins
//   - boolean false: disable all builtins (including auto-includes)
//   - array of strings: enable specific builtins (supports "!name" exclusion)
type BuiltinsConfig struct {
	All   *bool    // true = all, false = none
	Names []string // specific builtin names (when All is nil)
}

// IsEnabled returns true if builtins are configured (not nil)
func (b *BuiltinsConfig) IsEnabled() bool {
	return b != nil
}

// IsAll returns true if all builtins should be loaded
func (b *BuiltinsConfig) IsAll() bool {
	return b != nil && b.All != nil && *b.All
}

// IsNone returns true if all builtins should be disabled
func (b *BuiltinsConfig) IsNone() bool {
	return b != nil && b.All != nil && !*b.All
}

// GetNames returns the list of builtin names (empty if All is set)
func (b *BuiltinsConfig) GetNames() []string {
	if b == nil {
		return nil
	}
	return b.Names
}

// UnmarshalYAML implements custom YAML unmarshaling for BuiltinsConfig
func (b *BuiltinsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try boolean first
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		b.All = &boolVal
		return nil
	}

	// Try array of strings
	var names []string
	if err := unmarshal(&names); err != nil {
		return err
	}
	b.Names = names
	return nil
}

// MarshalYAML implements custom YAML marshaling for BuiltinsConfig
func (b BuiltinsConfig) MarshalYAML() (interface{}, error) { //nolint:gocritic // Value receiver required for marshaling
	if b.All != nil {
		return *b.All, nil
	}
	return b.Names, nil
}

// UnmarshalJSON implements custom JSON unmarshaling for BuiltinsConfig
func (b *BuiltinsConfig) UnmarshalJSON(data []byte) error {
	// Try boolean first
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		b.All = &boolVal
		return nil
	}

	// Try array of strings
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	b.Names = names
	return nil
}

// MarshalJSON implements custom JSON marshaling for BuiltinsConfig
func (b BuiltinsConfig) MarshalJSON() ([]byte, error) { //nolint:gocritic // Value receiver required for marshaling
	if b.All != nil {
		return json.Marshal(*b.All)
	}
	return json.Marshal(b.Names)
}

// Preset represents either a built-in preset name or a custom preset configuration
type Preset struct {
	// Built-in preset (e.g., "claude", "cursor")
	BuiltIn string `yaml:"-" json:"-" toml:"-"`

	// Custom preset fields
	Name     string     `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	Type     PresetType `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"`
	Path     string     `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Template string     `yaml:"template,omitempty" json:"template,omitempty" toml:"template,omitempty"`

	// Provider references a declarative provider spec (schema/provider.schema.json)
	// relative to the project root. A provider-backed preset has full parity with
	// a built-in preset (root file, skills/agents/commands, frontmatter, MCP
	// sidecars). When set, Type and Path must be empty.
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" toml:"provider,omitempty"`
}

// PresetType defines the type of custom preset output
type PresetType string

const (
	PresetTypeMarkdown  PresetType = "markdown"
	PresetTypeDirectory PresetType = "directory"
	PresetTypeJSON      PresetType = "json"
)

// Config schema versions accepted by the loader and validator.
const (
	ConfigVersionV3 = "3.0"
	ConfigVersionV4 = "4.0"
)

// UnmarshalYAML implements custom YAML unmarshaling for Preset
func (p *Preset) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try to unmarshal as a string (built-in preset)
	var builtIn string
	if err := unmarshal(&builtIn); err == nil {
		p.BuiltIn = builtIn
		return nil
	}

	// Try to unmarshal as a custom preset object
	type presetAlias Preset
	var custom presetAlias
	if err := unmarshal(&custom); err != nil {
		return err
	}

	p.Name = custom.Name
	p.Type = custom.Type
	p.Path = custom.Path
	p.Template = custom.Template
	p.Provider = custom.Provider
	return nil
}

// MarshalYAML implements custom YAML marshaling for Preset
func (p Preset) MarshalYAML() (interface{}, error) { //nolint:gocritic // Value receiver required for marshaling
	if p.IsBuiltIn() {
		return p.BuiltIn, nil
	}

	// Marshal as custom preset object
	type presetAlias Preset
	return presetAlias(p), nil
}

// UnmarshalJSON implements custom JSON unmarshaling for Preset
func (p *Preset) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string (built-in preset)
	var builtIn string
	if err := json.Unmarshal(data, &builtIn); err == nil {
		p.BuiltIn = builtIn
		return nil
	}

	// Try to unmarshal as a custom preset object
	type presetAlias Preset
	var custom presetAlias
	if err := json.Unmarshal(data, &custom); err != nil {
		return err
	}

	p.Name = custom.Name
	p.Type = custom.Type
	p.Path = custom.Path
	p.Template = custom.Template
	p.Provider = custom.Provider
	return nil
}

// MarshalJSON implements custom JSON marshaling for Preset
func (p Preset) MarshalJSON() ([]byte, error) { //nolint:gocritic // Value receiver required for marshaling
	if p.IsBuiltIn() {
		return json.Marshal(p.BuiltIn)
	}

	// Marshal as custom preset object
	type presetAlias Preset
	return json.Marshal(presetAlias(p))
}

// IsBuiltIn returns true if this is a built-in preset
func (p *Preset) IsBuiltIn() bool {
	return p.BuiltIn != ""
}

// GetName returns the preset name (built-in or custom)
func (p *Preset) GetName() string {
	if p.IsBuiltIn() {
		return p.BuiltIn
	}
	return p.Name
}

// IsValid returns true if the preset is valid
//
// For a built-in name the answer depends on registration: provider-spec presets
// are known only once internal/generator/providers is linked into the binary
// (see AllPresetNames). Import internal/generator before validating presets.
func (p *Preset) IsValid() bool {
	if p.IsBuiltIn() {
		return isValidBuiltInPreset(p.BuiltIn)
	}
	// Provider-backed presets carry a spec reference instead of type/path.
	if p.Provider != "" {
		return p.Name != "" && p.Type == "" && p.Path == ""
	}
	return p.Name != "" && p.Type != "" && p.Path != ""
}

// builtInPresets holds the names accepted as `presets = ["<name>"]`. It is seeded
// with the Go-implemented presets (their PresetName constants) and extended at
// init time by RegisterBuiltInPresetName, which internal/generator/providers
// calls for every embedded provider spec, so a new builtin/*.toml needs no edit
// here. Guarded by builtInPresetsMu because registration is a package-level
// side effect that tests may also trigger.
var (
	builtInPresetsMu sync.RWMutex
	builtInPresets   = map[string]bool{
		string(PresetClaude):      true,
		string(PresetCursor):      true,
		string(PresetGemini):      true,
		string(PresetCopilot):     true,
		string(PresetDevin):       true,
		string(PresetCline):       true,
		string(PresetCodex):       true,
		string(PresetAmp):         true,
		string(PresetJunie):       true,
		string(PresetHermes):      true,
		string(PresetOpenCode):    true,
		string(PresetAntigravity): true,
		string(PresetMCP):         true,
		string(PresetXum):         true,
		string(PresetPi):          true,
		string(PresetBaz):         true,
	}
)

// RegisterBuiltInPresetName makes name a valid built-in preset. It is idempotent.
// Config cannot import the providers package (providers imports config), so
// providers pushes its embedded spec names in from its init().
func RegisterBuiltInPresetName(name string) {
	if name == "" {
		return
	}
	builtInPresetsMu.Lock()
	builtInPresets[name] = true
	builtInPresetsMu.Unlock()
}

func isValidBuiltInPreset(name string) bool {
	builtInPresetsMu.RLock()
	defer builtInPresetsMu.RUnlock()
	return builtInPresets[name]
}

// MCPServer represents an MCP (Model Context Protocol) server configuration
// MCP transport protocol identifiers, used by preset renderers to decide the
// per-tool config shape (stdio command vs remote URL keys).
const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
	TransportSSE   = "sse"
)

type MCPServer struct {
	Name        string            `yaml:"name" json:"name" toml:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Command     string            `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty" json:"args,omitempty" toml:"args,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty" toml:"env,omitempty"`
	Transport   string            `yaml:"transport,omitempty" json:"transport,omitempty" toml:"transport,omitempty"`
	URL         string            `yaml:"url,omitempty" json:"url,omitempty" toml:"url,omitempty"`
	// Headers are HTTP headers sent to a remote (http/sse) server, typically for
	// auth. Values may contain ${VAR} placeholders, resolved like Env.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty" toml:"headers,omitempty"`
	Enabled *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
	// Profiles restricts this server to the named profiles. Empty means every
	// profile, preserving the pre-existing behavior.
	Profiles []string `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`

	// SecretEnvKeys records env keys whose generated values should be treated as
	// sensitive. It is populated during generation and never serialized.
	SecretEnvKeys []string `yaml:"-" json:"-" toml:"-"`
	// SecretHeaderKeys is the Headers counterpart of SecretEnvKeys.
	SecretHeaderKeys []string `yaml:"-" json:"-" toml:"-"`
	// EnvRefs and HeaderRefs map the Env and Headers keys whose value held a
	// ${VAR} placeholder to that value as written. They are populated during
	// generation and never serialized; a tool that expands environment references
	// itself (Codex, Cursor) is given the reference instead of the resolved secret.
	EnvRefs    map[string]string `yaml:"-" json:"-" toml:"-"`
	HeaderRefs map[string]string `yaml:"-" json:"-" toml:"-"`
}

// IsEnabled returns true if the MCP server is enabled (defaults to true if not specified)
func (m *MCPServer) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// GetTransport returns the transport protocol, defaulting to "stdio"
func (m *MCPServer) GetTransport() string {
	if m == nil || m.Transport == "" {
		return TransportStdio
	}
	return m.Transport
}

// ContentTree represents the scanned content from .ai-rulez/ directory
type ContentTree struct {
	Rules    []ContentFile      `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context  []ContentFile      `yaml:"context,omitempty" json:"context,omitempty"`
	Skills   []ContentFile      `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents   []ContentFile      `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands []ContentFile      `yaml:"commands,omitempty" json:"commands,omitempty"`
	Checks   []ContentFile      `yaml:"checks,omitempty" json:"checks,omitempty"`
	Domains  map[string]*Domain `yaml:"domains,omitempty" json:"domains,omitempty"`
}

// Domain represents content from a specific domain directory
type Domain struct {
	Name          string        `yaml:"name" json:"name"`
	Rules         []ContentFile `yaml:"rules,omitempty" json:"rules,omitempty"`
	Context       []ContentFile `yaml:"context,omitempty" json:"context,omitempty"`
	Skills        []ContentFile `yaml:"skills,omitempty" json:"skills,omitempty"`
	Agents        []ContentFile `yaml:"agents,omitempty" json:"agents,omitempty"`
	Commands      []ContentFile `yaml:"commands,omitempty" json:"commands,omitempty"`
	Checks        []ContentFile `yaml:"checks,omitempty" json:"checks,omitempty"`
	Builtin       bool          `yaml:"-" json:"-"` // true if loaded from builtins
	BuiltinScoped bool          `yaml:"-" json:"-"` // true if a builtin loaded only for the profiles that name it (builtin:<name>)
	FromInclude   bool          `yaml:"-" json:"-"` // true if loaded from an external include
}

// ContentFile represents a single content file with optional frontmatter
type ContentFile struct {
	Name     string    `yaml:"name" json:"name"`
	Path     string    `yaml:"path" json:"path"`
	Content  string    `yaml:"content" json:"content"`
	Metadata *Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	// Profiles scopes this file to the named profiles (used by installed
	// skills). Empty means every profile. Runtime-only; not serialized.
	Profiles []string `yaml:"-" json:"-"`

	// Resources holds skill supporting files (references/, scripts/, assets/)
	// loaded alongside SKILL.md. Always empty for non-skill content.
	Resources []SkillResource `yaml:"-" json:"-"`

	// MalformedFrontmatter is true when the source file contained a delimited
	// YAML frontmatter block (---...---) but its content was unparseable.
	// It is set during loading and used by Config.Validate to fail fast.
	MalformedFrontmatter bool `yaml:"-" json:"-"`
}

// SkillResource is one supporting file bundled with a skill.
//
// The canonical Agent Skills layout (followed by Claude Code, OpenAI Codex,
// and the agentskills.io standard) places these under three subdirectories:
//
//	references/  — markdown docs the agent reads on demand
//	scripts/     — executable scripts the agent invokes
//	assets/      — files used in output (templates, images, etc.)
//
// Resources are emitted as individual files under the rendered skill
// directory and indexed from SKILL.md so the agent can discover them.
type SkillResource struct {
	// Kind is one of "references", "scripts", "assets".
	Kind string
	// RelPath is the resource path relative to the skill root, including the
	// kind subdirectory (e.g. "references/api.md").
	RelPath string
	// Content holds the raw bytes of the file. Bytes (not string) so binary
	// assets round-trip without UTF-8 corruption.
	Content []byte
	// Mode is the file permission bits read from disk. Preserved through
	// generation so bundled scripts keep their executable bit.
	Mode os.FileMode
	// Description is parsed from a reference file's frontmatter `description`
	// field, or falls back to the first non-empty markdown line. Empty for
	// scripts and assets, or when no description is available.
	Description string
}

// Metadata represents parsed frontmatter metadata.
//
// Tools, Skills, and Keywords are list-valued and need typed handling because
// YAML sequences cannot round-trip through map[string]string — they would be
// stringified via fmt %v ("[a b c]") instead of preserved as proper lists.
type Metadata struct {
	Priority string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Targets  []string `yaml:"targets,omitempty" json:"targets,omitempty"`
	Aliases  []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Tools    []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills   []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Keywords []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Usage    string   `yaml:"usage,omitempty" json:"usage,omitempty"`
	Shortcut string   `yaml:"shortcut,omitempty" json:"shortcut,omitempty"`
	Category string   `yaml:"category,omitempty" json:"category,omitempty"`
	Effort   string   `yaml:"effort,omitempty" json:"effort,omitempty"`
	// Activation selects when a rule or context file applies: always, glob,
	// auto, or manual. See ResolveActivation for precedence and defaults.
	Activation string `yaml:"activation,omitempty" json:"activation,omitempty"`
	// Globs and Paths declare the files a rule applies to. They are two
	// spellings of the same idea (Cursor calls them globs, Claude Code calls
	// them paths); either populates the same path-scope used by presets.
	Globs []string          `yaml:"globs,omitempty" json:"globs,omitempty"`
	Paths []string          `yaml:"paths,omitempty" json:"paths,omitempty"`
	Extra map[string]string `yaml:",inline" json:",inline"`

	// extraNodes holds the original, typed YAML value of every Extra key (nested
	// maps, lists, booleans, numbers, dates). Extra keeps a string form for the
	// lookups that only need text; generators emit the typed value through
	// TypedExtra so the frontmatter round-trips without Go syntax.
	extraNodes map[string]*yaml.Node
}

// PathScope returns the file globs a rule declares, from either `globs` or
// `paths` frontmatter. Empty means the rule is not path-scoped.
func (m *Metadata) PathScope() []string {
	if m == nil {
		return nil
	}
	if len(m.Globs) > 0 {
		return NormalizeGlobs(m.Globs)
	}
	return NormalizeGlobs(m.Paths)
}

// GetPriority returns the priority as a Priority type, defaulting to medium
func (m *Metadata) GetPriority() Priority {
	if m == nil || m.Priority == "" {
		return PriorityMedium
	}
	p, err := ParsePriority(m.Priority)
	if err != nil {
		return PriorityMedium
	}
	return p
}

// HasTargets returns true if targets are specified
func (m *Metadata) HasTargets() bool {
	return m != nil && len(m.Targets) > 0
}

// Helper methods for Config

// ShouldUpdateGitignore returns whether .gitignore should be updated
func (c *Config) ShouldUpdateGitignore() bool {
	if c.Gitignore == nil {
		return true
	}
	return *c.Gitignore
}

// GetDefaultProfile returns the default profile name
func (c *Config) GetDefaultProfile() string {
	return c.Default
}

// GetProfileDomains returns the list of domains for a profile. A composed value
// ("base,backend") returns the de-duplicated union of its elements' domains, in
// the order they are first named.
func (c *Config) GetProfileDomains(profile string) []string {
	if profile == "" {
		profile = c.Default
	}
	names := SplitProfileNames(profile)
	switch len(names) {
	case 0:
		return nil
	case 1:
		return c.Profiles[names[0]]
	}

	union := make([]string, 0, len(names))
	seen := make(map[string]bool)
	for _, name := range names {
		for _, domain := range c.Profiles[name] {
			if seen[domain] {
				continue
			}
			seen[domain] = true
			union = append(union, domain)
		}
	}
	return union
}

// HasProfile returns true if the profile exists. Every element of a composed
// value must exist; a value with no elements at all exists only if a profile was
// literally defined under that name.
func (c *Config) HasProfile(profile string) bool {
	names := SplitProfileNames(profile)
	if len(names) == 0 {
		_, ok := c.Profiles[profile]
		return ok
	}
	for _, name := range names {
		if _, ok := c.Profiles[name]; !ok {
			return false
		}
	}
	return true
}

// GetVersion returns the config version
func (c *Config) GetVersion() string {
	return c.Version
}

// IsV3 returns true if this is a V3 config (version == "3.0")
func (c *Config) IsV3() bool {
	return c.Version == ConfigVersionV3
}

// IsV4 returns true if this is a V4 config (version == "4.0")
func (c *Config) IsV4() bool {
	return c.Version == ConfigVersionV4
}

// IsCompact reports whether compact rendering is enabled. When true, presets
// omit per-rule "**Priority:**" annotations from inline rule sections to reduce
// output size. Defaults to false (full output).
func (c *Config) IsCompact() bool {
	return c != nil && c.Compact != nil && *c.Compact
}

// GetHeaderStyle returns the configured header style ("detailed", "compact", or "minimal").
// Defaults to "minimal" when no header style is configured.
func (c *Config) GetHeaderStyle() string {
	if c.Header == nil {
		return headerStyleMinimal
	}
	return c.Header.GetHeaderStyle()
}

// GetHeaderHashes returns the configured header hash mode ("full", "content" or
// "none"), defaulting to "full".
func (c *Config) GetHeaderHashes() string {
	if c == nil {
		return HeaderHashesFull
	}
	return c.Header.GetHeaderHashes()
}

// ShowHeaderTimestamp reports whether generated headers include the
// "Generated:" line. Defaults to false; see HeaderConfig.Timestamp.
func (c *Config) ShowHeaderTimestamp() bool {
	if c == nil {
		return false
	}
	return c.Header.ShowTimestamp()
}

// HeaderTimestamp returns the timestamp every header of the current run carries.
// Generation resolves it once into GeneratedAt; the wall-clock fallback covers
// callers that render a header outside a generation run, such as a preview.
func (c *Config) HeaderTimestamp() time.Time {
	if c == nil || c.GeneratedAt.IsZero() {
		return ResolveGenerationTime()
	}
	return c.GeneratedAt
}

// sourceDateEpochEnv is the reproducible-builds convention for pinning the
// timestamp embedded in build artifacts.
const sourceDateEpochEnv = "SOURCE_DATE_EPOCH"

// ResolveGenerationTime returns the timestamp for one generation run: the value
// of SOURCE_DATE_EPOCH when it holds a parsable Unix second count, otherwise the
// wall clock. A malformed value falls through to the clock rather than failing
// generation or silently stamping the epoch, which would be indistinguishable
// from a deliberate pin to 1970.
func ResolveGenerationTime() time.Time {
	raw, ok := os.LookupEnv(sourceDateEpochEnv)
	if !ok {
		return time.Now()
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.Unix(seconds, 0)
}

// GetContentForProfile returns all content for a given profile.
// Root-level slices contain only root content. Domains are placed in the
// Domains map so that preset generators can combine them via
// combineContentFiles / getAllDomain* helpers without duplication.
func (c *Config) GetContentForProfile(profile string) (*ContentTree, error) {
	return c.SelectContentForProfile(c.Content, profile)
}

// SelectContentForProfile applies a profile's domain selection to a content tree:
// the root content plus the domains the profile names, the globally active
// builtins and every domain that came from an include. It is the single
// implementation of that selection, used for the shared tree and for the
// machine-local one (.ai-rulez/local/) alike.
func (c *Config) SelectContentForProfile(content *ContentTree, profile string) (*ContentTree, error) {
	if content == nil {
		return nil, ErrNoContent
	}

	profileDomains := c.GetProfileDomains(profile)

	// Installed skills may be scoped to profiles; drop the ones not active.
	activeProfile := profile
	if activeProfile == "" {
		activeProfile = c.Default
	}
	rootSkills := FilterContentFilesByProfile(content.Skills, activeProfile)

	// Build filtered domains map: profile-listed domains + global builtins + FromInclude
	activeDomains := make(map[string]*Domain)

	// First pass: include globally-active builtins and every FromInclude domain
	// unconditionally. A builtin loaded only because a profile named it is
	// excluded here and re-added by the second pass for that profile alone.
	for name, domain := range content.Domains {
		if domain.FromInclude || (domain.Builtin && !domain.BuiltinScoped) {
			activeDomains[name] = domain
		}
	}

	// Second pass: add profile-specified domains (may overlap with FromInclude).
	// A "builtin:<name>" element selects a profile-scoped builtin; a bare name
	// selects an on-disk or include domain and never widens a scoped builtin
	// into a profile that did not ask for it.
	for _, ref := range profileDomains {
		name := builtins.TrimRefPrefix(ref)
		domain, ok := content.Domains[name]
		if !ok {
			continue
		}
		if domain.BuiltinScoped && !builtins.HasRefPrefix(ref) {
			continue
		}
		activeDomains[name] = domain
	}

	return &ContentTree{
		Rules:    content.Rules,
		Context:  content.Context,
		Skills:   rootSkills,
		Agents:   content.Agents,
		Commands: content.Commands,
		Checks:   content.Checks,
		Domains:  activeDomains,
	}, nil
}

// Helper methods for ContentTree

// IsEmpty reports whether the tree carries no content of any kind.
func (t *ContentTree) IsEmpty() bool {
	if t == nil {
		return true
	}
	if len(t.Rules) > 0 || len(t.Context) > 0 || len(t.Skills) > 0 ||
		len(t.Agents) > 0 || len(t.Commands) > 0 || len(t.Checks) > 0 {
		return false
	}
	for _, domain := range t.Domains {
		if domain != nil && (len(domain.Rules) > 0 || len(domain.Context) > 0 ||
			len(domain.Skills) > 0 || len(domain.Agents) > 0 || len(domain.Commands) > 0 || len(domain.Checks) > 0) {
			return false
		}
	}
	return true
}

// GetAllContentFiles returns all content files from the tree
func (t *ContentTree) GetAllContentFiles() []ContentFile {
	var files []ContentFile
	files = append(files, t.Rules...)
	files = append(files, t.Context...)
	files = append(files, t.Skills...)
	files = append(files, t.Agents...)
	files = append(files, t.Commands...)
	files = append(files, t.Checks...)
	for _, domain := range t.Domains {
		files = append(files, domain.Rules...)
		files = append(files, domain.Context...)
		files = append(files, domain.Skills...)
		files = append(files, domain.Agents...)
		files = append(files, domain.Commands...)
		files = append(files, domain.Checks...)
	}
	return files
}

// GetRulesForDomains returns rules for specified domains (including root)
func (t *ContentTree) GetRulesForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Rules))
	copy(files, t.Rules)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Rules...)
		}
	}
	return files
}

// GetContextForDomains returns context files for specified domains (including root)
func (t *ContentTree) GetContextForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Context))
	copy(files, t.Context)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Context...)
		}
	}
	return files
}

// GetSkillsForDomains returns skills for specified domains (including root)
func (t *ContentTree) GetSkillsForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Skills))
	copy(files, t.Skills)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Skills...)
		}
	}
	return files
}

// GetAgentsForDomains returns agents for specified domains (including root)
func (t *ContentTree) GetAgentsForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Agents))
	copy(files, t.Agents)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Agents...)
		}
	}
	return files
}

// GetCommandsForDomains returns commands for specified domains (including root)
func (t *ContentTree) GetCommandsForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Commands))
	copy(files, t.Commands)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Commands...)
		}
	}
	return files
}

// GetChecksForDomains returns checks for specified domains (including root)
func (t *ContentTree) GetChecksForDomains(domains []string) []ContentFile {
	files := make([]ContentFile, len(t.Checks))
	copy(files, t.Checks)

	for _, domainName := range domains {
		if domain, ok := t.Domains[domainName]; ok {
			files = append(files, domain.Checks...)
		}
	}
	return files
}

// Helper methods for ContentFile

// GetFileExtension returns the file extension for the content file
func (f *ContentFile) GetFileExtension() string {
	if f == nil || f.Path == "" {
		return ""
	}
	if idx := len(f.Path) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if f.Path[i] == '.' {
				return f.Path[i:]
			}
			if f.Path[i] == '/' {
				break
			}
		}
	}
	return ""
}

// IsMarkdown returns true if the content file is markdown
func (f *ContentFile) IsMarkdown() bool {
	ext := f.GetFileExtension()
	return ext == markdownExt || ext == ".markdown"
}

// IncludeConfig represents a content source (git repo or local path)
type IncludeConfig struct {
	Name          string   `yaml:"name" json:"name" toml:"name"`
	Source        string   `yaml:"source" json:"source" toml:"source"`
	Path          string   `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Include       []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	Ref           string   `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	InstallTo     string   `yaml:"install_to,omitempty" json:"install_to,omitempty" toml:"install_to,omitempty"`             //nolint:tagliatelle
	MergeStrategy string   `yaml:"merge_strategy,omitempty" json:"merge_strategy,omitempty" toml:"merge_strategy,omitempty"` //nolint:tagliatelle
	LocalOverride string   `yaml:"local_override,omitempty" json:"local_override,omitempty" toml:"local_override,omitempty"` //nolint:tagliatelle
}

// InstalledSkillConfig represents a named skill to install from an external source
type InstalledSkillConfig struct {
	Name          string   `yaml:"name" json:"name" toml:"name"`
	Source        string   `yaml:"source" json:"source" toml:"source"`
	Path          string   `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Ref           string   `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	LocalOverride string   `yaml:"local_override,omitempty" json:"local_override,omitempty" toml:"local_override,omitempty"` //nolint:tagliatelle
	Profiles      []string `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
}

// GetPath returns the path within the repo, defaulting to "skills/<name>"
func (s *InstalledSkillConfig) GetPath() string {
	if s.Path != "" {
		return s.Path
	}
	return "skills/" + s.Name
}

// IncludeLock tracks resolved include sources
type IncludeLock struct {
	Includes map[string]IncludeLockEntry `yaml:"includes" json:"includes"`
}

// IncludeLockEntry represents a locked include source
type IncludeLockEntry struct {
	Source      string    `yaml:"source" json:"source"`
	Type        string    `yaml:"type" json:"type"`                                     // "git" or "local"
	ResolvedRef string    `yaml:"resolved_ref,omitempty" json:"resolved_ref,omitempty"` // git only
	ResolvedAt  time.Time `yaml:"resolved_at" json:"resolved_at"`
}
