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
)

// Preset name constants used in MCP handlers.
const (
	presetAmp         = "amp"
	presetClaude      = "claude"
	presetCursor      = "cursor"
	presetWindsurf    = "windsurf"
	presetCopilot     = "copilot"
	presetGemini      = "gemini"
	presetCodex       = "codex"
	presetCline       = "cline"
	presetContinueDev = "continue-dev"
)
