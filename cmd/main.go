package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/cmd/commands"
	_ "github.com/Goldziher/ai-rulez/internal/includes" // Register includes resolver callback
)

var version = "dev"

func main() {
	commands.Version = version

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
