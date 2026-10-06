// Package providers implements the declarative provider DSL. Each builtin
// preset (and any user-defined provider) is expressed as a TOML/YAML/JSON
// document matching schema/provider.schema.json. A single generic renderer
// dispatches on the closed-set enums declared in the spec, replacing the 13
// hand-written preset generators in internal/generator/presets/.
//
// Commit (a): builds the package + embeds claude.toml but does NOT register
// the DSL-backed generator. The existing internal/generator/presets/claude.go
// remains authoritative until commit (b) swaps registration.
package providers

import "github.com/Goldziher/ai-rulez/v5/internal/config"

// ProviderSpec is the typed mirror of schema/provider.schema.json. Loaded
// from disk (TOML/YAML/JSON), validated, and fed into Render.
type ProviderSpec struct {
	Name        string                 `toml:"name" yaml:"name" json:"name"`
	DisplayName string                 `toml:"display_name,omitempty" yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Root        *RootSpec              `toml:"root,omitempty" yaml:"root,omitempty" json:"root,omitempty"`
	Directories []string               `toml:"directories,omitempty" yaml:"directories,omitempty" json:"directories,omitempty"`
	Outputs     map[string]*OutputSpec `toml:"outputs,omitempty" yaml:"outputs,omitempty" json:"outputs,omitempty"`
	EffortMap   *EffortMapSpec         `toml:"effort_map,omitempty" yaml:"effort_map,omitempty" json:"effort_map,omitempty"`
	Model       *ModelSpec             `toml:"model,omitempty" yaml:"model,omitempty" json:"model,omitempty"`
	Sidecars    []*SidecarSpec         `toml:"sidecars,omitempty" yaml:"sidecars,omitempty" json:"sidecars,omitempty"`
	// Global maps the spec's outputs onto the user-scope (home directory) layout
	// of the tool. It is declarative only; see GlobalSpec.
	Global *GlobalSpec `toml:"global,omitempty" yaml:"global,omitempty" json:"global,omitempty"`
	// Listing says which item kinds the harness lists in its prompt at session
	// start, so `ai-rulez tokens` can charge their name and description.
	Listing *config.ListingSpec `toml:"listing,omitempty" yaml:"listing,omitempty" json:"listing,omitempty"`
}

// RootSpec declares the top-level instructions file (CLAUDE.md, AGENTS.md, ...).
type RootSpec struct {
	File     string   `toml:"file" yaml:"file" json:"file"`
	Sections []string `toml:"sections" yaml:"sections" json:"sections"`
	// LocalFile is the machine-local counterpart of File that the tool loads
	// natively. Empty means the ".local" variant of File (CLAUDE.md →
	// CLAUDE.local.md), LocalFileNone means the tool has no such file and local
	// content is reported as not written, and any other value is a relative path.
	LocalFile string `toml:"local_file,omitempty" yaml:"local_file,omitempty" json:"local_file,omitempty"`
	// OmitHeaderAgents leaves the agent count out of the detailed header (it reads
	// "agents=0"), for a root file another preset also writes with that count.
	OmitHeaderAgents bool `toml:"omit_header_agents,omitempty" yaml:"omit_header_agents,omitempty" json:"omit_header_agents,omitempty"`
}

// LocalFileNone is the RootSpec.LocalFile value for a tool with no machine-local
// instructions file.
const LocalFileNone = "none"

// OutputSpec is the per-content-type emit rule. Keyed on content type
// (rules, context, skills, agents, commands) inside ProviderSpec.Outputs.
type OutputSpec struct {
	Mode        string           `toml:"mode" yaml:"mode" json:"mode"`
	Dir         string           `toml:"dir,omitempty" yaml:"dir,omitempty" json:"dir,omitempty"`
	Filename    string           `toml:"filename,omitempty" yaml:"filename,omitempty" json:"filename,omitempty"`
	Resources   bool             `toml:"resources,omitempty" yaml:"resources,omitempty" json:"resources,omitempty"`
	Filter      string           `toml:"filter,omitempty" yaml:"filter,omitempty" json:"filter,omitempty"`
	Body        *BodySpec        `toml:"body,omitempty" yaml:"body,omitempty" json:"body,omitempty"`
	Frontmatter *FrontmatterSpec `toml:"frontmatter,omitempty" yaml:"frontmatter,omitempty" json:"frontmatter,omitempty"`
	// Split (outputs.rules only) makes the output honor the `[rules] mode`
	// setting: in split mode every rule and path-scoped context item becomes a
	// file; in inline mode only what InlineFilter selects does.
	Split bool `toml:"split,omitempty" yaml:"split,omitempty" json:"split,omitempty"`
	// InlineFilter (split outputs only) selects the items that still get a file
	// when the rules mode is inline: "path_scoped" or "" (none).
	InlineFilter string `toml:"inline_filter,omitempty" yaml:"inline_filter,omitempty" json:"inline_filter,omitempty"`
	// AlwaysFiles (split outputs only) writes every rule and context item to a
	// rule file in every rules mode, always-on ones included, so the spec needs
	// no root file that inlines rules (trae, aiassistant, takt).
	AlwaysFiles bool `toml:"always_files,omitempty" yaml:"always_files,omitempty" json:"always_files,omitempty"`
	// InlineUnscoped (split outputs only) keeps the items the tool's rule files
	// cannot apply automatically (auto and manual activation, and globs that are
	// all negated) in the root file instead of writing rule files for them
	// (copilot: no applyTo, so the file would never load).
	InlineUnscoped bool `toml:"inline_unscoped,omitempty" yaml:"inline_unscoped,omitempty" json:"inline_unscoped,omitempty"`
	// Dialect (split outputs only) names the rule-file frontmatter vocabulary.
	// "mapped" (implied by an Activation block) builds the frontmatter from the
	// spec instead of from Go.
	Dialect string `toml:"dialect,omitempty" yaml:"dialect,omitempty" json:"dialect,omitempty"`
	// Activation (split outputs only) declares the frontmatter each activation
	// mode of a rule produces, so a tool's rules folder needs no Go dialect.
	Activation *ActivationSpec `toml:"activation,omitempty" yaml:"activation,omitempty" json:"activation,omitempty"`
	// File (aggregate outputs only) is the single project-relative file every
	// item is rendered into.
	File string `toml:"file,omitempty" yaml:"file,omitempty" json:"file,omitempty"`
	// Header (aggregate outputs only) is text written above the first item.
	Header string `toml:"header,omitempty" yaml:"header,omitempty" json:"header,omitempty"`
}

// ActivationSpec maps a rule's activation mode onto the frontmatter fields a
// tool's rules folder understands. Each mode table lists frontmatter keys; a
// string value may be a template with these placeholders:
//
//	{globs}       the rule's globs joined with ","            (whole value: a string)
//	{globs_list}  the rule's globs                            (whole value: a list)
//	{description} the activation description
//	{name}        the rule name
//
// A key whose value resolves to nothing (no globs, no description) is dropped.
// A mode without a table falls back to the always table and is reported as a
// downgrade; an absent always table writes no frontmatter.
type ActivationSpec struct {
	Always map[string]any `toml:"always,omitempty" yaml:"always,omitempty" json:"always,omitempty"`
	Glob   map[string]any `toml:"glob,omitempty" yaml:"glob,omitempty" json:"glob,omitempty"`
	Auto   map[string]any `toml:"auto,omitempty" yaml:"auto,omitempty" json:"auto,omitempty"`
	Manual map[string]any `toml:"manual,omitempty" yaml:"manual,omitempty" json:"manual,omitempty"`
	// Format is "yaml" (default) or "lines": bare "key: value" lines between ---
	// fences, with lists joined by ", " (JetBrains AI Assistant).
	Format string `toml:"format,omitempty" yaml:"format,omitempty" json:"format,omitempty"`
}

// GlobalSpec declares where a tool keeps the user-scope counterparts of the
// project outputs. Every path is relative to the user's home directory and
// slash-separated. `generate --user` maps the outputs of the spec onto these
// paths (see internal/generator/userscope); GlobalPaths exposes them resolved.
type GlobalSpec struct {
	// HomeEnv names an environment variable that relocates the tool's home
	// directory (HERMES_HOME). When it is set, the part of a path under HomeDir
	// is re-rooted at the variable's value.
	HomeEnv string `toml:"home_env,omitempty" yaml:"home_env,omitempty" json:"home_env,omitempty"`
	// HomeDir is the tool's directory under the user's home (".hermes"); required
	// with HomeEnv.
	HomeDir string `toml:"home_dir,omitempty" yaml:"home_dir,omitempty" json:"home_dir,omitempty"`

	RootFile    string `toml:"root_file,omitempty" yaml:"root_file,omitempty" json:"root_file,omitempty"`
	SkillsDir   string `toml:"skills_dir,omitempty" yaml:"skills_dir,omitempty" json:"skills_dir,omitempty"`
	AgentsDir   string `toml:"agents_dir,omitempty" yaml:"agents_dir,omitempty" json:"agents_dir,omitempty"`
	CommandsDir string `toml:"commands_dir,omitempty" yaml:"commands_dir,omitempty" json:"commands_dir,omitempty"`
	RulesDir    string `toml:"rules_dir,omitempty" yaml:"rules_dir,omitempty" json:"rules_dir,omitempty"`

	// SkillReaders lists every user-level skill directory the tool reads, when it
	// reads more than skills_dir (Gemini CLI also reads ~/.agents/skills). A skill
	// of the same name in two of them loads twice, which `generate --user` warns about.
	SkillReaders []string `toml:"skill_readers,omitempty" yaml:"skill_readers,omitempty" json:"skill_readers,omitempty"`
	// SkillPrecedence says which copy runs when a skill of the same name exists at
	// user and project level, as the vendor documents it. Empty means none is documented.
	SkillPrecedence string `toml:"skill_precedence,omitempty" yaml:"skill_precedence,omitempty" json:"skill_precedence,omitempty"`
}

// BodySpec lists the ordered closed-set section renderers composed into a
// per-item file body.
type BodySpec struct {
	Sections []string `toml:"sections" yaml:"sections" json:"sections"`
	// Replace rewrites placeholders in the item's content (a command's
	// $ARGUMENTS becomes Junie's $prompt): every key is replaced by its value.
	Replace map[string]string `toml:"replace,omitempty" yaml:"replace,omitempty" json:"replace,omitempty"`
	// ReplaceFlag is a frontmatter key written true when Replace changed the
	// content, so a tool that must be told a command takes arguments
	// (allowPromptArgument) says so only for the commands that do.
	ReplaceFlag string `toml:"replace_flag,omitempty" yaml:"replace_flag,omitempty" json:"replace_flag,omitempty"`
}

// FrontmatterSpec describes how to build the YAML frontmatter map for a
// per-item file. Keys are emitted in this order: constants → resolved
// effort/model → typed lists (tools/skills) → ordered `fields` → extras
// (alphabetised, filtered by `extras_blacklist`).
type FrontmatterSpec struct {
	Fields    []string       `toml:"fields,omitempty" yaml:"fields,omitempty" json:"fields,omitempty"`
	Constants map[string]any `toml:"constants,omitempty" yaml:"constants,omitempty" json:"constants,omitempty"`
	Tools     bool           `toml:"tools,omitempty" yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills    bool           `toml:"skills,omitempty" yaml:"skills,omitempty" json:"skills,omitempty"`
	// Paths copies a rule's path scope (its `globs`/`paths` frontmatter) into a
	// `paths` frontmatter field, which is how Claude Code scopes a rule file.
	//
	// It applies to skills too: Claude Code and Cursor load a path-gated skill
	// only when files matching the paths are in play.
	Paths         bool `toml:"paths,omitempty" yaml:"paths,omitempty" json:"paths,omitempty"`
	IncludeExtras bool `toml:"include_extras,omitempty" yaml:"include_extras,omitempty" json:"include_extras,omitempty"`
	// HideKey is the frontmatter key that, written false, hides a skill from the
	// tool's slash menu. It is written only when the project opts in with
	// [claude.skills] hide_from_menu, and never over a value the author set.
	HideKey         string   `toml:"hide_key,omitempty" yaml:"hide_key,omitempty" json:"hide_key,omitempty"`
	ExtrasBlacklist []string `toml:"extras_blacklist,omitempty" yaml:"extras_blacklist,omitempty" json:"extras_blacklist,omitempty"`
	EmitEffort      bool     `toml:"emit_effort,omitempty" yaml:"emit_effort,omitempty" json:"emit_effort,omitempty"`
	EmitModel       bool     `toml:"emit_model,omitempty" yaml:"emit_model,omitempty" json:"emit_model,omitempty"`
	// EffortField is the frontmatter key the resolved effort is written under
	// when EmitEffort is true. Empty means "effort"; tools whose thinking knob
	// is spelled differently (pi: "thinking") set it explicitly.
	EffortField string `toml:"effort_field,omitempty" yaml:"effort_field,omitempty" json:"effort_field,omitempty"`
	// OmitName leaves the always-present `name` key out, for tools whose
	// command or prompt files take the name from the filename.
	OmitName bool `toml:"omit_name,omitempty" yaml:"omit_name,omitempty" json:"omit_name,omitempty"`
	// JoinLists writes the tools and skills lists as one comma-separated string
	// ("read, grep") instead of a YAML list, for tools whose frontmatter parser
	// reads only scalars (Letta Code).
	JoinLists bool `toml:"join_lists,omitempty" yaml:"join_lists,omitempty" json:"join_lists,omitempty"`
	// NameFirst writes `name` first and the other keys alphabetically, instead
	// of every key alphabetically (a tool whose files must match another
	// preset's byte for byte).
	NameFirst bool `toml:"name_first,omitempty" yaml:"name_first,omitempty" json:"name_first,omitempty"`
	// QuotedFields lists the frontmatter keys whose string value is always
	// written double-quoted.
	QuotedFields []string `toml:"quoted_fields,omitempty" yaml:"quoted_fields,omitempty" json:"quoted_fields,omitempty"`
	// ToolNames translates the Claude tool names an agent or skill lists (Read,
	// Grep, Bash, ...) into the tool's own names. The match ignores case. A tool
	// the table does not name is dropped, since a name the tool does not know
	// would make it reject the file; an agent left with no tool keeps no `tools`
	// key and so inherits all of them.
	ToolNames map[string]string `toml:"tool_names,omitempty" yaml:"tool_names,omitempty" json:"tool_names,omitempty"`
	// ToolCase is "lower" to write every tool name lower-case (applied after
	// ToolNames); empty keeps the case as written.
	ToolCase string `toml:"tool_case,omitempty" yaml:"tool_case,omitempty" json:"tool_case,omitempty"`
	// ModelAliases translates a resolved model (matched ignoring case) into the
	// tool's own id; a value of "" drops the model. A model that is no key passes
	// through.
	ModelAliases map[string]string `toml:"model_aliases,omitempty" yaml:"model_aliases,omitempty" json:"model_aliases,omitempty"`
	// DropBareAliases drops a model that is one of the bare Claude aliases
	// (sonnet, opus, haiku, inherit), which a tool with its own model namespace
	// cannot take; the agent then inherits the session model. It runs after
	// ModelAliases, so a mapped alias is kept.
	DropBareAliases bool `toml:"drop_bare_aliases,omitempty" yaml:"drop_bare_aliases,omitempty" json:"drop_bare_aliases,omitempty"`
	// Renames writes a listed field under another frontmatter key: with
	// renames = { severity = "severity-default" } a `severity` field is written
	// as `severity-default`.
	Renames map[string]string `toml:"renames,omitempty" yaml:"renames,omitempty" json:"renames,omitempty"`
}

// ToolCaseLower is the FrontmatterSpec.ToolCase value for lower-case tool names.
const ToolCaseLower = "lower"

// EffortMapSpec is the provider's effort tier → native value translation.
// Style is currently always "string"; "budget" (numeric) will be added when
// a numeric-budget preset migrates.
type EffortMapSpec struct {
	Style  string            `toml:"style" yaml:"style" json:"style"`
	Values map[string]string `toml:"values" yaml:"values" json:"values"`
}

// ModelSpec names the frontmatter key under which the resolved model string
// is written. Omit when the provider doesn't emit a per-agent model field.
type ModelSpec struct {
	Field string `toml:"field" yaml:"field" json:"field"`
}

// SidecarSpec is a single conditionally-emitted file (settings.json, .mcp.json,
// etc.). Kind picks the closed-set renderer in sidecars.go.
//
// Object-shaped sidecars are merged into an existing file rather than replacing
// it: ai-rulez owns a fixed set of keys and the consumer owns the rest.
//
// The tool-specific kinds (claude_settings_json, ...) render a fixed document.
// The generic kinds ("mcp", and the reserved "permissions" and "hooks") merge
// into any JSON, JSONC, TOML or YAML document, using Format, Key and Dialect.
type SidecarSpec struct {
	Kind     string `toml:"kind" yaml:"kind" json:"kind"`
	Path     string `toml:"path" yaml:"path" json:"path"`
	EmitWhen string `toml:"emit_when,omitempty" yaml:"emit_when,omitempty" json:"emit_when,omitempty"`
	// Format is the document format of a generic sidecar: json, jsonc, toml or
	// yaml. Empty means the one the path's extension names.
	Format string `toml:"format,omitempty" yaml:"format,omitempty" json:"format,omitempty"`
	// Key is the path of the owned member inside the document. Every element is a
	// literal member name, so ["amp.mcpServers"] is the one flat key
	// "amp.mcpServers" and ["amp", "mcpServers"] is a nested object. Empty means
	// the dialect's default.
	Key []string `toml:"key,omitempty" yaml:"key,omitempty" json:"key,omitempty"`
	// Dialect picks the entry style of a generic "mcp" sidecar (see MCPDialect*).
	// Empty means "standard".
	Dialect string `toml:"dialect,omitempty" yaml:"dialect,omitempty" json:"dialect,omitempty"`
	// GlobalPath is where the sidecar lives in the user scope, relative to the
	// home directory. Empty means the sidecar has no user-scope counterpart.
	GlobalPath string `toml:"global_path,omitempty" yaml:"global_path,omitempty" json:"global_path,omitempty"`
	// UserOnly limits the sidecar to a user-scope run (`--user`); it needs a
	// GlobalPath. It is for a document whose setting the tool reads from the user
	// file only (Zed's agent.tool_permissions), so a project run does not write a
	// file the tool would ignore.
	UserOnly bool `toml:"user_only,omitempty" yaml:"user_only,omitempty" json:"user_only,omitempty"`
	// GlobalMCPPath is where the MCP servers of the sidecar live in the user scope
	// when that is a different file than GlobalPath (Claude Code keeps user-scope
	// MCP servers in ~/.claude.json and its other settings in
	// ~/.claude/settings.json). A user-scope run writes the sidecar's MCP member to
	// this path and everything else to GlobalPath. Empty means the MCP servers go
	// where GlobalPath says.
	GlobalMCPPath string `toml:"global_mcp_path,omitempty" yaml:"global_mcp_path,omitempty" json:"global_mcp_path,omitempty"`
	// Transports (kind "mcp") limits the sidecar to servers with these transports
	// (stdio, http, sse); empty means every server. goose skips a whole plugin
	// document that holds a remote entry, so its sidecar is stdio-only.
	Transports []string `toml:"transports,omitempty" yaml:"transports,omitempty" json:"transports,omitempty"`
	// EnvRefSyntax (kind "mcp") writes a value that came from a ${VAR} placeholder
	// as a reference the tool expands itself instead of the resolved secret:
	// "dollar" is $NAME (Codebuff, Crush), "env_prefix" is ${env:NAME} (Cursor),
	// "braced" is ${NAME} (Factory) and "opencode_env" is {env:NAME} (Kilo).
	EnvRefSyntax string `toml:"env_ref_syntax,omitempty" yaml:"env_ref_syntax,omitempty" json:"env_ref_syntax,omitempty"`
	// Elements (kind "mcp" on a json or jsonc document) also adds values to an
	// array member of the document, such as Kilo's `instructions` globs.
	Elements *ElementsSpec `toml:"elements,omitempty" yaml:"elements,omitempty" json:"elements,omitempty"`
	// Flavor (kind "hook_plugin") is the plugin dialect of the generated module:
	// opencode, opencode-v1, pi or amp (see internal/generator/hookplugins).
	Flavor string `toml:"flavor,omitempty" yaml:"flavor,omitempty" json:"flavor,omitempty"`
}

// Closed-set enum constants. Extending any of these is a deliberate Go change
// paired with a new dispatch branch in render.go / sidecars.go.
const (
	// outputs.<type>.mode
	OutputModePerItemFile = "per_item_file"
	// OutputModeAggregate renders every item into the single file named by
	// outputs.<type>.file; only the checks type takes it.
	OutputModeAggregate = "aggregate"

	// outputs.rules.dialect for an activation-mapped rules folder
	DialectMapped = "mapped"

	// outputs.rules.activation.format
	ActivationFormatYAML  = "yaml"
	ActivationFormatLines = "lines"

	// outputs.<type>.filter
	FilterIncludeIfTargetingProvider = "include_if_targeting_provider"
	FilterPathScoped                 = "path_scoped"
	// FilterPlacementCore keeps a skill or command generated into the
	// provider's own directory: it drops items that [placement] or the
	// `placement` frontmatter ships only through a plugin, and applies `targets`.
	FilterPlacementCore = "placement_core"

	// outputs.rules.inline_filter
	InlineFilterPathScoped = "path_scoped"

	// root.sections
	SectionRootHeader           = "header"
	SectionRootTitle            = "title"
	SectionRootDescription      = "description"
	SectionRootRulesInline      = "rules_inline"
	SectionRootContextInline    = "context_inline"
	SectionRootAgentsDelegation = "agents_delegation"
	// SectionRootAgentsMDImport is not accepted in a spec: the renderer
	// substitutes it for the declared sections when agents_md turns the root
	// file into an "@AGENTS.md" shim.
	SectionRootAgentsMDImport = "agents_md_import"

	// outputs.<type>.body.sections
	SectionBodyFrontmatter     = "frontmatter"
	SectionBodyContent         = "content"
	SectionBodyResourceIndex   = "resource_index"
	SectionBodyTargetedRules   = "targeted_rules"
	SectionBodyTargetedContext = "targeted_context"

	// effort_map.style
	EffortMapStyleString = "string"

	// sidecars[].emit_when
	PredicateAlways            = "always"
	PredicateHasMCPServers     = "has_mcp_servers"
	PredicateHasMCPJSONEntries = "has_mcp_json_entries"
	PredicateHasPlugins        = "has_plugins"
	PredicateHasResolvedEffort = "has_resolved_effort"
	// PredicateHasMCPServersOrPluginSettings holds when the config has MCP
	// servers or manages the plugin keys of .claude/settings.json.
	PredicateHasMCPServersOrPluginSettings = "has_mcp_servers_or_plugin_settings"
	// PredicateHasResolvedEffortOrMCPServers holds when the config has MCP servers
	// or a resolved global effort (.amp/settings.json carries both).
	PredicateHasResolvedEffortOrMCPServers = "has_resolved_effort_or_mcp_servers"
	// PredicateHasClaudeSettings holds when the config manages the plugin keys of
	// .claude/settings.json, or declares [[hooks]], [permissions] or
	// [claude.settings.managed]. MCP servers do not count: Claude Code reads them
	// from .mcp.json.
	PredicateHasClaudeSettings = "has_claude_settings"

	// PredicateHasHooks holds when the config declares top-level [[hooks]].
	PredicateHasHooks = "has_hooks"

	// PredicateHasPermissions holds when the config declares a top-level
	// [permissions] block with at least one rule.
	PredicateHasPermissions = "has_permissions"

	// sidecars[].kind
	SidecarClaudeSettingsJSON = "claude_settings_json"
	SidecarClaudePluginsJSON  = "claude_plugins_json"
	SidecarMCPJSON            = "mcp_json"
	SidecarAmpSettingsJSON    = "amp_settings_json"
	SidecarPiMCPJSON          = "pi_mcp_json"
	// Generic sidecar kinds. SidecarMCP is implemented; the other two are
	// accepted by validation and reserved for later renderers.
	SidecarMCP = "mcp"
	// SidecarChecks merges code-review checks into a YAML review-guideline
	// document (dialects augment and gitlab-duo).
	SidecarChecks      = "checks"
	SidecarPermissions = "permissions"
	SidecarHooks       = "hooks"
	// SidecarHookPlugin renders [[hooks]] as a JavaScript or TypeScript plugin
	// module that ai-rulez owns wholly, for harnesses whose hooks are code (OpenCode
	// and its forks, Pi, Amp). The flavor picks the plugin dialect.
	SidecarHookPlugin = "hook_plugin"

	// sidecars[].format
	DocFormatJSON  = "json"
	DocFormatJSONC = "jsonc"
	DocFormatTOML  = "toml"
	DocFormatYAML  = "yaml"

	// Content type keys in ProviderSpec.Outputs
	OutputTypeRules    = "rules"
	OutputTypeSkills   = "skills"
	OutputTypeAgents   = "agents"
	OutputTypeCommands = "commands"
	OutputTypeChecks   = "checks"
)
