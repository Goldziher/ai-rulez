// Command docsllms generates the docs site's llms.txt and llms-full.txt from
// zensical.toml and the docs sources. The files are checked in under docs/, so
// the site build serves them at /llms.txt and /llms-full.txt, and --check fails
// when they differ from what the sources produce.
//
//	go run ./scripts/docsllms            # write docs/llms.txt and docs/llms-full.txt
//	go run ./scripts/docsllms --check    # exit 1 if they are stale or invalid
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docsllms:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "zensical.toml", "site configuration")
	docsDir := flag.String("docs", "docs", "docs source directory; the files are written here")
	check := flag.Bool("check", false, "verify the files instead of writing them")
	flag.Parse()

	raw, err := os.ReadFile(*configPath)
	if err != nil {
		return err
	}
	site, err := llmstxt.ParseSite(raw)
	if err != nil {
		return err
	}
	index, full, err := llmstxt.BuildDocs(site, os.DirFS(*docsDir))
	if err != nil {
		return err
	}
	if findings := llmstxt.Validate([]byte(index)); len(findings) > 0 {
		for _, f := range findings {
			fmt.Fprintf(os.Stderr, "llms.txt:%d: %s %s: %s\n", f.Line, f.Code, f.Name, f.Message)
		}
		return fmt.Errorf("the generated llms.txt does not match the llms.txt format")
	}
	outputs := []struct{ name, content string }{{"llms.txt", index}, {llmstxt.FullFileName, full}}
	stale := false
	for _, out := range outputs {
		path := filepath.Join(*docsDir, out.name)
		if !*check {
			if err := os.WriteFile(path, []byte(out.content), 0o644); err != nil { //nolint:gosec // published docs file
				return err
			}
			continue
		}
		have, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(have, []byte(out.content)) {
			fmt.Fprintf(os.Stderr, "%s is out of date\n", path)
			stale = true
		}
	}
	if stale {
		return fmt.Errorf("run `task docs:llms` and commit the result")
	}
	return nil
}
