package config

// ListingSpec declares what an agent harness puts in the prompt at session
// start to advertise the skills, commands and agents it can load. A harness
// that lists an item pays for its name and description (plus per-entry framing)
// on every request, whether or not the item is ever used.
//
// Builtin providers described by the provider DSL declare it in their spec
// (`[listing]`); the hand-written presets are covered by builtinListings below.
// A zero ListingSpec lists nothing.
type ListingSpec struct {
	// Skills, Commands and Agents say which generated item kinds the harness
	// lists. Claude Code merges commands into the skill listing and lists agents
	// in the Agent tool description, so it sets all three.
	Skills   bool `toml:"skills,omitempty" yaml:"skills,omitempty" json:"skills,omitempty"`
	Commands bool `toml:"commands,omitempty" yaml:"commands,omitempty" json:"commands,omitempty"`
	Agents   bool `toml:"agents,omitempty" yaml:"agents,omitempty" json:"agents,omitempty"`
	// IncludePath is true when each listing entry also carries the item's file
	// path (Codex and pi do).
	IncludePath bool `toml:"include_path,omitempty" yaml:"include_path,omitempty" json:"include_path,omitempty"`
	// DescriptionLimit is the harness's per-entry description truncation limit in
	// characters. Zero means none is known.
	DescriptionLimit int `toml:"description_limit,omitempty" yaml:"description_limit,omitempty" json:"description_limit,omitempty"`
}

// Lists reports whether the harness lists items of the given output kind.
func (s ListingSpec) Lists(kind OutputKind) bool {
	switch kind {
	case OutputKindSkill:
		return s.Skills
	case OutputKindCommand:
		return s.Commands
	case OutputKindAgent:
		return s.Agents
	default:
		return false
	}
}

// Any reports whether the harness lists at least one item kind.
func (s ListingSpec) Any() bool { return s.Skills || s.Commands || s.Agents }

// ClaudeSkillDescriptionLimit is the documented cap on the combined
// description text of one Claude Code skill listing entry.
const ClaudeSkillDescriptionLimit = 1536

// builtinListings covers the hand-written presets. Each entry cites how the
// harness documents (or, where marked, merely implies) a listing:
//
//   - codex: name, description and file path, capped to a share of the context
//     window (developers.openai.com/codex/skills).
//   - gemini: "injects the name and description of all enabled skills into the
//     system prompt" (geminicli.com/docs/cli/skills).
//   - opencode: names and descriptions in the skill tool description
//     (opencode.ai/docs/skills).
//   - devin: "only name + description until invoked" (Cascade skills docs).
//   - cline: name and description loaded at startup (docs.cline.bot skills).
//   - cursor, copilot: the docs describe discovery by name and description
//     without saying it is a per-request listing; modeled as listed.
//
// Presets absent from this table (amp, antigravity, baz, continue-dev, hermes,
// xum) are not modeled, so their listing cost is reported as zero rather than
// guessed.
var builtinListings = map[string]ListingSpec{
	"codex":    {Skills: true, IncludePath: true},
	"gemini":   {Skills: true},
	"opencode": {Skills: true},
	"devin":    {Skills: true},
	"cline":    {Skills: true},
	"cursor":   {Skills: true},
	"copilot":  {Skills: true},
}

// BuiltinListing returns the listing model of a hand-written preset.
func BuiltinListing(preset string) (ListingSpec, bool) {
	spec, ok := builtinListings[preset]
	return spec, ok
}
