package config

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
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
	OKF            *OKFConfig          `yaml:"okf,omitempty" json:"okf,omitempty" toml:"okf,omitempty"`
	LLMsTxt        *LLMsTxtConfig      `yaml:"llms_txt,omitempty" json:"llms_txt,omitempty" toml:"llms_txt,omitempty"` //nolint:tagliatelle
	Usage          *UsageConfig        `yaml:"usage,omitempty" json:"usage,omitempty" toml:"usage,omitempty"`
	// Roles map a job to a slice of the shared content (see roles.go); RoleManifest
	// is the [role_manifest] table, kept apart because [[roles]] is an array.
	Roles        []RoleConfig        `yaml:"roles,omitempty" json:"roles,omitempty" toml:"roles,omitempty"`
	RoleManifest *RoleManifestConfig `yaml:"role_manifest,omitempty" json:"role_manifest,omitempty" toml:"role_manifest,omitempty"` //nolint:tagliatelle
	// Lock configures content pinning in ai-rulez.lock (see internal/contentlock).
	Lock *LockConfig `yaml:"lock,omitempty" json:"lock,omitempty" toml:"lock,omitempty"`
	// Governance is the [governance] table: which content needs a reviewer approval (types_governance.go).
	Governance *GovernanceConfig `yaml:"governance,omitempty" json:"governance,omitempty" toml:"governance,omitempty"`
	// Catalog is the [catalog] table: defaults for `ai-rulez catalog` (types_catalog.go).
	Catalog *CatalogConfig `yaml:"catalog,omitempty" json:"catalog,omitempty" toml:"catalog,omitempty"`
	// Signing is the [signing] table: who may sign the lock and how fresh the signature must be (types_signing.go).
	Signing *SigningConfig `yaml:"signing,omitempty" json:"signing,omitempty" toml:"signing,omitempty"`
	// Publish is the [publish] table: what `ai-rulez publish` ships and its policy gates (types_publish.go).
	Publish *PublishConfig `yaml:"publish,omitempty" json:"publish,omitempty" toml:"publish,omitempty"`
	// ARD is the [ard] table: the publisher and namespace of the Agentic Resource Discovery manifest (types_ard.go).
	ARD *ARDConfig `yaml:"ard,omitempty" json:"ard,omitempty" toml:"ard,omitempty"`

	// Dynamic skill loading (types_dynamic.go, delivery.go).
	Skills         *SkillsConfig           `yaml:"skills,omitempty" json:"skills,omitempty" toml:"skills,omitempty"`
	DomainSettings map[string]DomainConfig `yaml:"domains,omitempty" json:"domains,omitempty" toml:"domains,omitempty"`
	SkillSources   []SkillSourceConfig     `yaml:"skill_sources,omitempty" json:"skill_sources,omitempty" toml:"skill_sources,omitempty"` //nolint:tagliatelle
	// ServeMode is set while a skills server renders: every skill is rendered,
	// served ones included, and no dynamic-skills stub is added.
	ServeMode bool `yaml:"-" json:"-" toml:"-"`
	// QuietDeliveryWarnings keeps GeneratePresets from logging the AR992
	// fallback warnings: a check that renders only to inspect the result (the
	// validate stub check) reports them as findings instead.
	QuietDeliveryWarnings bool `yaml:"-" json:"-" toml:"-"`
	// roleDelivery is the per-skill delivery of the role being rendered (see
	// SetRoleDelivery); nil when no role is active.
	roleDelivery map[string]string
	// LLM configures model access for features that call a model; nothing calls out unless allow_network is true.
	LLM       *llm.Config      `yaml:"llm,omitempty" json:"llm,omitempty" toml:"llm,omitempty"`
	Telemetry *TelemetryConfig `yaml:"telemetry,omitempty" json:"telemetry,omitempty" toml:"telemetry,omitempty"`
	// Review configures `ai-rulez review` (rubric, content mode, exclusions, spend ceilings).
	Review *ReviewConfig `yaml:"review,omitempty" json:"review,omitempty" toml:"review,omitempty"`
	// Improve configures `ai-rulez improve run` defaults: gate thresholds, the optimizer and its isolation (types_improve.go).
	Improve *ImproveConfig `yaml:"improve,omitempty" json:"improve,omitempty" toml:"improve,omitempty"`
	// Search configures `ai-rulez search` and find_skill ranking: lexical by default, optionally hybrid with embeddings (docs/search.md).
	Search *skillsearch.Config `yaml:"search,omitempty" json:"search,omitempty" toml:"search,omitempty"`
	// Verifiers declares deterministic repo checks run by `ai-rulez verifiers run`.
	Verifiers []VerifierConfig `yaml:"verifiers,omitempty" json:"verifiers,omitempty" toml:"verifiers,omitempty"`
	// VerifiersSettings holds the limits and policy of `verifiers run` ([verifiers_settings]).
	VerifiersSettings *VerifiersSettings `yaml:"verifiers_settings,omitempty" json:"verifiers_settings,omitempty" toml:"verifiers_settings,omitempty"`

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
	// Guard configures the built-in hook that blocks edits to generated files.
	Guard *GuardConfig `yaml:"guard,omitempty" json:"guard,omitempty" toml:"guard,omitempty"`
	// Permissions declares allow/ask/deny rules for .claude/settings.json and, translated,
	// for every harness with a native permission surface (docs/permissions.md).
	Permissions *Permissions `yaml:"permissions,omitempty" json:"permissions,omitempty" toml:"permissions,omitempty"`

	// Runtime fields (populated during load)
	BaseDir string `yaml:"-" json:"-" toml:"-"`
	// ContentProblems lists project content paths refused by the symlink
	// policy; validate reports them as errors.
	ContentProblems []ContentProblem `yaml:"-" json:"-" toml:"-"`
	ConfigDir       string           `yaml:"-" json:"-" toml:"-"`
	ConfigDirName   string           `yaml:"-" json:"-" toml:"-"`

	// PolicyOutcome is what the organization policy clamped and reported at load
	// time (docs/policy.md); nil when no policy is in force.
	PolicyOutcome *PolicyOutcome `yaml:"-" json:"-" toml:"-"`

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
	// ResolutionState holds what a load's include and skill fetches resolved to
	// (commit, digest, tag, problems) so `BuildLock` can write it down. It is
	// owned by the includes resolver and is per load: two loads in one process
	// keep separate records and never clobber each other. nil means "not
	// created yet".
	ResolutionState any `yaml:"-" json:"-" toml:"-"`
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
	// Host is the environment, clock, process runner and logger this config was
	// loaded with (WithHost); the zero value is the real process. Generation,
	// includes and lint read them from here instead of the process.
	Host ambient.Host `yaml:"-" json:"-" toml:"-"`
	// Diag collects the warnings of the generate run this config is rendered by
	// (see internal/diag); nil outside a run, where warnings go straight to the
	// logger.
	Diag *diag.Collector `yaml:"-" json:"-" toml:"-"`
	// Registry resolves preset names to generators (WithRegistry); the Generator
	// supplies the default one when a config carries none.
	Registry *Registry `yaml:"-" json:"-" toml:"-"`
	// RulesDirs holds the rules folders custom provider specs added to this
	// project's built-in set; nil is the built-in set. Copies of a Config share it.
	RulesDirs *RulesDirSet `yaml:"-" json:"-" toml:"-"`
	// Resolve are the resolvers this config was loaded with (WithResolvers); nested
	// loads of the same project (a baseline render, a shared view) reuse them.
	Resolve Resolvers `yaml:"-" json:"-" toml:"-"`
	// LockPolicy is how this config was loaded against ai-rulez.lock
	// (WithLockPolicy); the includes resolvers and nested loads read it.
	LockPolicy LockPolicy `yaml:"-" json:"-" toml:"-"`
	// Workspace is the project tree this config was loaded from (WithWorkspace,
	// or the repository containing BaseDir); nil on a Config built by hand.
	Workspace workspace.Workspace `yaml:"-" json:"-" toml:"-"`

	// PolicyDir is the project directory the organization policy is discovered
	// from (WithPolicyDir); empty means BaseDir. A snapshot of an old revision is
	// loaded from a temporary directory with no remote, and is judged by the policy
	// of the project it is a snapshot of.
	PolicyDir string `yaml:"-" json:"-" toml:"-"`

	// enforcer is the organization policy this configuration was loaded under.
	enforcer PolicyEnforcer

	// DeferMalformedFrontmatter makes Validate skip the malformed-frontmatter
	// failure because the caller reports each such file as a lint finding.
	DeferMalformedFrontmatter bool `yaml:"-" json:"-" toml:"-"`

	// frontmatterErrors records WithFrontmatterErrors, for ReloadOptions.
	frontmatterErrors bool

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
	// SelfServer makes generation add ai-rulez's own MCP server
	// (`npx -y ai-rulez@<version> mcp`) to the project .mcp.json. It defaults to
	// true in v5, so a project gets the server out of the box; set it to false
	// (or pass generate --no-self-mcp) to turn it off. The entry is merged into
	// the file, so hand-authored servers beside it survive, and no other MCP
	// output (.claude/settings.json, ...) is touched.
	SelfServer *bool `yaml:"self_server,omitempty" json:"self_server,omitempty" toml:"self_server,omitempty"`

	// SelfServerVersion pins the ai-rulez version the entry runs. Empty means
	// the version of the running binary, or "latest" for a dev build.
	SelfServerVersion string `yaml:"self_server_version,omitempty" json:"self_server_version,omitempty" toml:"self_server_version,omitempty"`

	// SelfServerCommand replaces the whole launch command (executable followed
	// by its arguments), for installs that do not run through npx, for example
	// ["ai-rulez", "mcp"]. SelfServerVersion is ignored when it is set.
	SelfServerCommand []string `yaml:"self_server_command,omitempty" json:"self_server_command,omitempty" toml:"self_server_command,omitempty"`
}

// HasSelfServer reports whether generation should add ai-rulez's own MCP server.
// It is on by default: only an explicit self_server = false (or --no-self-mcp)
// turns it off.
func (c *Config) HasSelfServer() bool {
	if c == nil {
		return false
	}
	if c.MCP == nil || c.MCP.SelfServer == nil {
		return true
	}
	return *c.MCP.SelfServer
}

// SetSelfServer forces the self server on or off, overriding the default and any
// configured value. The generate --no-self-mcp flag calls it.
func (c *Config) SetSelfServer(on bool) {
	if c.MCP == nil {
		c.MCP = &MCPConfig{}
	}
	c.MCP.SelfServer = &on
}

// SelfMCPServerName is the key of the ai-rulez entry in generated MCP files.
const SelfMCPServerName = "ai-rulez"

// mcpKeyCommand is the .mcp.json key naming the command a stdio server runs.
const mcpKeyCommand = "command"

// SelfMCPServerEntry builds the .mcp.json entry for ai-rulez's own MCP server.
// binaryVersion is the running binary's version; it is used unless the config
// pins one, and "dev"/empty resolve to "latest".
func (c *Config) SelfMCPServerEntry(binaryVersion string) map[string]any {
	if c.MCP != nil && len(c.MCP.SelfServerCommand) > 0 {
		entry := map[string]any{"type": "stdio", mcpKeyCommand: c.MCP.SelfServerCommand[0]}
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
		"type":        "stdio",
		mcpKeyCommand: "npx",
		"args":        []string{"-y", "ai-rulez@" + version, "mcp"},
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
	// writes Content-Hash and Source-Hash, "content" (default) keeps only the
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

// GetHeaderHashes returns the header hash mode, defaulting to "content".
func (h *HeaderConfig) GetHeaderHashes() string {
	if h == nil || h.Hashes == "" {
		return HeaderHashesContent
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
	// Package is the package URL (purl) of the package a command-based server
	// runs, for example "pkg:npm/%40scope/server@1.4.2". `ai-rulez sbom` uses it
	// instead of guessing the package from command and args. It is never written
	// to a harness file.
	Package string `yaml:"package,omitempty" json:"package,omitempty" toml:"package,omitempty"`

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

// Helper methods for Config

// ShouldUpdateGitignore returns whether .gitignore should be updated
func (c *Config) ShouldUpdateGitignore() bool {
	if c.Gitignore == nil {
		return false
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
// "none"), defaulting to "content".
func (c *Config) GetHeaderHashes() string {
	if c == nil {
		return HeaderHashesContent
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
	if c == nil {
		return ResolveGenerationTimeIn(ambient.Host{})
	}
	if c.GeneratedAt.IsZero() {
		return ResolveGenerationTimeIn(c.Host)
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
func ResolveGenerationTime() time.Time { return ResolveGenerationTimeIn(ambient.Host{}) }

// ResolveGenerationTimeIn is ResolveGenerationTime with the environment and
// clock taken from host.
func ResolveGenerationTimeIn(host ambient.Host) time.Time {
	raw, ok := host.LookupEnv(sourceDateEpochEnv)
	if !ok {
		return host.Now()
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return host.Now()
	}
	return time.Unix(seconds, 0)
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
	// Format names a non-default source layout. "okf" reads the source as an
	// Open Knowledge Format bundle instead of an .ai-rulez directory.
	Format string `yaml:"format,omitempty" json:"format,omitempty" toml:"format,omitempty"`
	// Version is a semver constraint resolved against the repository's tags
	// (see VersionSpec); it excludes Ref.
	Version string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	// TagPrefix scopes Version to tags that start with it.
	TagPrefix string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty" toml:"tag_prefix,omitempty"` //nolint:tagliatelle
	// IncludePrerelease admits prerelease tags the constraint does not name.
	IncludePrerelease bool `yaml:"include_prerelease,omitempty" json:"include_prerelease,omitempty" toml:"include_prerelease,omitempty"` //nolint:tagliatelle
	// MinReleaseAge holds back a tag younger than this ("7d", "12h", "2w"); needs Version.
	MinReleaseAge string `yaml:"min_release_age,omitempty" json:"min_release_age,omitempty" toml:"min_release_age,omitempty"` //nolint:tagliatelle
}

// IncludeFormatOKF reads an include as an OKF bundle (see docs/okf.md).
const IncludeFormatOKF = "okf"

// InstalledSkillConfig represents a named skill to install from an external source
type InstalledSkillConfig struct {
	Name          string   `yaml:"name" json:"name" toml:"name"`
	Source        string   `yaml:"source" json:"source" toml:"source"`
	Path          string   `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Ref           string   `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	LocalOverride string   `yaml:"local_override,omitempty" json:"local_override,omitempty" toml:"local_override,omitempty"` //nolint:tagliatelle
	Profiles      []string `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
	// Version, TagPrefix and IncludePrerelease are as on IncludeConfig.
	Version           string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	TagPrefix         string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty" toml:"tag_prefix,omitempty"`                         //nolint:tagliatelle
	IncludePrerelease bool   `yaml:"include_prerelease,omitempty" json:"include_prerelease,omitempty" toml:"include_prerelease,omitempty"` //nolint:tagliatelle
	MinReleaseAge     string `yaml:"min_release_age,omitempty" json:"min_release_age,omitempty" toml:"min_release_age,omitempty"`          //nolint:tagliatelle
}

// GetPath returns the path within the repo, defaulting to "skills/<name>"
func (s *InstalledSkillConfig) GetPath() string {
	if s.Path != "" {
		return s.Path
	}
	return "skills/" + s.Name
}
