package commands

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

const formatCycloneDX = "cyclonedx"

var (
	sbomFormat string
	sbomOutput string
	sbomOnline bool
)

// SBOMCmd prints the project's AI configuration as a CycloneDX 1.6 bill of materials.
var SBOMCmd = &cobra.Command{
	Use:   "sbom",
	Short: "Print a CycloneDX software bill of materials of the AI configuration",
	Long: `Print a CycloneDX 1.6 JSON bill of materials of the project's AI configuration:
the authored rules, context, skills, agents, commands, checks, hooks and roles,
the remote includes and skill sources (with their pinned commit), and the MCP
servers (a package URL is guessed from npx, uvx, docker run and go run commands;
remote servers are listed as services).

The document has no timestamp and is byte-identical across runs, operating
systems and line endings. ai-rulez digests appear only in "ai-rulez:" properties,
never in "hashes". Environment and header values are never read, and URLs lose
their credentials and query. The serial number is derived from the lock tree
(ai-rulez.lock) or, without a lock, from the tree computed from the sources.

By default sbom does not touch the network: remote includes and skill sources
come from ai-rulez.lock and the local cache (run "ai-rulez generate" or
"ai-rulez lock" once to fill it). --online lets it contact the remotes, as
generate does, to resolve moving refs.

The machine-local overlay (config.local.*, local/) is never included. Nothing is
rendered and nothing is written unless --output is given.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		exitOn(runSBOM(cmd.OutOrStdout()))
	},
}

func init() {
	SBOMCmd.Flags().StringVar(&sbomFormat, "format", formatCycloneDX, "Output format: cyclonedx (CycloneDX 1.6 JSON)")
	SBOMCmd.Flags().BoolVar(&sbomOnline, "online", false, "Allow contacting remote includes and skill sources (git ls-remote); by default only the lock and the cache are used")
	SBOMCmd.Flags().StringVarP(&sbomOutput, "output", "o", "", "Write the document to this file instead of stdout")
	SBOMCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func runSBOM(out io.Writer) error {
	if sbomFormat != formatCycloneDX {
		return oops.Errorf("unknown --format %q (use %s)", sbomFormat, formatCycloneDX)
	}
	ctx := context.Background()
	if !sbomOnline {
		ctx = config.WithOfflineIncludes(ctx)
	}
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutLocal())
	if err != nil {
		if !sbomOnline {
			return oops.Hint("sbom reads remote sources from the lock and the cache only; run `ai-rulez generate` or `ai-rulez lock` to fill the cache, or pass --online").Wrap(err)
		}
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	bom, err := sbom.Build(cfg, Version)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if sbomOutput == "" {
		return sbom.Write(out, bom)
	}
	var buf bytes.Buffer
	if err := sbom.Write(&buf, bom); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if err := os.WriteFile(sbomOutput, buf.Bytes(), 0o644); err != nil { //nolint:gosec // an SBOM is meant to be shared
		return oops.With("path", sbomOutput).Wrapf(err, "write sbom")
	}
	return nil
}
