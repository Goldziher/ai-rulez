package config

// This file defines the *authoring* (producer) side of plugins: describing a
// distributable plugin bundle that ai-rulez packages from the project's content
// tree for the Claude/Cursor/Codex/Gemini/Kimi/OpenCode/Factory/Hermes runtimes.
//
// It is intentionally distinct from PluginConfig / MarketplaceConfig in
// types_v4.go, which are the *consumer* side ("install these plugins from a
// marketplace" -> .claude/plugins.json). Do not conflate the two.

// PluginRuntime names a supported target runtime for authored plugin bundles.
const (
	PluginRuntimeClaude   = "claude"
	PluginRuntimeCursor   = "cursor"
	PluginRuntimeCodex    = "codex"
	PluginRuntimeGemini   = "gemini"
	PluginRuntimeKimi     = "kimi"
	PluginRuntimeOpenCode = "opencode"
	PluginRuntimeFactory  = "factory"
	PluginRuntimeHermes   = "hermes"
)

// AllPluginRuntimes lists every runtime the authoring generator can emit, in a
// stable order. Used as the default when a plugin does not restrict Runtimes.
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
type HookGroup struct {
	Event   string       `yaml:"event" json:"event" toml:"event"` // e.g. SessionStart, PreToolUse
	Matcher string       `yaml:"matcher,omitempty" json:"matcher,omitempty" toml:"matcher,omitempty"`
	Hooks   []HookAction `yaml:"hooks,omitempty" json:"hooks,omitempty" toml:"hooks,omitempty"`
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

// GeminiExtras holds Gemini-extension-specific manifest fields.
type GeminiExtras struct {
	ContextFileName string `yaml:"context_file_name,omitempty" json:"context_file_name,omitempty" toml:"context_file_name,omitempty"` //nolint:tagliatelle
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
}

// ResolvedRuntimes returns the runtimes to emit for this plugin: the explicit
// Runtimes list if set, otherwise AllPluginRuntimes.
func (p *PluginAuthoring) ResolvedRuntimes() []string {
	if p == nil || len(p.Runtimes) == 0 {
		return AllPluginRuntimes
	}
	return p.Runtimes
}
