package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

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
	PluginRuntimePi           = "pi"
)

// AllPluginRuntimes lists the runtimes emitted by default when a plugin does not
// restrict Runtimes, in a stable order. Agent Plugins, Copilot and Pi are
// deliberately excluded: they require an explicit runtimes list so adding them never changes
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
var KnownPluginRuntimes = append(append([]string{}, AllPluginRuntimes...),
	PluginRuntimeAgentPlugins, PluginRuntimeCopilot, PluginRuntimePi)

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
	Name        string `yaml:"name" json:"name" toml:"name"`
	DisplayName string `yaml:"display_name,omitempty" json:"display_name,omitempty" toml:"display_name,omitempty"` //nolint:tagliatelle
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	// VersionFrom is a project-relative manifest (package.json, pyproject.toml
	// or Cargo.toml) whose declared version replaces Version. Mutually
	// exclusive with Version.
	VersionFrom string   `yaml:"version_from,omitempty" json:"version_from,omitempty" toml:"version_from,omitempty"` //nolint:tagliatelle
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

	// SkillsDirOwned declares each skills directory the bundle writes (the
	// parent of every generated skill directory) fully owned by the bundle:
	// verify --plugin reports an entry it did not generate there as drift of kind
	// extra, and generate --plugin removes it. Off by default, when a skill
	// directory or file that the bundle does not generate is never touched.
	SkillsDirOwned bool `yaml:"skills_dir_owned,omitempty" json:"skills_dir_owned,omitempty" toml:"skills_dir_owned,omitempty"` //nolint:tagliatelle

	// IncludeDomains lists the domains (names or globs) whose skills, commands
	// and agents are bundled next to the root content. Empty keeps the bundle
	// root-only. On a name collision the root item wins.
	IncludeDomains []string `yaml:"include_domains,omitempty" json:"include_domains,omitempty" toml:"include_domains,omitempty"` //nolint:tagliatelle

	// IncludeEvals bundles eval cases into the plugin: each skill's evals/
	// directory and the project-level .ai-rulez/evals/ tree (the latter at
	// <bundle>/evals/). Off by default, so a skill's evals/ directory is not
	// shipped to consumers of the bundle.
	IncludeEvals bool `yaml:"include_evals,omitempty" json:"include_evals,omitempty" toml:"include_evals,omitempty"` //nolint:tagliatelle

	// ProvenanceHeaders writes the "AI-RULEZ :: GENERATED FILE" header into
	// bundled Markdown/JS/TS/PY outputs. Defaults to true; set false to write
	// those outputs byte-identical to their sources. The provenance sidecar
	// still records every output's content hash either way.
	ProvenanceHeaders *bool `yaml:"provenance_headers,omitempty" json:"provenance_headers,omitempty" toml:"provenance_headers,omitempty"` //nolint:tagliatelle

	// Include lists globs (relative to the plugin's content root) of arbitrary
	// files copied verbatim into the Agent Plugins package, keeping their
	// relative path. It is how a `./`-relative MCP command can point at a
	// launcher the package ships (Agent Plugins §4.2, §7.2.1). Empty keeps the
	// package to plugin.json, mcp.json and skills/.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`

	// SharedResources lists directories (relative to the plugin's content root)
	// whose files are copied into every bundled skill, at
	// <skill>/<directory>/... So a skill keeps the reference docs and templates
	// it links to once it is installed on its own. A relative SKILL.md link that
	// climbs to the shared directory (../../references/x.md) is rewritten to the
	// copy (references/x.md). A name clash with a file the skill already has is
	// an error. Empty copies nothing.
	SharedResources []string `yaml:"shared_resources,omitempty" json:"shared_resources,omitempty" toml:"shared_resources,omitempty"` //nolint:tagliatelle

	// Spec is the Agent Plugins version the agent-plugins, copilot and root-layout
	// codex bundles follow: "1.0.0" (the default) or "1.1.0". plugin.json and
	// mcp.json carry the matching $schema.
	Spec string `yaml:"spec,omitempty" json:"spec,omitempty" toml:"spec,omitempty"`

	// PackageManager is written as packageManager in the OpenCode and Pi root
	// package.json (for example "pnpm@9.15.0"), so a consumer can pin the
	// package manager it uses. Empty omits the field.
	PackageManager string `yaml:"package_manager,omitempty" json:"package_manager,omitempty" toml:"package_manager,omitempty"` //nolint:tagliatelle

	// OpencodeDependency overrides the "@opencode/plugin" dependency version in
	// the OpenCode root package.json. Empty uses the built-in default.
	OpencodeDependency string `yaml:"opencode_dependency,omitempty" json:"opencode_dependency,omitempty" toml:"opencode_dependency,omitempty"` //nolint:tagliatelle

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

	// Pi holds Pi-package-specific packaging fields.
	Pi *PiExtras `yaml:"pi,omitempty" json:"pi,omitempty" toml:"pi,omitempty"`
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

// hookEventPreToolUse is the Claude Code event that fires before a tool call.
const hookEventPreToolUse = "PreToolUse"

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
	hookEventPreToolUse,
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
	hookEventPreToolUse,
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
	ContextFileName string          `yaml:"context_file_name,omitempty" json:"context_file_name,omitempty" toml:"context_file_name,omitempty"` //nolint:tagliatelle
	Settings        []GeminiSetting `yaml:"settings,omitempty" json:"settings,omitempty" toml:"settings,omitempty"`
	// Commands also bundles the plugin's commands as commands/<name>.toml
	// custom commands (default off).
	Commands bool `yaml:"commands,omitempty" json:"commands,omitempty" toml:"commands,omitempty"`
}

// GeminiSetting declares a runtime-provided value; credential values are never authored here.
// https://geminicli.com/docs/extensions/reference/#extension-settings
type GeminiSetting struct {
	Name        string `yaml:"name" json:"name" toml:"name"`
	Description string `yaml:"description" json:"description" toml:"description"`
	EnvVar      string `yaml:"env_var" json:"env_var" toml:"env_var"` //nolint:tagliatelle
	Sensitive   bool   `yaml:"sensitive,omitempty" json:"sensitive,omitempty" toml:"sensitive,omitempty"`
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
	// Dependencies are PEP 508 requirements installed with the Hermes plugin.
	// Empty leaves both the wheel and project plugin dependency-free.
	Dependencies []string `yaml:"dependencies,omitempty" json:"dependencies,omitempty" toml:"dependencies,omitempty"`
}

// Pi package_json modes.
const (
	// PiPackageJSONGenerate writes the root package.json from the plugin
	// metadata, replacing any existing file (the default).
	PiPackageJSONGenerate = "generate"
	// PiPackageJSONMerge updates only the keys ai-rulez owns in an existing root
	// package.json and keeps every other key.
	PiPackageJSONMerge = "merge"
)

// Pi skills_entry modes.
const (
	// PiSkillsEntryEach lists every skill directory in pi.skills and every file
	// in files (the default).
	PiSkillsEntryEach = "each"
	// PiSkillsEntryDirectory lists the skills directory once in pi.skills and
	// files, so adding a skill does not change package.json.
	PiSkillsEntryDirectory = "directory"
)

// piPackageNamePattern is the npm package-name grammar: an optional lowercase
// scope and a lowercase name, each from URL-safe characters.
var piPackageNamePattern = regexp.MustCompile(`^(@[a-z0-9~-][a-z0-9._~-]*/)?[a-z0-9~-][a-z0-9._~-]*$`)

// maxPiPackageNameLen is npm's package name length limit.
const maxPiPackageNameLen = 214

// DefaultPiSkillsDir is where the Pi runtime writes bundled skills when
// [plugin.pi] skills_dir is unset.
const DefaultPiSkillsDir = ".pi/skills"

// PiExtras holds Pi-package-specific packaging fields.
type PiExtras struct {
	// SkillsDir is the directory each bundled skill is written to, relative to
	// the project root. Empty uses DefaultPiSkillsDir (".pi/skills"); set it to
	// "skills" to reuse the tree the other runtimes write, so a skill is written
	// once and Pi points at the same copy.
	SkillsDir string `yaml:"skills_dir,omitempty" json:"skills_dir,omitempty" toml:"skills_dir,omitempty"` //nolint:tagliatelle
	// PackageJSON is PiPackageJSONGenerate (default) to write the root
	// package.json from the plugin metadata, or PiPackageJSONMerge to update
	// only the keys ai-rulez owns and keep every other key.
	PackageJSON string `yaml:"package_json,omitempty" json:"package_json,omitempty" toml:"package_json,omitempty"` //nolint:tagliatelle
	// PackageName is the npm package name. Empty derives @<owner>/pi-<plugin-name>
	// (and, under package_json = "merge", keeps an authored name).
	PackageName string `yaml:"package_name,omitempty" json:"package_name,omitempty" toml:"package_name,omitempty"` //nolint:tagliatelle
	// SkillsEntry is PiSkillsEntryEach (default) to list every skill, or
	// PiSkillsEntryDirectory to list the skills directory once.
	SkillsEntry string `yaml:"skills_entry,omitempty" json:"skills_entry,omitempty" toml:"skills_entry,omitempty"` //nolint:tagliatelle
}

// ResolvedSkillsEntry returns the configured skills_entry mode or the default.
// Safe on a nil receiver.
func (e *PiExtras) ResolvedSkillsEntry() string {
	if e == nil || e.SkillsEntry == "" {
		return PiSkillsEntryEach
	}
	return e.SkillsEntry
}

// ResolvedSkillsDir returns the configured Pi skills directory or the default.
// Safe on a nil receiver.
func (e *PiExtras) ResolvedSkillsDir() string {
	if e == nil || e.SkillsDir == "" {
		return DefaultPiSkillsDir
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(e.SkillsDir)))
}

// ResolvedPackageJSON returns the configured package_json mode or the default.
// Safe on a nil receiver.
func (e *PiExtras) ResolvedPackageJSON() string {
	if e == nil || e.PackageJSON == "" {
		return PiPackageJSONGenerate
	}
	return e.PackageJSON
}

// MergesPackageJSON reports whether the Pi runtime merges the root package.json
// instead of generating it. Safe on a nil receiver.
func (e *PiExtras) MergesPackageJSON() bool {
	return e != nil && e.PackageJSON == PiPackageJSONMerge
}

// Validate checks the [plugin.pi] block. Safe on a nil receiver.
func (e *PiExtras) Validate() error {
	if e == nil {
		return nil
	}
	switch e.PackageJSON {
	case "", PiPackageJSONGenerate, PiPackageJSONMerge:
	default:
		return oops.
			With("field", "plugin.pi.package_json").
			With("value", e.PackageJSON).
			Hint("Use \"generate\" (default) or \"merge\"").
			Errorf("plugin.pi.package_json has an unknown value %q", e.PackageJSON)
	}
	switch e.SkillsEntry {
	case "", PiSkillsEntryEach, PiSkillsEntryDirectory:
	default:
		return oops.
			With("field", "plugin.pi.skills_entry").
			With("value", e.SkillsEntry).
			Hint("Use \"each\" (default) or \"directory\"").
			Errorf("plugin.pi.skills_entry has an unknown value %q", e.SkillsEntry)
	}
	if e.PackageName != "" && (len(e.PackageName) > maxPiPackageNameLen || !piPackageNamePattern.MatchString(e.PackageName)) {
		return oops.
			With("field", "plugin.pi.package_name").
			With("value", e.PackageName).
			Hint("Use a lowercase npm name such as \"my-skills\" or \"@scope/my-skills\"").
			Errorf("plugin.pi.package_name is not a valid npm package name: %q", e.PackageName)
	}
	if e.SkillsDir != "" && isUnsafeProjectPath(e.SkillsDir) {
		return oops.
			With("field", "plugin.pi.skills_dir").
			With("value", e.SkillsDir).
			Hint("Use a project-relative directory that does not contain '..'").
			Errorf("plugin.pi.skills_dir is not a safe project path: %q", e.SkillsDir)
	}
	return nil
}

// PluginContentRootDir resolves the directory [plugin] shared_resources and
// include globs are relative to: content_root when set (under sourceDir),
// otherwise the config directory, otherwise sourceDir. Safe on a nil receiver.
func (p *PluginAuthoring) PluginContentRootDir(cfg *Config, sourceDir string) string {
	if p != nil && p.ContentRoot != "" {
		return filepath.Join(sourceDir, p.ContentRoot)
	}
	if cfg != nil && cfg.ConfigDir != "" {
		return cfg.ConfigDir
	}
	return sourceDir
}

// SharedResource is a resolved [plugin] shared_resources directory: its base
// name (the subdirectory copied into every skill) and its absolute source
// directory.
type SharedResource struct {
	Name string
	Dir  string
}

// ResolveSharedResources resolves the [plugin] shared_resources directories
// against the plugin content root. Each entry must be a safe project-relative
// directory that exists: a missing, non-directory or escaping entry is an error,
// because the bundle would otherwise silently ship skills with broken links. Safe
// on a nil receiver.
func (p *PluginAuthoring) ResolveSharedResources(cfg *Config) ([]SharedResource, error) {
	if p == nil || len(p.SharedResources) == 0 {
		return nil, nil
	}
	sourceDir := ""
	if cfg != nil {
		sourceDir = cfg.BaseDir
	}
	root := p.PluginContentRootDir(cfg, sourceDir)
	resources := make([]SharedResource, 0, len(p.SharedResources))
	seen := make(map[string]bool, len(p.SharedResources))
	for _, entry := range p.SharedResources {
		trimmed := strings.TrimSuffix(strings.TrimSpace(filepath.ToSlash(entry)), "/")
		if trimmed == "" || trimmed == "." || isUnsafeProjectPath(trimmed) {
			return nil, oops.
				With("field", "plugin.shared_resources").
				With("value", entry).
				Hint("Use a directory relative to the plugin's content root that does not contain '..'").
				Errorf("plugin shared_resources entry is not a safe directory: %q", entry)
		}
		name := filepath.Base(trimmed)
		if seen[name] {
			return nil, oops.
				With("field", "plugin.shared_resources").
				With("value", entry).
				Errorf("plugin shared_resources lists duplicate directory name %q", name)
		}
		seen[name] = true
		dir := filepath.Join(root, filepath.FromSlash(trimmed))
		info, err := os.Stat(dir)
		if err != nil {
			return nil, oops.
				With("field", "plugin.shared_resources").
				With("path", dir).
				Hint("Create the directory or remove it from shared_resources").
				Wrapf(err, "plugin shared_resources directory not found: %q", entry)
		}
		if !info.IsDir() {
			return nil, oops.
				With("field", "plugin.shared_resources").
				With("path", dir).
				Errorf("plugin shared_resources entry is not a directory: %q", entry)
		}
		resources = append(resources, SharedResource{Name: name, Dir: dir})
	}
	return resources, nil
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
	// MCPServers names root [[mcp_servers]] the plugin bundles. Empty bundles none.
	MCPServers []string `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty" toml:"mcp_servers,omitempty"` //nolint:tagliatelle
	// Hooks are the lifecycle hooks the plugin ships. Empty ships none.
	Hooks []HookGroup `yaml:"hooks,omitempty" json:"hooks,omitempty" toml:"hooks,omitempty"`
	// Author, License, Homepage and Repository fall back to the [plugin] block.
	Author     *Author `yaml:"author,omitempty" json:"author,omitempty" toml:"author,omitempty"`
	License    string  `yaml:"license,omitempty" json:"license,omitempty" toml:"license,omitempty"`
	Homepage   string  `yaml:"homepage,omitempty" json:"homepage,omitempty" toml:"homepage,omitempty"`
	Repository string  `yaml:"repository,omitempty" json:"repository,omitempty" toml:"repository,omitempty"`
	// Relevance tells Claude Code when to suggest the plugin.
	Relevance      *PluginRelevance `yaml:"relevance,omitempty" json:"relevance,omitempty" toml:"relevance,omitempty"`
	PluginDefaults `yaml:",inline"`
}

// MarketplaceDefinition is one [[marketplaces_authoring]] entry: a marketplace
// index that lists plugin definitions by name. Several can come from one root,
// each with its own index and provenance sidecar. Distinct from the consumer
// MarketplaceConfig and from the single [marketplace] block.
type MarketplaceDefinition struct {
	Name        string  `yaml:"name" json:"name" toml:"name"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	Owner       *Author `yaml:"owner,omitempty" json:"owner,omitempty" toml:"owner,omitempty"`
	// Plugins are the names of the plugin definitions ([[plugins_authoring]] or
	// [[marketplace.plugins]]) the index lists. An empty list lists nothing.
	Plugins []string `yaml:"plugins,omitempty" json:"plugins,omitempty" toml:"plugins,omitempty"`
	// OutputDir is the project-relative directory holding the index and its
	// plugins/<name>/ bundles. Empty means the project root. Unique per marketplace.
	OutputDir   string `yaml:"output_dir,omitempty" json:"output_dir,omitempty" toml:"output_dir,omitempty"`       //nolint:tagliatelle
	CursorIndex bool   `yaml:"cursor_index,omitempty" json:"cursor_index,omitempty" toml:"cursor_index,omitempty"` //nolint:tagliatelle
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
	// ClaudeMD controls the CLAUDE.md file the claude preset writes when
	// agents_md is on: empty or "shim" keeps the "@AGENTS.md" import file,
	// "none" writes only AGENTS.md (which Claude Code >= 2.1.277 reads
	// directly) and removes a previously generated shim.
	ClaudeMD string `yaml:"claude_md,omitempty" json:"claude_md,omitempty" toml:"claude_md,omitempty"` //nolint:tagliatelle
}

// OmitsClaudeShim reports whether claude_md = "none" turns off the CLAUDE.md
// shim, so the claude preset relies on the shared AGENTS.md alone. Safe on a
// nil receiver.
func (c *ClaudeConfig) OmitsClaudeShim() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.ClaudeMD), ClaudeMDNone)
}

// ClaudeMDOrEmpty returns the configured claude_md mode, or "" when the [claude]
// table is absent. Safe on a nil receiver.
func (c *ClaudeConfig) ClaudeMDOrEmpty() string {
	if c == nil {
		return ""
	}
	return c.ClaudeMD
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

// needsLocalMarketplace reports whether the block needs the [marketplace] name:
// a bare plugin name is qualified with it, and the default is to register it.
// A block that lists only fully qualified ids and does not ask to register the
// local marketplace needs none.
func (s *ClaudeSettings) needsLocalMarketplace() bool {
	bare, listed := false, false
	for _, names := range [][]string{s.EnablePlugins, s.DisablePlugins} {
		for _, name := range names {
			listed = true
			bare = bare || !strings.Contains(name, "@")
		}
	}
	explicit := s.RegisterMarketplace != nil && *s.RegisterMarketplace
	return bare || (s.RegistersMarketplace() && (!listed || explicit))
}

// RegistersLocalMarketplace reports whether extraKnownMarketplaces gets the
// entry of this project's own [marketplace].
func (c *Config) RegistersLocalMarketplace() bool {
	if !c.ManagesClaudeSettings() || !c.Claude.Settings.RegistersMarketplace() {
		return false
	}
	// A validated block without a [marketplace] name lists only qualified ids.
	return (c.Marketplace != nil && c.Marketplace.Name != "") || c.Claude.Settings.needsLocalMarketplace()
}

// RelativeDirectoryMarketplace reports whether the managed
// extraKnownMarketplaces entry points at a directory given as a relative path,
// which is the default when no marketplace_source is set. It returns the path.
func (c *Config) RelativeDirectoryMarketplace() (path string, ok bool) {
	if !c.ManagesClaudeSettings() {
		return "", false
	}
	s := c.Claude.Settings
	if !c.RegistersLocalMarketplace() {
		return "", false
	}
	if src := s.MarketplaceSource; src != nil {
		if src.Source != MarketplaceTypeDirectory || src.Path == "" || rooted(src.Path) {
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

// WritesProvenanceHeaders reports whether bundled comment-safe outputs carry the
// AI-RULEZ generated-file header. It defaults to true, so only an explicit false
// turns the headers off. Safe on a nil receiver.
func (p *PluginAuthoring) WritesProvenanceHeaders() bool {
	return p == nil || p.ProvenanceHeaders == nil || *p.ProvenanceHeaders
}

// Supported version_from manifest file names, shared by the resolver and its
// allowlist so the two can never drift.
const (
	manifestPackageJSON   = "package.json"
	manifestPyprojectTOML = "pyproject.toml"
	manifestCargoTOML     = "Cargo.toml"
)

// versionFromKeys returns the key path that holds the version in a supported
// version_from manifest: package.json -> version, pyproject.toml ->
// project.version, Cargo.toml -> package.version. The second result is false for
// an unsupported file name. It is a function, not a package variable, so no
// mutable table is added to the package.
func versionFromKeys(name string) ([]string, bool) {
	switch name {
	case manifestPackageJSON:
		return []string{docKeyVersion}, true
	case manifestPyprojectTOML:
		return []string{"project", docKeyVersion}, true
	case manifestCargoTOML:
		return []string{"package", docKeyVersion}, true
	}
	return nil, false
}

// VersionFromSources lists, sorted, the file names version_from can read. It is
// the allowlist the resolver and its error hints share.
func VersionFromSources() []string {
	return []string{manifestCargoTOML, manifestPackageJSON, manifestPyprojectTOML}
}

// ResolvedVersion returns the plugin version to publish: the literal Version, or
// the value VersionFrom names. Safe on a nil receiver.
func (p *PluginAuthoring) ResolvedVersion(baseDir string) (string, error) {
	if p == nil {
		return "", nil
	}
	if p.VersionFrom == "" {
		return p.Version, nil
	}
	return ReadVersionFrom(baseDir, p.VersionFrom)
}

// ReadVersionFrom returns the version declared by versionFrom, a project-relative
// path to a supported manifest. It errors when the path escapes the project, the
// file name is unsupported, the file is missing, or it carries no version key.
func ReadVersionFrom(baseDir, versionFrom string) (string, error) {
	if isUnsafeProjectPath(versionFrom) {
		return "", oops.
			With("field", "plugin.version_from").
			With("value", versionFrom).
			Hint("Use a project-relative path that does not contain '..'").
			Errorf("plugin version_from is not a safe project path: %q", versionFrom)
	}
	keys, ok := versionFromKeys(filepath.Base(filepath.FromSlash(versionFrom)))
	if !ok {
		return "", oops.
			With("field", "plugin.version_from").
			With("value", versionFrom).
			Hint("Supported version_from files: "+strings.Join(VersionFromSources(), ", ")).
			Errorf("plugin version_from names an unsupported file: %q", versionFrom)
	}
	path := filepath.Join(baseDir, filepath.FromSlash(versionFrom))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", oops.
			With("field", "plugin.version_from").
			With("path", path).
			Hint("Point version_from at a manifest committed to the project").
			Wrapf(err, "plugin version_from file not found: %q", versionFrom)
	}
	version, found, err := versionFromDocument(data, versionFrom, keys)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(version) == "" {
		return "", oops.
			With("field", "plugin.version_from").
			With("path", path).
			With("key", strings.Join(keys, ".")).
			Hint("Set the version key in the manifest, or use version = \"...\" instead").
			Errorf("plugin version_from %q has no %s", versionFrom, strings.Join(keys, "."))
	}
	return version, nil
}

// versionFromDocument decodes a supported manifest and returns the string at the
// dotted key path. A malformed document is reported; a missing key is not found.
func versionFromDocument(data []byte, versionFrom string, keys []string) (version string, found bool, err error) {
	document := map[string]any{}
	if strings.EqualFold(filepath.Ext(versionFrom), ".json") {
		err = json.Unmarshal(data, &document)
	} else {
		err = toml.Unmarshal(data, &document)
	}
	if err != nil {
		return "", false, oops.
			With("field", "plugin.version_from").
			With("value", versionFrom).
			Wrapf(err, "plugin version_from is not parseable: %q", versionFrom)
	}
	var current any = document
	for _, key := range keys {
		table, ok := current.(map[string]any)
		if !ok {
			return "", false, nil
		}
		value, present := table[key]
		if !present {
			return "", false, nil
		}
		current = value
	}
	str, ok := current.(string)
	return str, ok, nil
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
