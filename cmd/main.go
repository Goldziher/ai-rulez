package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	_ "github.com/Goldziher/ai-rulez/v5/internal/includes" // Register includes resolver callback
	"github.com/Goldziher/ai-rulez/v5/schema"
)

var version = "4.24.2"

func main() {
	commands.Version = version
	schema.Version = version

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
