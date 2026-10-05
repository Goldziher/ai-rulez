package lint

// Strings the rule files share: frontmatter and JSON keys, command names,
// hook handler types and the units of the load-budget table.
const (
	keyAgents          = "agents"
	keyAllowedTools    = "allowed-tools"
	keyAlwaysApply     = "alwaysApply"
	keyArgumentHint    = "argument-hint"
	keyBackground      = "background"
	keyCategory        = "category"
	keyDescription     = "description"
	keyEnv             = "env"
	keyHooks           = "hooks"
	keyKeywords        = "keywords"
	keyMCPServers      = "mcpServers"
	keyMetadata        = "metadata"
	keyUserInvocable   = "user-invocable"
	keyName            = "name"
	keyLicense         = "license"
	keyCommands        = "commands"
	keyDisableModel    = "disable-model-invocation"
	keyDisallowedTools = "disallowed-tools"

	cmdDocker = "docker"
	cmdNPM    = "npm"
	cmdExec   = "exec"
	cmdPip    = "pip"
	cmdSudo   = "sudo"

	hookTypeAgent   = "agent"
	hookTypeCommand = "command"
	hookTypePrompt  = "prompt"
	transportHTTP   = "http"

	kindShape = "shape"

	presetCursor = "cursor"
	presetCodex  = "codex"
	scopeChain   = "chain"
	scopeFile    = "file"
	unitChars    = "chars"
	unitLines    = "lines"

	wordAnd     = "and"
	wordInstall = "install"
	wordRun     = "run"
)
