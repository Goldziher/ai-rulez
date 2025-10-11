package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

var version = "dev"

func main() {
	commands.Version = version

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
