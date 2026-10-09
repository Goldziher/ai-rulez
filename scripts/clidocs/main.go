// Command clidocs generates the command reference of the docs from the Cobra
// command tree: docs/cli-reference.md in full, and the command index between the
// markers in docs/cli.md. --check fails when either differs from the tree.
//
//	go run ./scripts/clidocs           # write both
//	go run ./scripts/clidocs --check   # exit 1 if stale
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/Goldziher/ai-rulez/v5/internal/clidocs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "clidocs:", err)
		os.Exit(1)
	}
}

func run() error {
	docsDir := flag.String("docs", "docs", "docs source directory")
	check := flag.Bool("check", false, "verify the files instead of writing them")
	flag.Parse()

	root := commands.CommandTree()
	cliPath := filepath.Join(*docsDir, "cli.md")
	page, err := os.ReadFile(cliPath)
	if err != nil {
		return err
	}
	cli, err := clidocs.ApplyIndex(string(page), clidocs.Index(root))
	if err != nil {
		return fmt.Errorf("%s: %w", cliPath, err)
	}
	outputs := []struct{ path, content string }{
		{cliPath, cli},
		{filepath.Join(*docsDir, "cli-reference.md"), clidocs.Reference(root)},
	}
	stale := false
	for _, out := range outputs {
		have, readErr := os.ReadFile(out.path)
		if string(have) == out.content && readErr == nil {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "%s is out of date\n", out.path)
			stale = true
			continue
		}
		if err := os.WriteFile(out.path, []byte(out.content), 0o644); err != nil { //nolint:gosec // published docs file
			return err
		}
	}
	if stale {
		return fmt.Errorf("run `task docs:cli` and commit the result")
	}
	return nil
}
