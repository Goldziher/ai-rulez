package main

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

var version = "5.0.0"

func main() {
	commands.Version = version
	schema.Version = version

	// The Sigstore backend checks signatures; internal/signing does not link it itself.
	signing.UseBackend(sigstore.New())

	// Main renders a failure once and returns the exit code of the contract; this
	// is the only os.Exit in the CLI.
	os.Exit(commands.Main())
}
