package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

var version = "5.0.0"

func main() {
	commands.Version = version
	schema.Version = version

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
