package main

import (
	"github.com/Goldziher/ai-rulez/cmd/commands"
)

var version = "dev"

func main() {
	commands.Version = version
	commands.Execute()
}
