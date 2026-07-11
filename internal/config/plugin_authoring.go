package config

// This file defines the *authoring* (producer) side of plugins: describing a
// distributable plugin bundle that ai-rulez packages from the project's content
// tree for the Claude/Cursor/Codex/Gemini/Kimi/OpenCode/Factory runtimes.
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

	// Runtimes restricts which runtime manifests are emitted. Empty means all
	// of AllPluginRuntimes.
	Runtimes []string `yaml:"runtimes,omitempty" json:"runtimes,omitempty" toml:"runtimes,omitempty"`

	// MCP declares the plugin's bundled MCP servers using the canonical
	// ${PLUGIN_ROOT} launch variable; each runtime renderer rewrites it to the
	// runtime-specific root variable. When empty, the project's [[mcp_servers]]
	// are used as the source.
	MCP []PluginMCPLaunch `yaml:"mcp,omitempty" json:"mcp,omitempty" toml:"mcp,omitempty"`

	// Hooks declares lifecycle hooks emitted as hooks.json (Claude/Cursor) or
	// inline hooks{} (Gemini). Hook scripts referenced by command are passed
	// through, not synthesized.
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

// HookGroup is one lifecycle-event hook group.
type HookGroup struct {
	Event   string       `yaml:"event" json:"event" toml:"event"` // e.g. SessionStart, PreToolUse
	Matcher string       `yaml:"matcher,omitempty" json:"matcher,omitempty" toml:"matcher,omitempty"`
	Hooks   []HookAction `yaml:"hooks,omitempty" json:"hooks,omitempty" toml:"hooks,omitempty"`
}

// HookAction is one action within a HookGroup.
type HookAction struct {
	Type    string `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"` // defaults to "command"
	Command string `yaml:"command" json:"command" toml:"command"`
	Async   bool   `yaml:"async,omitempty" json:"async,omitempty" toml:"async,omitempty"`
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
