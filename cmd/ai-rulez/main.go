package main

import (
	"fmt"
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

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, commands.FormatError(err))
		os.Exit(1)
	}
}
