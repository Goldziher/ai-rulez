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
	keySuccess = "success"
)

// Config file base names supported by the CLI.
const (
	configFileTOML = "config.toml"
	configFileYAML = "config.yaml"
	configFileJSON = "config.json"
)
