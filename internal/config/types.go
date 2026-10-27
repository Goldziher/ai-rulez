package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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
	Schema      string   `yaml:"$schema,omitempty" json:"$schema,omitempty" toml:"schema,omitempty"`
	Version     string   `yaml:"version" json:"version" toml:"version"`
	Name        string   `yaml:"name" json:"name" toml:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Presets     []Preset `yaml:"presets,omitempty" json:"presets,omitempty" toml:"presets,omitempty"`
	// RecursiveExclude lists directories, relative to this project root, that
	// `generate --recursive` run from here never searches for configuration: a
	// glob such as "svc/*/gen" or a plain directory such as "tests/fixtures". The
	// --exclude flag adds to it.
	RecursiveExclude []string `yaml:"recursive_exclude,omitempty" json:"recursive_exclude,omitempty" toml:"recursive_exclude,omitempty"`
	// DefaultLayers names config.<name>.toml files beside config.toml (in .agents/.ai-rulez/
	// for an .agents project) that are always
	// merged over it, in order, before the layers AI_RULEZ_LAYERS names. A listed layer
	// with no file is an error. The key is ignored inside a layer.
	DefaultLayers []string            `yaml:"default_layers,omitempty" json:"default_layers,omitempty" toml:"default_layers,omitempty"` //nolint:tagliatelle
	Default       string              `yaml:"default,omitempty" json:"default,omitempty" toml:"default,omitempty"`
	Profiles      map[string][]string `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
	Gitignore     *bool               `yaml:"gitignore,omitempty" json:"gitignore,omitempty" toml:"gitignore,omitempty"`
	// GitignoreExclude lists ignore patterns kept out of the managed .gitignore
	// block: generated outputs the project commits on purpose. Machine-local and
	// secret entries are never excluded.
	GitignoreExclude []string               `yaml:"gitignore_exclude,omitempty" json:"gitignore_exclude,omitempty" toml:"gitignore_exclude,omitempty"` //nolint:tagliatelle
	Includes         []IncludeConfig        `yaml:"includes,omitempty" json:"includes,omitempty" toml:"includes,omitempty"`
	InstalledSkills  []InstalledSkillConfig `yaml:"installed_skills,omitempty" json:"installed_skills,omitempty" toml:"installed_skills,omitempty"` //nolint:tagliatelle
	// InstalledMarketplaces and InstalledPlugins import the content of Claude
	// Code, Codex, Cursor and Gemini plugins, read through plugin marketplaces
	// or straight from a repository (see docs/installed-plugins.md).
	InstalledMarketplaces []InstalledMarketplaceConfig `yaml:"installed_marketplaces,omitempty" json:"installed_marketplaces,omitempty" toml:"installed_marketplaces,omitempty"` //nolint:tagliatelle
	InstalledPlugins      []InstalledPluginConfig      `yaml:"installed_plugins,omitempty" json:"installed_plugins,omitempty" toml:"installed_plugins,omitempty"`                //nolint:tagliatelle
	// EnabledPlugins switches on plugins from other marketplaces for the Claude
	// Code, Codex and OpenCode presets (see docs/plugins.md).
	EnabledPlugins []EnabledPlugin `yaml:"enabled_plugins,omitempty" json:"enabled_plugins,omitempty" toml:"enabled_plugins,omitempty"` //nolint:tagliatelle
	Header         *HeaderConfig   `yaml:"header,omitempty" json:"header,omitempty" toml:"header,omitempty"`
	Defaults       *DefaultsConfig `yaml:"defaults,omitempty" json:"defaults,omitempty" toml:"defaults,omitempty"`
	Builtins       *BuiltinsConfig `yaml:"builtins,omitempty" json:"builtins,omitempty" toml:"builtins,omitempty"`
	// SkipOutputs lists output kinds ai-rulez never writes, overwrites or deletes
	// ("instructions", "agents_md", "claude_md", "rules", "skills", "agents",
	// "commands"); see skip_outputs.go.
	SkipOutputs []string `yaml:"skip_outputs,omitempty" json:"skip_outputs,omitempty" toml:"skip_outputs,omitempty"` //nolint:tagliatelle
	Compact     *bool    `yaml:"compact,omitempty" json:"compact,omitempty" toml:"compact,omitempty"`
	AgentsMD    bool     `yaml:"agents_md,omitempty" json:"agents_md,omitempty" toml:"agents_md,omitempty"` //nolint:tagliatelle
	// Source selects where the content is read from: "ai-rulez" (default, the
	// .ai-rulez content tree) or "agents" (AGENTS.md and .agents/, see agents_source.go).
	Source string `yaml:"source,omitempty" json:"source,omitempty" toml:"source,omitempty"`
	// AgentsMDSplit makes the "agents" source read AGENTS.md as one context item
	// per H2 heading (named agents-<heading slug>) instead of one item.
	AgentsMDSplit bool `yaml:"agents_md_split,omitempty" json:"agents_md_split,omitempty" toml:"agents_md_split,omitempty"`
	// BundleExclude adds patterns to DefaultBundleExcludes: skill and command
	// resources matching one are not listed in SKILL.md or copied.
	BundleExclude []string `yaml:"bundle_exclude,omitempty" json:"bundle_exclude,omitempty" toml:"bundle_exclude,omitempty"` //nolint:tagliatelle
	// CodexSkillsDir is where the codex preset writes skills, relative to the
	// output base dir. Empty means the documented ".agents/skills"; set
	// ".codex/skills" to keep the pre-4.24 location.
	CodexSkillsDir string              `yaml:"codex_skills_dir,omitempty" json:"codex_skills_dir,omitempty" toml:"codex_skills_dir,omitempty"` //nolint:tagliatelle
	Plugins        []PluginConfig      `yaml:"plugins,omitempty" json:"plugins,omitempty" toml:"plugins,omitempty"`
	Marketplaces   []MarketplaceConfig `yaml:"marketplaces,omitempty" json:"marketplaces,omitempty" toml:"marketplaces,omitempty"`
	// PluginsAuthoring and MarketplacesAuthoring are the producer-side definitions
	// of several plugins and several marketplace indexes from one root. They are
	// distinct from the consumer-side Plugins and Marketplaces above.
	PluginsAuthoring      []MarketplacePlugin     `yaml:"plugins_authoring,omitempty" json:"plugins_authoring,omitempty" toml:"plugins_authoring,omitempty"`                //nolint:tagliatelle
	MarketplacesAuthoring []MarketplaceDefinition `yaml:"marketplaces_authoring,omitempty" json:"marketplaces_authoring,omitempty" toml:"marketplaces_authoring,omitempty"` //nolint:tagliatelle
	Scopes                []ScopeConfig           `yaml:"scopes,omitempty" json:"scopes,omitempty" toml:"scopes,omitempty"`
	MCP                   *MCPConfig              `yaml:"mcp,omitempty" json:"mcp,omitempty" toml:"mcp,omitempty"`
	Rules                 *RulesConfig            `yaml:"rules,omitempty" json:"rules,omitempty" toml:"rules,omitempty"`
	Lint                  *LintConfig             `yaml:"lint,omitempty" json:"lint,omitempty" toml:"lint,omitempty"`
	OKF                   *OKFConfig              `yaml:"okf,omitempty" json:"okf,omitempty" toml:"okf,omitempty"`
	LLMsTxt               *LLMsTxtConfig          `yaml:"llms_txt,omitempty" json:"llms_txt,omitempty" toml:"llms_txt,omitempty"` //nolint:tagliatelle
	Usage                 *UsageConfig            `yaml:"usage,omitempty" json:"usage,omitempty" toml:"usage,omitempty"`
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
	// Delivery is the [delivery] table; [delivery.stub] customizes the generated
	// dynamic-skills stub (delivery_stub.go).
	Delivery *DeliveryConfig `yaml:"delivery,omitempty" json:"delivery,omitempty" toml:"delivery,omitempty"`
	// stubBodyFileText is the text of [delivery.stub] body_file, read at load.
	stubBodyFileText string
	SkillSources     []SkillSourceConfig `yaml:"skill_sources,omitempty" json:"skill_sources,omitempty" toml:"skill_sources,omitempty"` //nolint:tagliatelle
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
	// Judge configures `ai-rulez judge` (categories, gating, attached conventions, rubric skills, prompts, suppressions); docs/judge.md.
	Judge *JudgeConfig `yaml:"judge,omitempty" json:"judge,omitempty" toml:"judge,omitempty"`
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
	// SessionStart configures the built-in SessionStart hook that regenerates outputs in a remote session.
	SessionStart *SessionStartConfig `yaml:"session_start,omitempty" json:"session_start,omitempty" toml:"session_start,omitempty"`
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
	// RootRel is where this project's base directory sits below the root of its
	// git work tree (forward slashes), empty for the work tree's own root and
	// outside a work tree. Generated headers use it to name a nested root's own
	// config by its path from the repository root.
	RootRel string `yaml:"-" json:"-" toml:"-"`
	// NestedRoot is set when another ai-rulez root encloses this project inside
	// the same git work tree; see HeaderConfig.Nested.
	NestedRoot bool `yaml:"-" json:"-" toml:"-"`

	// PolicyOutcome is what the organization policy clamped and reported at load
	// time (docs/policy.md); nil when no policy is in force.
	PolicyOutcome *PolicyOutcome `yaml:"-" json:"-" toml:"-"`

	// Run is the state of the generation in progress; nil outside one.
	Run        *RunState `yaml:"-" json:"-" toml:"-"`
	ConfigFile string    `yaml:"-" json:"-" toml:"-"` // Actual config filename (e.g. "config.toml")
	// Implicit marks the synthetic configuration of a project that has none (see
	// LoadImplicit): no presets, includes or config file, the content of the
	// .ai-rulez/ tree when it exists, and the project root as the authored root.
	// Only the read-only commands (validate, scan, doctor, okf validate) accept it.
	Implicit bool `yaml:"-" json:"-" toml:"-"`
	// agentsMDOptIn records an explicit agents_md = true in the main config file,
	// which the agents source rejects (see validateSource).
	agentsMDOptIn bool
	// ConfigFilePath is the absolute path of the main config file; it differs from
	// ConfigDir/ConfigFile for a configuration kept in .agents/ai-rulez.toml.
	ConfigFilePath string `yaml:"-" json:"-" toml:"-"`
	// AgentsLayout is set when the configuration was read from .agents/ai-rulez.toml,
	// which selects the "agents" source (ConfigDir is then .agents/.ai-rulez).
	AgentsLayout bool `yaml:"-" json:"-" toml:"-"`
	// contentless is set when the configuration was read from ai-rulez.toml at the
	// project root: there is no content tree (see Contentless).
	contentless bool
	Content     *ContentTree `yaml:"-" json:"-" toml:"-"`
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
	LocalOverlay *LocalOverlay `yaml:"-" json:"-" toml:"-"`
	// ActiveLayers names the config.<name>.toml layers merged into this
	// configuration, in merge order. A layered Config is a merged view and is never
	// written back as config.toml.
	ActiveLayers []string `yaml:"-" json:"-" toml:"-"`
	// LayerOrigins maps each active layer to how it was activated: "default"
	// (default_layers), "env" (AI_RULEZ_LAYERS or AI_RULEZ_PROFILE) or "option" (WithLayers).
	LayerOrigins map[string]string     `yaml:"-" json:"-" toml:"-"`
	MCPServers   map[string]*MCPServer `yaml:"-" json:"-" toml:"-"`
	// ImportedScan lists the hook commands, hook scripts and MCP definitions of
	// installed plugins that the import security scan reads besides the content
	// files. Runtime-only.
	ImportedScan []ImportedScanFile `yaml:"-" json:"-" toml:"-"`
	// PluginMCPServers names the entries of MCPServers installed plugins
	// delivered, so the organization policy can bound them. Runtime-only.
	PluginMCPServers []string    `yaml:"-" json:"-" toml:"-"`
	MCPServersRaw    []MCPServer `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty" toml:"mcp_servers,omitempty"`

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
	return c.Plugin != nil || c.HasPluginSet()
}

// IsPluginAuthoringRoot reports whether the root exists to build plugin
// bundles: it authors a plugin or marketplace (HasPluginAuthoring, the same
// test `generate --plugin` uses to decide it has something to build) and
// configures no preset. A root that keeps a preset also generates harness
// files for its own project, so a normal generate stays correct there.
func (c *Config) IsPluginAuthoringRoot() bool {
	return c != nil && len(c.Presets) == 0 && c.HasPluginAuthoring()
}

// LintOnly reports whether the configuration declares no preset and authors no
// plugin: ai-rulez is used only to validate, scan and run [[lint.external]], and
// generate has nothing to render.
func (c *Config) LintOnly() bool {
	return len(c.Presets) == 0 && !c.HasPluginAuthoring()
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
	// (`npx -y ai-rulez@latest mcp`) to the project .mcp.json. It defaults to
	// true in v5, so a project gets the server out of the box; set it to false
	// (or pass generate --no-self-mcp) to turn it off. The entry is merged into
	// the file, so hand-authored servers beside it survive, and no other MCP
	// output (.claude/settings.json, ...) is touched.
	SelfServer *bool `yaml:"self_server,omitempty" json:"self_server,omitempty" toml:"self_server,omitempty"`

	// SelfServerVersion pins the ai-rulez version the entry runs. Empty means
	// "latest", so a project picks up new releases without regenerating.
	SelfServerVersion string `yaml:"self_server_version,omitempty" json:"self_server_version,omitempty" toml:"self_server_version,omitempty"`

	// SelfServerCommand replaces the whole launch command (executable followed
	// by its arguments), for installs that do not run through npx, for example
	// ["ai-rulez", "mcp"]. SelfServerVersion is ignored when it is set.
	SelfServerCommand []string `yaml:"self_server_command,omitempty" json:"self_server_command,omitempty" toml:"self_server_command,omitempty"`

	// SelfServerLauncher chooses how the self entry starts ai-rulez: "npx" (the
	// default) runs it through npx, "binary-then-npx" runs an installed `ai-rulez`
	// when one is on PATH and falls back to npx. SelfServerVersion pins the npx
	// fallback. It cannot be combined with SelfServerCommand.
	SelfServerLauncher string `yaml:"self_server_launcher,omitempty" json:"self_server_launcher,omitempty" toml:"self_server_launcher,omitempty"`

	// SkillsInstructions, when false, leaves the "Unlisted skills" section out of
	// the instruction files. Nil means on: while skills are served, every
	// instruction file tells the agent to call find_skill and load_skill.
	SkillsInstructions *bool `yaml:"skills_instructions,omitempty" json:"skills_instructions,omitempty" toml:"skills_instructions,omitempty"`

	// SkillsInstructionsText replaces the default section. {{server}} is the
	// MCP server that serves the skills and {{skills_dir}} the config directory's
	// skills folder, relative to the instruction file.
	SkillsInstructionsText string `yaml:"skills_instructions_text,omitempty" json:"skills_instructions_text,omitempty" toml:"skills_instructions_text,omitempty"`

	// DiscoverSkills makes `mcp --serve-skills` also serve the skills already in
	// the repository: "off" (the default), "repo" (skill directories of the working
	// tree) or "all" (repo plus installed and included skills). The
	// --discover-skills flag overrides it.
	DiscoverSkills string `yaml:"discover_skills,omitempty" json:"discover_skills,omitempty" toml:"discover_skills,omitempty"`

	// IndexCache says where the repository index of the discovery keeps its
	// cache: "user" (the default; the user cache directory, with a gitignored
	// directory in the project as the fallback when it is not writable), "repo"
	// (that project directory) or "off". The cache holds summaries and scan
	// verdicts, never file bodies.
	IndexCache string `yaml:"index_cache,omitempty" json:"index_cache,omitempty" toml:"index_cache,omitempty"`

	// The keys below are the settings of `ai-rulez mcp` that a project can pin
	// instead of every launcher repeating a flag. A flag given on the command
	// line wins over the key. They are read when the server starts.

	// ServeSkills makes a bare `ai-rulez mcp` behave as `mcp --serve-skills`, so
	// a client entry needs no flag. The --serve-skills flag wins, including
	// --serve-skills=false.
	ServeSkills *bool `yaml:"serve_skills,omitempty" json:"serve_skills,omitempty" toml:"serve_skills,omitempty"`

	// ReadOnly registers only the read-only tools, as `mcp --read-only` does. An
	// organization policy's mcp.read_only = true cannot be loosened by it.
	ReadOnly *bool `yaml:"read_only,omitempty" json:"read_only,omitempty" toml:"read_only,omitempty"`

	// Daemon shares one long-lived skills server per repository, as
	// `mcp --serve-skills --daemon` does.
	Daemon *bool `yaml:"daemon,omitempty" json:"daemon,omitempty" toml:"daemon,omitempty"`

	// DaemonIdleTimeout is how long the shared daemon outlives its last client,
	// as a Go duration ("30s", "5m"); empty keeps the 15 minute default.
	DaemonIdleTimeout string `yaml:"daemon_idle_timeout,omitempty" json:"daemon_idle_timeout,omitempty" toml:"daemon_idle_timeout,omitempty"`

	// Authoring keeps the authoring tools on a read-only skills server. Without
	// it, `mcp --serve-skills` with read_only registers only the skill-serving
	// tools (find_skill, load_skill, search_skills, read_skill_file,
	// list_skill_resources) instead of the whole authoring set. A `tools`
	// list wins over both.
	Authoring *bool `yaml:"authoring,omitempty" json:"authoring,omitempty" toml:"authoring,omitempty"`

	// ServeUserInvokedSkills serves the skills that set `disable-model-invocation:
	// true` (Claude Code's user-only flag) to the model as well. Off by default:
	// find_skill, search_skills and the skill:// resources leave them out and
	// load_skill refuses them, as the Skill tool does.
	ServeUserInvokedSkills *bool `yaml:"serve_user_invoked_skills,omitempty" json:"serve_user_invoked_skills,omitempty" toml:"serve_user_invoked_skills,omitempty"`

	// NoWatch stops `mcp --serve-skills` reloading when skill files change, as
	// --no-watch does.
	NoWatch *bool `yaml:"no_watch,omitempty" json:"no_watch,omitempty" toml:"no_watch,omitempty"`

	// Allow and Deny are the --allow and --deny name globs of the served skills.
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty" toml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty" json:"deny,omitempty" toml:"deny,omitempty"`

	// Tools, when set, is the only list of tools the server registers (the
	// --tools flag), for example ["find_skill", "load_skill", "find_context",
	// "load_context"] for a server that can only read.
	Tools []string `yaml:"tools,omitempty" json:"tools,omitempty" toml:"tools,omitempty"`

	// Exclude lists globs, relative to the project root, of the paths repository
	// discovery (`mcp --serve-skills --discover-skills`) never reads: skill
	// directories and the context index alike. The --exclude flag adds to it.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
}

// HasSelfServer reports whether generation should add ai-rulez's own MCP server.
// It is on by default: only an explicit self_server = false (or --no-self-mcp)
// turns it off. A plugin-authoring root (IsPluginAuthoringRoot) never gets it:
// its .mcp.json belongs to the shipped bundle, which declares its own servers.
func (c *Config) HasSelfServer() bool {
	if c == nil || c.IsPluginAuthoringRoot() {
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
const (
	mcpKeyCommand = "command"
	mcpKeyType    = "type"
)

// serveSkillsArg is the flag that makes the self entry serve the project's
// skills as well as the authoring tools.
const serveSkillsArg = "--serve-skills"

// servesSkills reports whether generation will serve skills, so the self entry
// must run `mcp --serve-skills` (see SelfMCPServerEntry).
func (c *Config) servesSkills() bool {
	return c != nil && (c.ServesSkills(c.Content) || c.RolesServeSkills())
}

// Values of mcp.self_server_launcher.
const (
	SelfLauncherNPX           = "npx"
	SelfLauncherBinaryThenNPX = "binary-then-npx"
)

// binaryThenNPXScript starts an installed ai-rulez, else the pinned npx package.
// The arguments after the script name reach it as "$@" (the trailing "sh" fills $0).
const binaryThenNPXScript = `if command -v ai-rulez >/dev/null 2>&1; then exec ai-rulez mcp "$@"; else exec npx -y ai-rulez@%s mcp "$@"; fi`

// isShellScriptLaunch reports whether command is a shell run with -c, where an
// appended argument lands in $0 of the script instead of reaching ai-rulez.
func isShellScriptLaunch(command []string) bool {
	if len(command) < 3 {
		return false
	}
	switch filepath.Base(command[0]) {
	case "sh", "bash", "zsh", "dash", "ash":
		return command[1] == "-c"
	}
	return false
}

// withServeSkillsArg appends --serve-skills to a launch's arguments unless it is
// already there, so a hand-written command that already serves skills is left as
// it is. A shell script launch (`sh -c <script>`) is left alone as well: the
// flag would become $0 of the script, so the script must carry it itself.
func withServeSkillsArg(command, args []string) []string {
	if isShellScriptLaunch(command) {
		return args
	}
	for _, a := range args {
		if a == serveSkillsArg {
			return args
		}
	}
	return append(args, serveSkillsArg)
}

// SelfMCPServerEntry builds the .mcp.json entry for ai-rulez's own MCP server.
// It runs ai-rulez@latest by default, so a project picks up new releases without
// regenerating; pin a version with mcp.self_server_version or replace the launch
// with mcp.self_server_command. An empty or "dev" version resolves to "latest".
//
// When the project serves skills the entry runs `mcp --serve-skills`, the merged
// server that serves the skills and the authoring tools from one process, so a
// project needs one MCP entry rather than the self entry plus a separate skills
// server.
func (c *Config) SelfMCPServerEntry() map[string]any {
	if c.MCP != nil && len(c.MCP.SelfServerCommand) > 0 {
		args := append([]string(nil), c.MCP.SelfServerCommand[1:]...)
		if c.servesSkills() {
			args = withServeSkillsArg(c.MCP.SelfServerCommand, args)
		}
		entry := map[string]any{mcpKeyType: TransportStdio, mcpKeyCommand: c.MCP.SelfServerCommand[0]}
		if len(args) > 0 {
			entry["args"] = args
		}
		return entry
	}
	version := ""
	if c.MCP != nil {
		version = c.MCP.SelfServerVersion
	}
	version = resolveLauncherVersion(version, "")
	if c.MCP != nil && c.MCP.SelfServerLauncher == SelfLauncherBinaryThenNPX {
		args := []string{"-c", fmt.Sprintf(binaryThenNPXScript, version), "sh"}
		if c.servesSkills() {
			args = append(args, serveSkillsArg)
		}
		return map[string]any{mcpKeyType: "stdio", mcpKeyCommand: "sh", "args": args}
	}
	args := []string{"-y", "ai-rulez@" + version, "mcp"}
	if c.servesSkills() {
		args = append(args, serveSkillsArg)
	}
	return map[string]any{
		mcpKeyType:    "stdio",
		mcpKeyCommand: SelfLauncherNPX,
		"args":        args,
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
	// MCPLine, when false, leaves the "Use MCP server: ..." line out of every
	// generated header. Nil means on: the line is derived from [mcp] and is
	// omitted anyway while the self server is off.
	MCPLine *bool `yaml:"mcp_line,omitempty" json:"mcp_line,omitempty" toml:"mcp_line,omitempty"`
	// Nested sets the header of a root that sits inside another ai-rulez root:
	// "compact" (the default) writes one line naming the config, "omit" writes
	// no header, "inherit" writes the same header as the outer root. A custom
	// Text is kept unless Nested is set explicitly.
	Nested string `yaml:"nested,omitempty" json:"nested,omitempty" toml:"nested,omitempty"`
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

	// BazDefault selects which items reach the baz preset's path-scoped
	// AGENTS.md files: "all" (the default) lets every item through, "none"
	// only items whose frontmatter `targets` name baz.
	BazDefault string `yaml:"baz_default,omitempty" json:"baz_default,omitempty" toml:"baz_default,omitempty"` //nolint:tagliatelle

	// BazUnscoped says what the baz preset does with a glob-activated item that
	// cannot be nested because no glob names a directory (`**/*.go`, `**/BUILD`):
	// "root" (the default) inlines it into the root AGENTS.md, "skip" leaves it
	// out of that file when baz is its only writer.
	BazUnscoped string `yaml:"baz_unscoped,omitempty" json:"baz_unscoped,omitempty" toml:"baz_unscoped,omitempty"` //nolint:tagliatelle

	// BazRest says what the baz preset does with a glob-activated item it does not
	// nest (not selected under baz_default = "none", or excluded by `!baz`):
	// "root" (the default) leaves it inlined in the root AGENTS.md, "skip" leaves
	// it out of that file when baz is its only writer.
	BazRest string `yaml:"baz_rest,omitempty" json:"baz_rest,omitempty" toml:"baz_rest,omitempty"` //nolint:tagliatelle

	// BazDedupe, when true, writes a path-scoped item once into a shared parent
	// AGENTS.md when its globs point into several directories under one parent,
	// instead of a copy in each. Off by default so existing output is unchanged.
	BazDedupe bool `yaml:"baz_dedupe,omitempty" json:"baz_dedupe,omitempty" toml:"baz_dedupe,omitempty"` //nolint:tagliatelle

	// BazDedupeMax, when above 0, collapses an item that would be copied into
	// more than that many directories to the directory covering all of them, up
	// to the repository root (the root AGENTS.md). 0 (the default) is off.
	BazDedupeMax int `yaml:"baz_dedupe_max,omitempty" json:"baz_dedupe_max,omitempty" toml:"baz_dedupe_max,omitempty"` //nolint:tagliatelle

	// BazBudget is the byte budget of the AGENTS.md files the baz preset
	// writes; generate warns when their total exceeds it. 0 (the default)
	// disables the warning.
	BazBudget int `yaml:"baz_budget,omitempty" json:"baz_budget,omitempty" toml:"baz_budget,omitempty"` //nolint:tagliatelle

	// ScopedFallback says what the shared AGENTS.md does with glob-scoped rules
	// and context for the presets that read it without a rules folder of their
	// own (codex, opencode, pi, ...): "inline" (the default) inlines them with an
	// "Applies to" line, which every reader of AGENTS.md then loads at start;
	// "skill" writes each one as a skill gated by its paths in .agents/skills
	// instead, so they load on demand. See ScopedFallbackRules.
	ScopedFallback string `yaml:"scoped_fallback,omitempty" json:"scoped_fallback,omitempty" toml:"scoped_fallback,omitempty"` //nolint:tagliatelle

	// ScopedFallbackGroup folds the skills scoped_fallback = "skill" writes into
	// fewer ones, so the skill listing a harness budgets stays short: "none" (the
	// default) writes one skill per item, "include" one per include or builtin
	// pack, "directory" one per directory the item's globs point into. See
	// ScopedFallbackGrouping.
	ScopedFallbackGroup string `yaml:"scoped_fallback_group,omitempty" json:"scoped_fallback_group,omitempty" toml:"scoped_fallback_group,omitempty"` //nolint:tagliatelle

	// Baz holds per-rule controls for the baz preset ([rules.baz]).
	Baz *BazRulesConfig `yaml:"baz,omitempty" json:"baz,omitempty" toml:"baz,omitempty"`
}

// BazRulesConfig is the [rules.baz] table.
type BazRulesConfig struct {
	// Include names the rules and context files the baz preset selects for its
	// path-scoped AGENTS.md files when rules.baz_default is "none". The same
	// opt-in is `baz: true` in an item's frontmatter.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
}

// BazIncludes reports whether rules.baz.include names the item.
func (c *Config) BazIncludes(name string) bool {
	if c == nil || c.Rules == nil || c.Rules.Baz == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSuffix(name, ".md"))
	for _, entry := range c.Rules.Baz.Include {
		if strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry), ".md")) == name {
			return true
		}
	}
	return false
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

// Values of rules.scoped_fallback.
const (
	ScopedFallbackInline = "inline"
	ScopedFallbackSkill  = "skill"
)

// Values of rules.scoped_fallback_group.
const (
	ScopedFallbackGroupNone      = "none"
	ScopedFallbackGroupInclude   = "include"
	ScopedFallbackGroupDirectory = "directory"
)

// ScopedFallbackGrouping returns how scoped_fallback = "skill" groups its
// skills: ScopedFallbackGroupNone unless rules.scoped_fallback_group says
// otherwise.
func (c *Config) ScopedFallbackGrouping() string {
	if c != nil && c.Rules != nil && c.Rules.ScopedFallbackGroup != "" {
		return c.Rules.ScopedFallbackGroup
	}
	return ScopedFallbackGroupNone
}

// ScopedFallbackRules returns how the shared AGENTS.md serves glob-scoped items
// to presets without a rules folder: ScopedFallbackInline unless
// rules.scoped_fallback says ScopedFallbackSkill.
func (c *Config) ScopedFallbackRules() string {
	if c != nil && c.Rules != nil && c.Rules.ScopedFallback != "" {
		return c.Rules.ScopedFallback
	}
	return ScopedFallbackInline
}

// Values of rules.baz_unscoped.
const (
	BazUnscopedRoot = "root"
	BazUnscopedSkip = "skip"
)

// BazUnscopedRules returns what the baz preset does with a path-scoped item that
// cannot be nested: BazUnscopedRoot unless rules.baz_unscoped says BazUnscopedSkip.
func (c *Config) BazUnscopedRules() string {
	if c != nil && c.Rules != nil && c.Rules.BazUnscoped != "" {
		return c.Rules.BazUnscoped
	}
	return BazUnscopedRoot
}

// Values of rules.baz_rest.
const (
	BazRestRoot = "root"
	BazRestSkip = "skip"
)

// BazRestRules returns what the baz preset does with a path-scoped item it does
// not nest: BazRestRoot unless rules.baz_rest says BazRestSkip.
func (c *Config) BazRestRules() string {
	if c != nil && c.Rules != nil && c.Rules.BazRest != "" {
		return c.Rules.BazRest
	}
	return BazRestRoot
}

// Values of rules.baz_default.
const (
	BazDefaultAll  = "all"
	BazDefaultNone = "none"
)

// BazDefaultRules returns which items the baz preset selects for its
// path-scoped AGENTS.md files: BazDefaultAll (every item) unless rules.baz_default
// says BazDefaultNone (only items whose targets name baz).
func (c *Config) BazDefaultRules() string {
	if c != nil && c.Rules != nil && c.Rules.BazDefault != "" {
		return c.Rules.BazDefault
	}
	return BazDefaultAll
}

// BazDedupeEnabled reports whether a path-scoped item whose globs point into
// several directories under one parent is written once to the shared parent
// AGENTS.md (rules.baz_dedupe). Off by default.
func (c *Config) BazDedupeEnabled() bool {
	return c != nil && c.Rules != nil && c.Rules.BazDedupe
}

// BazDedupeMaxCopies returns rules.baz_dedupe_max, the most nested copies an item
// may produce before it collapses to a covering directory, or 0 when off.
func (c *Config) BazDedupeMaxCopies() int {
	if c == nil || c.Rules == nil {
		return 0
	}
	return c.Rules.BazDedupeMax
}

// BazBudgetBytes returns the byte budget of the AGENTS.md files the baz preset
// writes (rules.baz_budget), or 0 when the warning is off.
func (c *Config) BazBudgetBytes() int {
	if c == nil || c.Rules == nil {
		return 0
	}
	return c.Rules.BazBudget
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
	// Type is the explicit transport written to a harness's `type` key
	// (stdio, http or sse). It is an alias of Transport: when both are set they
	// must agree, and an unset Type falls back to Transport.
	Type string `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"`
	URL  string `yaml:"url,omitempty" json:"url,omitempty" toml:"url,omitempty"`
	// OAuth is the pre-registered OAuth client of a hosted (http/sse) server,
	// rendered into the harnesses whose MCP format has an `oauth` block. A
	// harness that cannot express it skips the server with a warning rather
	// than writing one that could not authenticate.
	OAuth *MCPOAuth `yaml:"oauth,omitempty" json:"oauth,omitempty" toml:"oauth,omitempty"`
	// Headers are HTTP headers sent to a remote (http/sse) server, typically for
	// auth. Values may contain ${VAR} placeholders, resolved like Env.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty" toml:"headers,omitempty"`
	Enabled *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
	// Profiles restricts this server to the named profiles. Empty means every
	// profile, preserving the pre-existing behavior.
	Profiles []string `yaml:"profiles,omitempty" json:"profiles,omitempty" toml:"profiles,omitempty"`
	// Domain restricts this server to the named domain: it is active only when
	// that domain is selected by the active profile or role. Empty means global,
	// preserving the pre-existing behavior. When both Domain and Profiles are
	// set, both must pass.
	Domain string `yaml:"domain,omitempty" json:"domain,omitempty" toml:"domain,omitempty"`
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

// MCPOAuth is the pre-registered OAuth client a hosted (http or sse) MCP server
// authenticates with: the client ID registered with the provider, the local
// callback port it was registered for, and the scopes to request. A harness
// that reads an `oauth` block renders the fields it understands.
type MCPOAuth struct {
	ClientID     string   `yaml:"client_id,omitempty" json:"client_id,omitempty" toml:"client_id,omitempty"`             //nolint:tagliatelle
	CallbackPort int      `yaml:"callback_port,omitempty" json:"callback_port,omitempty" toml:"callback_port,omitempty"` //nolint:tagliatelle
	Scopes       []string `yaml:"scopes,omitempty" json:"scopes,omitempty" toml:"scopes,omitempty"`
}

// IsZero reports whether the block carries no OAuth configuration at all.
func (o *MCPOAuth) IsZero() bool {
	return o == nil || (o.ClientID == "" && o.CallbackPort == 0 && len(o.Scopes) == 0)
}

// IsEnabled returns true if the MCP server is enabled (defaults to true if not specified)
func (m *MCPServer) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// GetTransport returns the transport protocol, defaulting to "stdio". An
// explicit Type wins over Transport so a config may spell the harness's `type`
// key directly.
func (m *MCPServer) GetTransport() string {
	if m == nil {
		return TransportStdio
	}
	if m.Type != "" {
		return m.Type
	}
	if m.Transport != "" {
		return m.Transport
	}
	return TransportStdio
}

// HasOAuth reports whether the server declares a pre-registered OAuth client.
func (m *MCPServer) HasOAuth() bool {
	return m != nil && !m.OAuth.IsZero()
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
// the order they are first named. A "*" entry expands to every domain the config
// knows and "!name" entries are removed; a profile made only of exclusions has
// no domains (Validate reports it as an error).
func (c *Config) GetProfileDomains(profile string) []string {
	resolved, err := ResolveProfileDomains(c.profileDomainEntries(profile), knownDomainNames(c.Content))
	if err != nil {
		return nil
	}
	return resolved
}

// profileDomainEntries returns a profile's raw domain entries: the entries of a
// single named profile, or the de-duplicated union of a composed value's
// elements, in the order they are first named. Wildcards and exclusions are left
// as written for ResolveProfileDomains to interpret.
func (c *Config) profileDomainEntries(profile string) []string {
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
	// Domain places this installed skill in the named domain, so profile and role
	// selection include it exactly like a local domain skill. Empty keeps it in
	// the root content. When both Domain and Profiles are set, both must pass.
	Domain string `yaml:"domain,omitempty" json:"domain,omitempty" toml:"domain,omitempty"`
	// Include keeps only skills whose directory name matches one of these globs,
	// so one entry can select several skills from one repository. Empty selects
	// every skill below the entry's path.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	// Exclude drops skills whose directory name matches one of these globs; it
	// wins over Include.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// Resources narrows or widens what is bundled from each skill's directory. An
	// installed skill is bundled whole by default (everything but SKILL.md, evals/,
	// agents/ and hidden files), so a skill with rules/ or a cli.md beside SKILL.md
	// arrives complete. references/, scripts/ and assets/ are always bundled;
	// setting Resources replaces the default for everything else. A pattern is a
	// name, a path or a glob; a name matches that file or directory at any depth,
	// and a "!" prefix removes what an earlier pattern added. ["none"] (a name no
	// file has) keeps the canonical directories only; ["*"] also bundles evals/,
	// agents/ and hidden files.
	Resources []string `yaml:"resources,omitempty" json:"resources,omitempty" toml:"resources,omitempty"`
	// NamePrefix is prepended to every resolved skill name and to its output
	// directory, so two entries cannot collide.
	NamePrefix string `yaml:"name_prefix,omitempty" json:"name_prefix,omitempty" toml:"name_prefix,omitempty"` //nolint:tagliatelle
	// Trust is the repository-scoped scan level of the imported skills: "error"
	// (every finding counts, the default) or "warn" (only error-severity
	// findings block). See TrustError and TrustWarn.
	Trust string `yaml:"trust,omitempty" json:"trust,omitempty" toml:"trust,omitempty"`
	// Version, TagPrefix and IncludePrerelease are as on IncludeConfig.
	Version           string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	TagPrefix         string `yaml:"tag_prefix,omitempty" json:"tag_prefix,omitempty" toml:"tag_prefix,omitempty"`                         //nolint:tagliatelle
	IncludePrerelease bool   `yaml:"include_prerelease,omitempty" json:"include_prerelease,omitempty" toml:"include_prerelease,omitempty"` //nolint:tagliatelle
	MinReleaseAge     string `yaml:"min_release_age,omitempty" json:"min_release_age,omitempty" toml:"min_release_age,omitempty"`          //nolint:tagliatelle
}

// GetPath returns the path within the repo that holds the entry's skills. An
// entry that selects several skills with include/exclude and names no path is a
// directory of skill directories: it defaults to the conventional "skills"
// directory, so `name` is only the entry's label. A single-skill entry keeps its
// "skills/<name>" default.
func (s *InstalledSkillConfig) GetPath() string {
	if s.Path != "" {
		return s.Path
	}
	if s.SelectsSeveral() {
		return skillsDir
	}
	return skillsDir + "/" + s.Name
}

// SelectsSeveral reports whether the entry picks skills by include/exclude
// rather than naming one skill directory with `name`.
func (s *InstalledSkillConfig) SelectsSeveral() bool {
	return len(s.Include) > 0 || len(s.Exclude) > 0
}
