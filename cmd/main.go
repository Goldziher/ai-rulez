package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/cmd/commands"
	_ "github.com/Goldziher/ai-rulez/internal/includes" // Register includes resolver callback
	"github.com/Goldziher/ai-rulez/schema"
)

var version = "4.10.0"

func main() {
	commands.Version = version
	schema.Version = version

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
