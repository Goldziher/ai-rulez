package commands

// Cobra command "Use" strings shared across subcommands.
const (
	cmdUseList       = "list"
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
)

// Config file base names supported by the CLI.
const (
	configFileTOML = "config.toml"
	configFileYAML = "config.yaml"
	configFileJSON = "config.json"
)

// flagRole is the --role flag of generate, tokens, usage and mcp --serve-skills.
// It always names a role of the project's [[roles]] (see `ai-rulez roles list`).
const flagRole = "role"
