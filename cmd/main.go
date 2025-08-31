package main

import (
	"github.com/Goldziher/ai-rulez/cmd/commands"
)

// Version is set during build
var Version = "dev"

func main() {
	// Set version for commands
	commands.Version = Version

	// Execute root command
	commands.Execute()
}
