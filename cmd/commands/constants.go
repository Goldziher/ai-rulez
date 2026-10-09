package commands

// Cobra command "Use" strings shared across subcommands.
const (
	cmdUseList       = "list"
	cmdUseVersion    = "version"
	cmdUseRemoveName = "remove <name>"
)

// JSON output keys reused across CLI command outputs.
const (
	keyName    = "name"
	keyPath    = "path"
	keyType    = "type"
	keySource  = "source"
	keyDesc    = "description"
	keySuccess = "success"
	keyBundle  = "bundle"
	keyStatus  = "status"
	keyKind    = "kind"
	keySigner  = "signer"
	keySubject = "subject"
	kindLock   = "lock"
)

// Values that several commands compare against or print.
const (
	valueTrue     = "true"
	statusDrift   = "drift"
	valueNone     = "none"
	answerYes     = "yes"
	failOnError   = "error"
	failOnWarning = "warning"
	failOnInfo    = "info"
	kindSkill     = "skill"
	kindCommand   = "command"
	presetClaude  = "claude"
	labelChanged  = "changed"
)

// Config file base names supported by the CLI.
const (
	configFileTOML = "config.toml"
)

// flagRole is the --role flag of generate, tokens, usage and mcp --serve-skills.
// It always names a role of the project's [[roles]] (see `ai-rulez roles list`).
const flagRole = "role"
