// Command obscure decodes an obscured data file to standard output for editing,
// or encodes an edited plain file back:
//
//	go run ./scripts/obscure decode internal/lint/ar_malware_rules.dat > /tmp/rules.toml
//	go run ./scripts/obscure encode /tmp/rules.toml > internal/lint/ar_malware_rules.dat
//
// Keep the plain file outside the repository: it holds the payload strings that
// anti-malware scanners report.
package main

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/obscure"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "obscure:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 || args[0] != "encode" && args[0] != "decode" {
		return fmt.Errorf("usage: obscure encode|decode FILE")
	}
	in, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	out := obscure.Encode(in)
	if args[0] == "decode" {
		if out, err = obscure.Decode(in); err != nil {
			return err
		}
	}
	_, err = os.Stdout.Write(out)
	return err
}
