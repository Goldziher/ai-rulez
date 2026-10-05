package handlers

// JSON output keys used across MCP tool responses.
const (
	keySuccess   = "success"
	keyOperation = "operation"
	keyName      = "name"
	keyMessage   = "message"
	keyDomain    = "domain"
	keyPath      = "path"
	keyCount     = "count"
	keyContent   = "content"
	keySource    = "source"
	keyValid     = "valid"
	keyConfig    = "config"

	keyDescription = "description"
	keyBuiltins    = "builtins"
	keyGitignore   = "gitignore"
	keyUpdated     = "updated"
	keyDefaults    = "defaults"
	keyRules       = "rules"
	opUpdateConfig = "update_config"
)

// Preset name constants used in MCP handlers.
const (
	presetAmp     = "amp"
	presetClaude  = "claude"
	presetCursor  = "cursor"
	presetDevin   = "devin"
	presetCopilot = "copilot"
	presetGemini  = "gemini"
	presetCodex   = "codex"
	presetCline   = "cline"
)
