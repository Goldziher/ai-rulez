package config

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

// This file defines the *authoring* (producer) side of plugins: describing a
// distributable plugin bundle that ai-rulez packages from the project's content
// tree for the Claude/Cursor/Codex/Gemini/Kimi/OpenCode/Factory/Hermes runtimes.
//
// It is intentionally distinct from PluginConfig / MarketplaceConfig in
// types_v4.go, which are the *consumer* side ("install these plugins from a
// marketplace" -> .claude/plugins.json). Do not conflate the two.

// PluginRuntime names a supported target runtime for authored plugin bundles.
const (
	PluginRuntimeClaude       = "claude"
	PluginRuntimeCursor       = "cursor"
	PluginRuntimeCodex        = "codex"
	PluginRuntimeGemini       = "gemini"
	PluginRuntimeKimi         = "kimi"
	PluginRuntimeOpenCode     = "opencode"
	PluginRuntimeFactory      = "factory"
	PluginRuntimeHermes       = "hermes"
	PluginRuntimeAgentPlugins = "agent-plugins"
	PluginRuntimeCopilot      = "copilot"
)

// AllPluginRuntimes lists the runtimes emitted by default when a plugin does not
// restrict Runtimes, in a stable order. PluginRuntimeAgentPlugins and
// PluginRuntimeCopilot are deliberately excluded: they are opt-in via an explicit
// runtimes = ["agent-plugins"] / ["copilot"] so adding them never changes
// existing bundles' output.
var AllPluginRuntimes = []string{
	PluginRuntimeClaude,
	PluginRuntimeCursor,
	PluginRuntimeCodex,
	PluginRuntimeGemini,
	PluginRuntimeKimi,
	PluginRuntimeOpenCode,
	PluginRuntimeFactory,
	PluginRuntimeHermes,
}

// KnownPluginRuntimes lists every runtime the generator can emit, including
// opt-in ones. Used for validation and schema documentation.
var KnownPluginRuntimes = append(append([]string{}, AllPluginRuntimes...), PluginRuntimeAgentPlugins, PluginRuntimeCopilot)

// Author identifies a person or organization in plugin/marketplace metadata.
type Author struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	Email string `yaml:"email,omitempty" json:"email,omitempty" toml:"email,omitempty"`
	URL   string `yaml:"url,omitempty" json:"url,omitempty" toml:"url,omitempty"`
}

// PluginAuthoring describes a distributable plugin bundle authored from this
// project. Skills, commands, and agents are NOT declared here: they come from
// the existing ContentTree. This block only carries packaging metadata.
type PluginAuthoring struct {
	Name        string   `yaml:"name" json:"name" toml:"name"`
	DisplayName string   `yaml:"display_name,omitempty" json:"display_name,omitempty" toml:"display_name,omitempty"` //nolint:tagliatelle
	Description string   `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Version     string   `yaml:"version" json:"version" toml:"version"`
	Author      *Author  `yaml:"author,omitempty" json:"author,omitempty" toml:"author,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty" json:"homepage,omitempty" toml:"homepage,omitempty"`
	Repository  string   `yaml:"repository,omitempty" json:"repository,omitempty" toml:"repository,omitempty"`
	License     string   `yaml:"license,omitempty" json:"license,omitempty" toml:"license,omitempty"`
	Category    string   `yaml:"category,omitempty" json:"category,omitempty" toml:"category,omitempty"`
	BrandColor  string   `yaml:"brand_color,omitempty" json:"brand_color,omitempty" toml:"brand_color,omitempty"` //nolint:tagliatelle
	Icon        string   `yaml:"icon,omitempty" json:"icon,omitempty" toml:"icon,omitempty"`
	Logo        string   `yaml:"logo,omitempty" json:"logo,omitempty" toml:"logo,omitempty"`
	Keywords    []string `yaml:"keywords,omitempty" json:"keywords,omitempty" toml:"keywords,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty" toml:"tags,omitempty"`
	// ContentRoot optionally points at a project-relative directory containing
	// plugin-only skills/, commands/, and agents/. Empty uses governance content.
	ContentRoot string `yaml:"content_root,omitempty" json:"content_root,omitempty" toml:"content_root,omitempty"` //nolint:tagliatelle

	// IncludeDomains lists the domains (names or globs) whose skills, commands
	// and agents are bundled next to the root content. Empty keeps the bundle
	// root-only. On a name collision the root item wins.
	IncludeDomains []string `yaml:"include_domains,omitempty" json:"include_domains,omitempty" toml:"include_domains,omitempty"` //nolint:tagliatelle

	// IncludeEvals bundles eval cases into the plugin: each skill's evals/
	// directory and the project-level .ai-rulez/evals/ tree (the latter at
	// <bundle>/evals/). Off by default, so a skill's evals/ directory is not
	// shipped to consumers of the bundle.
	IncludeEvals bool `yaml:"include_evals,omitempty" json:"include_evals,omitempty" toml:"include_evals,omitempty"` //nolint:tagliatelle

	// Runtimes restricts which runtime manifests are emitted. Empty means all
	// of AllPluginRuntimes.
	Runtimes []string `yaml:"runtimes,omitempty" json:"runtimes,omitempty" toml:"runtimes,omitempty"`

	// MCP declares the plugin's bundled MCP servers using the canonical
	// ${PLUGIN_ROOT} launch variable; each runtime renderer rewrites it to the
	// runtime-specific root variable. When empty, the project's [[mcp_servers]]
	// are used as the source.
	MCP []PluginMCPLaunch `yaml:"mcp,omitempty" json:"mcp,omitempty" toml:"mcp,omitempty"`

	// Hooks declares lifecycle hooks emitted as hooks.json (Claude/Cursor) or
	// inline hooks{} (Gemini). A hook action either points at a command that
	// already exists in the consumer's environment, or declares a project-local
	// 'script' that is bundled into the plugin's hooks/ directory.
	Hooks []HookGroup `yaml:"hooks,omitempty" json:"hooks,omitempty" toml:"hooks,omitempty"`

	// Statusline is a Claude-only capability: a bundled status-line script plus
	// the slash command that wires it into user settings.
	Statusline *Statusline `yaml:"statusline,omitempty" json:"statusline,omitempty" toml:"statusline,omitempty"`

	// Interface carries the rich UI block used by Codex and Kimi manifests.
	Interface *PluginInterface `yaml:"interface,omitempty" json:"interface,omitempty" toml:"interface,omitempty"`

	// Codex selects the Codex manifest layout and marketplace index.
	Codex *CodexExtras `yaml:"codex,omitempty" json:"codex,omitempty" toml:"codex,omitempty"`

	// Cursor holds Cursor-plugin-specific packaging switches.
	Cursor *CursorExtras `yaml:"cursor,omitempty" json:"cursor,omitempty" toml:"cursor,omitempty"`

	// Gemini holds Gemini-extension-specific fields.
	Gemini *GeminiExtras `yaml:"gemini,omitempty" json:"gemini,omitempty" toml:"gemini,omitempty"`

	// Kimi holds Kimi-plugin-specific fields.
	Kimi *KimiExtras `yaml:"kimi,omitempty" json:"kimi,omitempty" toml:"kimi,omitempty"`

	// Hermes holds Hermes-Agent-specific packaging fields.
	Hermes *HermesExtras `yaml:"hermes,omitempty" json:"hermes,omitempty" toml:"hermes,omitempty"`
}

// PluginMCPLaunch is a bundled MCP server launch declaration for a plugin. For
// stdio servers the command may reference ${PLUGIN_ROOT}; renderers rewrite it
// per runtime. For remote servers, set Transport ("http"/"sse") and URL instead.
type PluginMCPLaunch struct {
	Name      string            `yaml:"name" json:"name" toml:"name"`
	Command   string            `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty" json:"args,omitempty" toml:"args,omitempty"`
	Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty" toml:"env,omitempty"`
	Transport string            `yaml:"transport,omitempty" json:"transport,omitempty" toml:"transport,omitempty"`
	URL       string            `yaml:"url,omitempty" json:"url,omitempty" toml:"url,omitempty"`

	// Disabled carries a project server's enabled = false into the bundle. It is
	// not authored: [[plugin.mcp]] entries are always enabled.
	Disabled bool `yaml:"-" json:"-" toml:"-"`
}

// HookTypeCommand is the default (and only bundled) hook handler type: the
// runtime spawns Command, optionally with Args.
const HookTypeCommand = "command"

// KnownHookEvents lists every lifecycle event Claude Code documents. It exists so
// a typo in an authored event name ("SesionStart") is reported instead of silently
// producing a hook that never fires. Membership is advisory only — an event
// outside this list is warned about, never rejected, so a config written against a
// newer Claude Code keeps working on an older ai-rulez.
var KnownHookEvents = []string{
	"SessionStart",
	"Setup",
	"UserPromptSubmit",
	"UserPromptExpansion",
	"PreToolUse",
	"PermissionRequest",
	"PermissionDenied",
	"PostToolUse",
	"PostToolUseFailure",
	"PostToolBatch",
	"Notification",
	"MessageDisplay",
	"SubagentStart",
	"SubagentStop",
	"TaskCreated",
	"TaskCompleted",
	"Stop",
	"StopFailure",
	"TeammateIdle",
	"InstructionsLoaded",
	"ConfigChange",
	"CwdChanged",
	"DirectoryAdded",
	"FileChanged",
	"WorktreeCreate",
	"WorktreeRemove",
	"PreCompact",
	"PostCompact",
	"PreModelSwitch",
	"PostModelSwitch",
	"Elicitation",
	"ElicitationResult",
	"SessionEnd",
}

// HookEventsWithoutMatcher lists the events that carry no matchable subject, so a
// declared matcher is silently ignored at runtime. Authors get a warning rather
// than a hook that appears filtered but is not.
var HookEventsWithoutMatcher = []string{
	"UserPromptSubmit",
	"PostToolBatch",
	"Stop",
	"TeammateIdle",
	"TaskCreated",
	"TaskCompleted",
	"WorktreeCreate",
	"WorktreeRemove",
	"MessageDisplay",
}

// HookEventsEvaluatingIf lists the events on which Claude Code evaluates a
// handler's `if` rule. The field is tool-scoped, so it only means anything where a
// tool call is the subject of the event; on every other event a handler carrying
// `if` never fires at all. Declaring `if` on, say, SessionStart therefore disables
// the hook rather than conditioning it, which is worth a warning.
var HookEventsEvaluatingIf = []string{
	"PreToolUse",
	"PostToolUse",
	"PostToolUseFailure",
	"PermissionRequest",
	"PermissionDenied",
}

// HookGroup is one lifecycle-event hook group. Matcher filters which occurrences
// of Event run the group (for SessionStart: startup, resume, clear, compact,
// fork); it is ignored for the events in HookEventsWithoutMatcher.
//
// Targets and Matchers apply to the top-level [[hooks]] only, which render for
// several harnesses whose event and tool vocabularies differ; a plugin hook
// group is rendered per runtime and rejects them.
type HookGroup struct {
	Event   string       `yaml:"event" json:"event" toml:"event"` // e.g. SessionStart, PreToolUse
	Matcher string       `yaml:"matcher,omitempty" json:"matcher,omitempty" toml:"matcher,omitempty"`
	Hooks   []HookAction `yaml:"hooks,omitempty" json:"hooks,omitempty" toml:"hooks,omitempty"`
	// Targets restricts a top-level [[hooks]] group to the listed harnesses
	// (claude, codex, cursor, gemini, copilot). Empty means every harness the
	// group can be expressed for.
	Targets []string `yaml:"targets,omitempty" json:"targets,omitempty" toml:"targets,omitempty"`
	// Matchers overrides Matcher per harness, for harnesses that name tools
	// differently from Claude Code (gemini matches `write_file|replace` where
	// Claude matches `Write|Edit`).
	Matchers map[string]string `yaml:"matchers,omitempty" json:"matchers,omitempty" toml:"matchers,omitempty"`
	// Builtin names the feature that synthesized this group (HookBuiltinGuard). It
	// is never read from or written to config.toml.
	Builtin string `yaml:"-" json:"-" toml:"-"`
}

// HookAction is one action within a HookGroup. Exactly one of Command or Script
// is required: Command is passed through verbatim and must already resolve in the
// consumer's environment, while Script is a project-relative file that ai-rulez
// bundles into the plugin's hooks/ directory and rewrites into a
// ${PLUGIN_ROOT}-rooted command. Script is what makes a hook self-contained —
// a bootstrap hook cannot rely on a path that only exists after generation has
// already run (generated outputs are gitignored, so a fresh clone or a new git
// worktree has none of them).
type HookAction struct {
	Type    string `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"` // defaults to HookTypeCommand
	Command string `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	// Script is a project-relative path to a script bundled at hooks/<basename>.
	Script string `yaml:"script,omitempty" json:"script,omitempty" toml:"script,omitempty"`
	// Args are passed to the command as argv. When set, the runtime resolves the
	// command as an executable and spawns it directly, with no shell.
	Args []string `yaml:"args,omitempty" json:"args,omitempty" toml:"args,omitempty"`
	// Timeout is the handler's timeout in seconds; zero leaves the runtime default.
	Timeout int  `yaml:"timeout,omitempty" json:"timeout,omitempty" toml:"timeout,omitempty"`
	Async   bool `yaml:"async,omitempty" json:"async,omitempty" toml:"async,omitempty"`
	// If restricts the handler to matching tool calls, in permission-rule syntax
	// ("Bash(git *)", "Edit(*.ts)") — exactly one rule, with no boolean operators
	// and no expression language. Claude Code evaluates it only on the events in
	// HookEventsEvaluatingIf; on any other event a handler carrying If never runs,
	// so it cannot guard a bootstrap hook. A bootstrap script has to decide for
	// itself whether its work is already done.
	If string `yaml:"if,omitempty" json:"if,omitempty" toml:"if,omitempty"`
	// StatusMessage is shown to the user while the handler runs, which matters for
	// a bootstrap script that blocks the first turn.
	StatusMessage string `yaml:"status_message,omitempty" json:"status_message,omitempty" toml:"status_message,omitempty"` //nolint:tagliatelle
}

// Statusline declares a Claude-only status-line script passthrough.
type Statusline struct {
	Script  string `yaml:"script" json:"script" toml:"script"`                                  // path to the (hand-authored) statusline script
	Command string `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"` // enabling slash-command name
}

// PluginInterface is the rich UI block shared by Codex and Kimi manifests.
type PluginInterface struct {
	DisplayName       string   `yaml:"display_name,omitempty" json:"display_name,omitempty" toml:"display_name,omitempty"`                //nolint:tagliatelle
	ShortDescription  string   `yaml:"short_description,omitempty" json:"short_description,omitempty" toml:"short_description,omitempty"` //nolint:tagliatelle
	LongDescription   string   `yaml:"long_description,omitempty" json:"long_description,omitempty" toml:"long_description,omitempty"`    //nolint:tagliatelle
	DeveloperName     string   `yaml:"developer_name,omitempty" json:"developer_name,omitempty" toml:"developer_name,omitempty"`          //nolint:tagliatelle
	Category          string   `yaml:"category,omitempty" json:"category,omitempty" toml:"category,omitempty"`
	Capabilities      []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty" toml:"capabilities,omitempty"`
	DefaultPrompt     []string `yaml:"default_prompt,omitempty" json:"default_prompt,omitempty" toml:"default_prompt,omitempty"`                   //nolint:tagliatelle
	WebsiteURL        string   `yaml:"website_url,omitempty" json:"website_url,omitempty" toml:"website_url,omitempty"`                            //nolint:tagliatelle
	PrivacyPolicyURL  string   `yaml:"privacy_policy_url,omitempty" json:"privacy_policy_url,omitempty" toml:"privacy_policy_url,omitempty"`       //nolint:tagliatelle
	TermsOfServiceURL string   `yaml:"terms_of_service_url,omitempty" json:"terms_of_service_url,omitempty" toml:"terms_of_service_url,omitempty"` //nolint:tagliatelle
	BrandColor        string   `yaml:"brand_color,omitempty" json:"brand_color,omitempty" toml:"brand_color,omitempty"`                            //nolint:tagliatelle
	ComposerIcon      string   `yaml:"composer_icon,omitempty" json:"composer_icon,omitempty" toml:"composer_icon,omitempty"`                      //nolint:tagliatelle
	Logo              string   `yaml:"logo,omitempty" json:"logo,omitempty" toml:"logo,omitempty"`
	LogoDark          string   `yaml:"logo_dark,omitempty" json:"logo_dark,omitempty" toml:"logo_dark,omitempty"` //nolint:tagliatelle
	Screenshots       []string `yaml:"screenshots,omitempty" json:"screenshots,omitempty" toml:"screenshots,omitempty"`
}

// Codex manifest layouts for [plugin.codex] manifest.
const (
	// CodexManifestLegacy writes .codex-plugin/plugin.json only (the default).
	CodexManifestLegacy = "legacy"
	// CodexManifestRoot writes the Agent Plugins root plugin.json that Codex
	// documents as the preferred format, with the Codex interface block under
	// extensions.com.openai, skills/ and a root mcp.json.
	CodexManifestRoot = "root"
	// CodexManifestBoth writes the root manifest and the legacy one.
	CodexManifestBoth = "both"
)

// CodexExtras selects how the Codex runtime is laid out.
type CodexExtras struct {
	// Manifest is legacy (default), root or both; see the CodexManifest constants.
	Manifest string `yaml:"manifest,omitempty" json:"manifest,omitempty" toml:"manifest,omitempty"`
	// Marketplace also writes the Codex marketplace index
	// (.agents/plugins/marketplace.json) for a single-plugin repository.
	Marketplace bool `yaml:"marketplace,omitempty" json:"marketplace,omitempty" toml:"marketplace,omitempty"`
}

// ManifestLayout returns the resolved Codex manifest layout.
func (c *CodexExtras) ManifestLayout() string {
	if c == nil || c.Manifest == "" {
		return CodexManifestLegacy
	}
	return c.Manifest
}

// CursorExtras holds Cursor-plugin-specific switches.
type CursorExtras struct {
	// Marketplace also writes .cursor-plugin/marketplace.json for a
	// single-plugin repository.
	Marketplace bool `yaml:"marketplace,omitempty" json:"marketplace,omitempty" toml:"marketplace,omitempty"`
}

// GeminiExtras holds Gemini-extension-specific manifest fields.
type GeminiExtras struct {
	ContextFileName string `yaml:"context_file_name,omitempty" json:"context_file_name,omitempty" toml:"context_file_name,omitempty"` //nolint:tagliatelle
	// Commands also bundles the plugin's commands as commands/<name>.toml
	// custom commands (default off).
	Commands bool `yaml:"commands,omitempty" json:"commands,omitempty" toml:"commands,omitempty"`
}

// KimiExtras holds Kimi-plugin-specific manifest fields.
type KimiExtras struct {
	SkillInstructions string `yaml:"skill_instructions,omitempty" json:"skill_instructions,omitempty" toml:"skill_instructions,omitempty"`    //nolint:tagliatelle
	SessionStartSkill string `yaml:"session_start_skill,omitempty" json:"session_start_skill,omitempty" toml:"session_start_skill,omitempty"` //nolint:tagliatelle
}

// HermesExtras holds Hermes-Agent-specific packaging fields.
type HermesExtras struct {
	// Source is a project-relative Python module implementing register(ctx).
	// Empty defaults to .ai-rulez/hermes/index.py.
	Source string `yaml:"source,omitempty" json:"source,omitempty" toml:"source,omitempty"`
	// RequiresPython is the Python requirement for the generated wheel.
	// Empty defaults to >=3.11.
	RequiresPython string `yaml:"requires_python,omitempty" json:"requires_python,omitempty" toml:"requires_python,omitempty"` //nolint:tagliatelle
}

// MarketplaceAuthoring describes the marketplace index emitted for a plugin (or
// a set of plugins, for a monorepo). Distinct from the consumer MarketplaceConfig.
type MarketplaceAuthoring struct {
	Name        string  `yaml:"name" json:"name" toml:"name"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Owner       *Author `yaml:"owner,omitempty" json:"owner,omitempty" toml:"owner,omitempty"`

	// Members lists sub-project directories for a multi-plugin monorepo. Empty
	// means single-plugin (marketplace source "./").
	Members []string `yaml:"members,omitempty" json:"members,omitempty" toml:"members,omitempty"`

	// OutputDir is the project-relative directory the domain-plugin marketplace
	// is written to (".claude-plugin/marketplace.json" and "plugins/<name>/"
	// beneath it). Empty means the project root. Used only when FromDomains or
	// Plugins is set.
	OutputDir string `yaml:"output_dir,omitempty" json:"output_dir,omitempty" toml:"output_dir,omitempty"` //nolint:tagliatelle

	// FromDomains turns every selected domain into its own plugin.
	FromDomains *DomainPluginsConfig `yaml:"from_domains,omitempty" json:"from_domains,omitempty" toml:"from_domains,omitempty"` //nolint:tagliatelle

	// Plugins declares plugins by hand, mixing domains and root content. An entry
	// named like a FromDomains plugin replaces it.
	Plugins []MarketplacePlugin `yaml:"plugins,omitempty" json:"plugins,omitempty" toml:"plugins,omitempty"`

	// CursorIndex also writes .cursor-plugin/marketplace.json next to the
	// Claude index for members and domain plugins (default off).
	CursorIndex bool `yaml:"cursor_index,omitempty" json:"cursor_index,omitempty" toml:"cursor_index,omitempty"` //nolint:tagliatelle

	// CatalogSkill generates a skill listing the plugins and how to enable them.
	CatalogSkill *CatalogSkillConfig `yaml:"catalog_skill,omitempty" json:"catalog_skill,omitempty" toml:"catalog_skill,omitempty"` //nolint:tagliatelle
}

// HasDomainPlugins reports whether the marketplace generates plugins from the
// domain tree (from_domains or [[marketplace.plugins]]).
func (m *MarketplaceAuthoring) HasDomainPlugins() bool {
	if m == nil {
		return false
	}
	return m.FromDomains.IsEnabled() || len(m.Plugins) > 0
}

// DomainPluginsConfig is the [marketplace.from_domains] block: one plugin per
// domain, named NamePrefix + domain.
type DomainPluginsConfig struct {
	// Enabled defaults to true when the block is present.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
	// NamePrefix is prepended to the domain name to form the plugin name.
	NamePrefix string `yaml:"name_prefix,omitempty" json:"name_prefix,omitempty" toml:"name_prefix,omitempty"` //nolint:tagliatelle
	// Include and Exclude select domains by name or glob. Empty Include means all.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// PluginDefaults supplies version, category, keywords, default_enabled and
	// runtimes to every generated plugin.
	PluginDefaults `yaml:",inline"`
}

// IsEnabled reports whether the block is present and not switched off.
func (d *DomainPluginsConfig) IsEnabled() bool {
	return d != nil && (d.Enabled == nil || *d.Enabled)
}

// PluginDefaults are the per-plugin fields shared by from_domains and
// [[marketplace.plugins]]. Unset fields fall back to the [plugin] block.
type PluginDefaults struct {
	Version  string   `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	Category string   `yaml:"category,omitempty" json:"category,omitempty" toml:"category,omitempty"`
	Keywords []string `yaml:"keywords,omitempty" json:"keywords,omitempty" toml:"keywords,omitempty"`
	// DefaultEnabled is emitted as the marketplace entry's defaultEnabled
	// (Claude Code starts the plugin disabled when false). Unset omits it.
	DefaultEnabled *bool    `yaml:"default_enabled,omitempty" json:"default_enabled,omitempty" toml:"default_enabled,omitempty"` //nolint:tagliatelle
	Runtimes       []string `yaml:"runtimes,omitempty" json:"runtimes,omitempty" toml:"runtimes,omitempty"`
}

// MarketplacePlugin is one [[marketplace.plugins]] entry.
type MarketplacePlugin struct {
	Name        string `yaml:"name" json:"name" toml:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Domains are the domains (names or globs) whose skills, commands and agents
	// the plugin bundles.
	Domains []string `yaml:"domains,omitempty" json:"domains,omitempty" toml:"domains,omitempty"`
	// Skills, Commands and Agents select root content by name or glob.
	Skills   []string `yaml:"skills,omitempty" json:"skills,omitempty" toml:"skills,omitempty"`
	Commands []string `yaml:"commands,omitempty" json:"commands,omitempty" toml:"commands,omitempty"`
	Agents   []string `yaml:"agents,omitempty" json:"agents,omitempty" toml:"agents,omitempty"`
	// Relevance tells Claude Code when to suggest the plugin.
	Relevance      *PluginRelevance `yaml:"relevance,omitempty" json:"relevance,omitempty" toml:"relevance,omitempty"`
	PluginDefaults `yaml:",inline"`
}

// PluginRelevance is the marketplace entry's relevance object: when Claude Code
// suggests installing the plugin. See https://code.claude.com/docs/en/plugins/relevance.
type PluginRelevance struct {
	Topic   string           `yaml:"topic,omitempty" json:"topic,omitempty" toml:"topic,omitempty"`
	Signals RelevanceSignals `yaml:"signals" json:"signals" toml:"signals"`
}

// RelevanceSignals are the matchers of a relevance object. At least one is set.
type RelevanceSignals struct {
	CWD          []string           `yaml:"cwd,omitempty" json:"cwd,omitempty" toml:"cwd,omitempty"`
	CLI          []string           `yaml:"cli,omitempty" json:"cli,omitempty" toml:"cli,omitempty"`
	Hosts        []string           `yaml:"hosts,omitempty" json:"hosts,omitempty" toml:"hosts,omitempty"`
	FilesRead    []string           `yaml:"files_read,omitempty" json:"files_read,omitempty" toml:"files_read,omitempty"`          //nolint:tagliatelle
	ManifestDeps []ManifestDepMatch `yaml:"manifest_deps,omitempty" json:"manifest_deps,omitempty" toml:"manifest_deps,omitempty"` //nolint:tagliatelle
}

// IsEmpty reports whether no signal is set.
func (s RelevanceSignals) IsEmpty() bool {
	return len(s.CWD)+len(s.CLI)+len(s.Hosts)+len(s.FilesRead)+len(s.ManifestDeps) == 0
}

// ManifestDepMatch pairs a manifest path regex with a content regex.
type ManifestDepMatch struct {
	File    string `yaml:"file" json:"file" toml:"file"`
	Pattern string `yaml:"pattern" json:"pattern" toml:"pattern"`
}

// CatalogSkillConfig is the [marketplace.catalog_skill] block.
type CatalogSkillConfig struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
	// Name is the skill name; empty means "plugin-catalog".
	Name string `yaml:"name,omitempty" json:"name,omitempty" toml:"name,omitempty"`
	// Description is the skill description shown to the model.
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
}

// DefaultCatalogSkillName is the catalog skill name when none is configured.
const DefaultCatalogSkillName = "plugin-catalog"

// SkillName returns the configured or default catalog skill name.
func (c *CatalogSkillConfig) SkillName() string {
	if c == nil || c.Name == "" {
		return DefaultCatalogSkillName
	}
	return c.Name
}

// Placement values for [placement] and the `placement` frontmatter key.
const (
	PlacementCore   = "core"
	PlacementPlugin = "plugin"
)

// PlacementConfig is the [placement] block: whether a skill or command is
// generated into .claude/skills ("core") or shipped only through a plugin.
type PlacementConfig struct {
	// Default is "core" (today's behavior) or "plugin".
	Default string `yaml:"default,omitempty" json:"default,omitempty" toml:"default,omitempty"`
	// Core and Plugin list names or globs. A pattern matches the item name or
	// "domains/<domain>/<name>". Core wins over Plugin; frontmatter `placement`
	// wins over both.
	Core   []string `yaml:"core,omitempty" json:"core,omitempty" toml:"core,omitempty"`
	Plugin []string `yaml:"plugin,omitempty" json:"plugin,omitempty" toml:"plugin,omitempty"`
	// HonorTargets makes frontmatter `targets` filter skills in .claude/skills,
	// as it already does for commands.
	HonorTargets bool `yaml:"honor_targets,omitempty" json:"honor_targets,omitempty" toml:"honor_targets,omitempty"` //nolint:tagliatelle
}

// ClaudeConfig groups Claude Code specific output options.
type ClaudeConfig struct {
	Settings *ClaudeSettings `yaml:"settings,omitempty" json:"settings,omitempty" toml:"settings,omitempty"`
	Skills   *ClaudeSkills   `yaml:"skills,omitempty" json:"skills,omitempty" toml:"skills,omitempty"`
}

// ClaudeSkills is the [claude.skills] block: options for the skills written to
// .claude/skills.
type ClaudeSkills struct {
	// HideFromMenu writes `user-invocable: false` on every skill that does not
	// set the key itself, so Claude Code hides them from the / menu and only the
	// model loads them. Off by default: a skill without the key is user-invocable.
	HideFromMenu bool `yaml:"hide_from_menu,omitempty" json:"hide_from_menu,omitempty" toml:"hide_from_menu,omitempty"` //nolint:tagliatelle
}

// HidesSkillsFromMenu reports whether [claude.skills] hide_from_menu is set.
// Safe on a nil receiver.
func (c *ClaudeConfig) HidesSkillsFromMenu() bool {
	return c != nil && c.Skills != nil && c.Skills.HideFromMenu
}

// ClaudeSettings is the [claude.settings] block: opt-in management of the
// plugin keys of .claude/settings.json. Only the listed entries are owned.
type ClaudeSettings struct {
	Manage bool `yaml:"manage,omitempty" json:"manage,omitempty" toml:"manage,omitempty"`
	// RegisterMarketplace owns extraKnownMarketplaces.<marketplace.name>.
	// Defaults to true when Manage is set.
	RegisterMarketplace *bool `yaml:"register_marketplace,omitempty" json:"register_marketplace,omitempty" toml:"register_marketplace,omitempty"` //nolint:tagliatelle
	// MarketplaceSource overrides the default directory source.
	MarketplaceSource *MarketplaceSource `yaml:"marketplace_source,omitempty" json:"marketplace_source,omitempty" toml:"marketplace_source,omitempty"` //nolint:tagliatelle
	AutoUpdate        *bool              `yaml:"auto_update,omitempty" json:"auto_update,omitempty" toml:"auto_update,omitempty"`                      //nolint:tagliatelle
	// EnablePlugins and DisablePlugins own enabledPlugins."<name>@<marketplace>"
	// as true and false. Names are plugin names in the marketplace.
	EnablePlugins  []string `yaml:"enable_plugins,omitempty" json:"enable_plugins,omitempty" toml:"enable_plugins,omitempty"`    //nolint:tagliatelle
	DisablePlugins []string `yaml:"disable_plugins,omitempty" json:"disable_plugins,omitempty" toml:"disable_plugins,omitempty"` //nolint:tagliatelle
	// Managed owns further keys of .claude/settings.json entry by entry (env,
	// skillOverrides). It needs no Manage flag.
	Managed *ManagedSettings `yaml:"managed,omitempty" json:"managed,omitempty" toml:"managed,omitempty"`
}

// MarketplaceSource is a Claude Code marketplace source object.
type MarketplaceSource struct {
	Source string `yaml:"source" json:"source" toml:"source"` // directory, github, git, url
	Path   string `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	Repo   string `yaml:"repo,omitempty" json:"repo,omitempty" toml:"repo,omitempty"`
	URL    string `yaml:"url,omitempty" json:"url,omitempty" toml:"url,omitempty"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
}

// ManagesClaudeSettings reports whether generate owns plugin keys in
// .claude/settings.json.
func (c *Config) ManagesClaudeSettings() bool {
	return c != nil && c.Claude != nil && c.Claude.Settings != nil && c.Claude.Settings.Manage
}

// RegistersMarketplace reports whether extraKnownMarketplaces is owned.
func (s *ClaudeSettings) RegistersMarketplace() bool {
	return s != nil && s.Manage && (s.RegisterMarketplace == nil || *s.RegisterMarketplace)
}

// RelativeDirectoryMarketplace reports whether the managed
// extraKnownMarketplaces entry points at a directory given as a relative path,
// which is the default when no marketplace_source is set. It returns the path.
func (c *Config) RelativeDirectoryMarketplace() (path string, ok bool) {
	if !c.ManagesClaudeSettings() {
		return "", false
	}
	s := c.Claude.Settings
	if !s.RegistersMarketplace() {
		return "", false
	}
	if src := s.MarketplaceSource; src != nil {
		if src.Source != "directory" || src.Path == "" || filepath.IsAbs(src.Path) {
			return "", false
		}
		return src.Path, true
	}
	dir := "."
	if c.Marketplace != nil && c.Marketplace.OutputDir != "" {
		dir = filepath.ToSlash(filepath.Clean(c.Marketplace.OutputDir))
	}
	if dir != "." && !strings.HasPrefix(dir, "./") {
		dir = "./" + dir
	}
	return dir, true
}

// ResolvedRuntimes returns the runtimes to emit for this plugin: the explicit
// Runtimes list if set, otherwise AllPluginRuntimes.
func (p *PluginAuthoring) ResolvedRuntimes() []string {
	if p == nil || len(p.Runtimes) == 0 {
		return AllPluginRuntimes
	}
	return p.Runtimes
}

// DefaultCodexProjectDocMaxBytes is Codex's default project_doc_max_bytes: the
// most combined AGENTS.md content Codex reads (32 KiB).
var DefaultCodexProjectDocMaxBytes = harnesslimits.MustValue("codex.agents_md_chain_bytes")

// CodexConfig is the [codex] block.
type CodexConfig struct {
	// ProjectDocMaxBytes is the project_doc_max_bytes your Codex is configured
	// with; generate warns when the AGENTS.md chain exceeds it. Unset means the
	// Codex default (32 KiB); 0 or a negative value turns the warning off.
	ProjectDocMaxBytes *int `yaml:"project_doc_max_bytes,omitempty" json:"project_doc_max_bytes,omitempty" toml:"project_doc_max_bytes,omitempty"` //nolint:tagliatelle
}

// ProjectDocLimit returns the AGENTS.md byte budget to warn against, or 0 when
// the warning is disabled. Safe on a nil receiver.
func (c *CodexConfig) ProjectDocLimit() int {
	if c == nil || c.ProjectDocMaxBytes == nil {
		return DefaultCodexProjectDocMaxBytes
	}
	if *c.ProjectDocMaxBytes <= 0 {
		return 0
	}
	return *c.ProjectDocMaxBytes
}
