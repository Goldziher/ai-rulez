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
// slash-separated. It is only parsed, validated and exposed (GlobalPaths); the
// generator does not write user-scope files yet.
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
}

// BodySpec lists the ordered closed-set section renderers composed into a
// per-item file body.
type BodySpec struct {
	Sections []string `toml:"sections" yaml:"sections" json:"sections"`
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
	Paths           bool     `toml:"paths,omitempty" yaml:"paths,omitempty" json:"paths,omitempty"`
	IncludeExtras   bool     `toml:"include_extras,omitempty" yaml:"include_extras,omitempty" json:"include_extras,omitempty"`
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
}

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
	// GlobalMCPPath is where the MCP servers of the sidecar live in the user scope
	// when that is a different file than GlobalPath (Claude Code keeps user-scope
	// MCP servers in ~/.claude.json and its other settings in
	// ~/.claude/settings.json). A user-scope run writes the sidecar's MCP member to
	// this path and everything else to GlobalPath. Empty means the MCP servers go
	// where GlobalPath says.
	GlobalMCPPath string `toml:"global_mcp_path,omitempty" yaml:"global_mcp_path,omitempty" json:"global_mcp_path,omitempty"`
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

	// sidecars[].kind
	SidecarClaudeSettingsJSON = "claude_settings_json"
	SidecarClaudePluginsJSON  = "claude_plugins_json"
	SidecarMCPJSON            = "mcp_json"
	SidecarAmpSettingsJSON    = "amp_settings_json"
	SidecarPiMCPJSON          = "pi_mcp_json"
	// Generic sidecar kinds. SidecarMCP is implemented; the other two are
	// accepted by validation and reserved for later renderers.
	SidecarMCP         = "mcp"
	SidecarPermissions = "permissions"
	SidecarHooks       = "hooks"

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
