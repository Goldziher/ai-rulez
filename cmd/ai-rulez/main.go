package main

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

var version = "5.13.3"

func main() {
	os.Exit(run())
}

func run() int {
	if err := watchNPMParent(); err != nil {
		logger.LogError("Starting npm process monitor", err)
		return 1
	}

	commands.Version = version
	schema.Version = version

	// The Sigstore backend checks signatures; internal/signing does not link it itself.
	signing.UseBackend(sigstore.New())

	// Main renders a failure once and returns the exit code of the contract.
	return commands.Main()
}
