package main

import (
	"github.com/Goldziher/ai-rulez/cmd/commands"
)

// Version is set during build
var Version = "dev"

func main() {
	commands.Version = Version

	commands.Execute()
}
