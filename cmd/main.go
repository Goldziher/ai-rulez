package main

import (
	"runtime/debug"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func main() {
	version := "dev"
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		version = buildInfo.Main.Version
	}
	commands.Version = version

	commands.Execute()
}
