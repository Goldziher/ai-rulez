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
}

// RootSpec declares the top-level instructions file (CLAUDE.md, AGENTS.md, ...).
type RootSpec struct {
	File     string   `toml:"file" yaml:"file" json:"file"`
	Sections []string `toml:"sections" yaml:"sections" json:"sections"`
}

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
	Fields          []string       `toml:"fields,omitempty" yaml:"fields,omitempty" json:"fields,omitempty"`
	Constants       map[string]any `toml:"constants,omitempty" yaml:"constants,omitempty" json:"constants,omitempty"`
	Tools           bool           `toml:"tools,omitempty" yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills          bool           `toml:"skills,omitempty" yaml:"skills,omitempty" json:"skills,omitempty"`
	IncludeExtras   bool           `toml:"include_extras,omitempty" yaml:"include_extras,omitempty" json:"include_extras,omitempty"`
	ExtrasBlacklist []string       `toml:"extras_blacklist,omitempty" yaml:"extras_blacklist,omitempty" json:"extras_blacklist,omitempty"`
	EmitEffort      bool           `toml:"emit_effort,omitempty" yaml:"emit_effort,omitempty" json:"emit_effort,omitempty"`
	EmitModel       bool           `toml:"emit_model,omitempty" yaml:"emit_model,omitempty" json:"emit_model,omitempty"`
}

// EffortMapSpec is the provider's effort tier → native value translation.
// Style is currently always "string"; "budget" (numeric) will be added when
// the continue-dev preset migrates.
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
type SidecarSpec struct {
	Kind     string `toml:"kind" yaml:"kind" json:"kind"`
	Path     string `toml:"path" yaml:"path" json:"path"`
	EmitWhen string `toml:"emit_when,omitempty" yaml:"emit_when,omitempty" json:"emit_when,omitempty"`
}

// Closed-set enum constants. Extending any of these is a deliberate Go change
// paired with a new dispatch branch in render.go / sidecars.go.
const (
	// outputs.<type>.mode
	OutputModePerItemFile = "per_item_file"

	// outputs.<type>.filter
	FilterIncludeIfTargetingProvider = "include_if_targeting_provider"

	// root.sections
	SectionRootHeader           = "header"
	SectionRootTitle            = "title"
	SectionRootDescription      = "description"
	SectionRootRulesInline      = "rules_inline"
	SectionRootContextInline    = "context_inline"
	SectionRootAgentsDelegation = "agents_delegation"

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
	PredicateHasPlugins        = "has_plugins"
	PredicateHasResolvedEffort = "has_resolved_effort"

	// sidecars[].kind
	SidecarClaudeSettingsJSON = "claude_settings_json"
	SidecarClaudePluginsJSON  = "claude_plugins_json"
	SidecarMCPJSON            = "mcp_json"
	SidecarAmpSettingsJSON    = "amp_settings_json"

	// Content type keys in ProviderSpec.Outputs
	OutputTypeSkills   = "skills"
	OutputTypeAgents   = "agents"
	OutputTypeCommands = "commands"
)
