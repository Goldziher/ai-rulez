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
	keyType            = "type"
	keyUserInvocable   = "user-invocable"
	keyName            = "name"
	keyLicense         = "license"
	keyCommands        = "commands"
	keyDisableModel    = "disable-model-invocation"
	keyDisallowedTools = "disallowed-tools"

	cmdDocker = "docker"
	cmdCurl   = "curl"
	cmdWget   = "wget"
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

// Names the rule files share beyond the keys above: packages, interpreters,
// severity levels and a few fixed phrases.
const (
	presetClaude = "claude"
	unitBytes    = "bytes"

	toolBun    = "bun"
	toolPnpm   = "pnpm"
	toolYarn   = "yarn"
	toolNpx    = "npx"
	toolUvx    = "uvx"
	toolPipx   = "pipx"
	toolPython = "python"

	shellBash = "bash"
	shellZsh  = "zsh"
	cmdTime   = "time"

	osWindows      = "windows"
	transportStdio = "stdio"
	keyTargets     = "targets"

	fenceFrontmatter   = "---"
	whyNoVersionPinned = "no version is pinned"

	boolTrue  = "true"
	boolFalse = "false"

	levelError   = "error"
	levelWarning = "warning"
	levelOff     = "off"
)
