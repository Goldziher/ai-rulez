package presets

// Shared string constants used across preset generators.
//
// These are kept in one place so that goconst stays quiet and so the
// underlying values can be updated in lockstep across presets.

// Preset names that don't already have a dedicated constant in their preset
// file. The remaining preset names live next to the generator that owns them
// (e.g. presetNameClaude in claude.go, codexPresetName in codex.go).
const (
	presetNameAmp         = "amp"
	presetNameAntigravity = "antigravity"
	presetNameJunie       = "junie"
)

// JSON / YAML object keys used when rendering settings, MCP, and frontmatter
// payloads. These are deliberately untyped strings (not field names of a Go
// struct) because the rendered maps are map[string]interface{}.
const (
	keyName        = "name"
	keyDescription = "description"
	keyModel       = "model"
	keyCommand     = "command"
	keyDisabled    = "disabled"
	keyMCPServers  = "mcpServers"
	keyMCP         = "mcp"
	keyTemperature = "temperature"
	keyPrompt      = "prompt"
	keyKind        = "kind"

	// Template field names used by the custom preset (capitalized for Go
	// template field-access semantics).
	keyContentField = "Content"
	keyNameField    = "Name"
)

// Output filenames shared across multiple presets.
const (
	fileClaudeMD = "CLAUDE.md"
)

// Command names used when generating shared MCP server entries.
const (
	cmdNPX = "npx"
)
