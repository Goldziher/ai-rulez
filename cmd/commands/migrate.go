package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/migrate"
)

var (
	migrateDryRun        bool
	migrateCheck         bool
	migrateAdoptDefaults bool
	migrateWrite         bool
	migrateRecursive     bool
	migrateFormat        string
)

// MigrateCmd groups the migrations: one subcommand per target.
var MigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate a 4.x configuration to the 5.0 format, or a content tree to OKF",
	Long: `Migrate a project: "migrate v5" rewrites a 4.x configuration for ai-rulez 5.0,
"migrate okf" converts the content tree to an OKF bundle.

Exit codes: 0 migrated or nothing to do, 1 a project could not be migrated,
2 --check found a project that still needs migration.`,
	Example: `  ai-rulez migrate v5 --dry-run
  ai-rulez migrate v5 --recursive --check --format json
  ai-rulez migrate okf --dry-run`,
}

func newMigrateV5Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "v5",
		Short: "Migrate a 4.x configuration to the 5.0 format",
		Long: `Rewrite a 4.x project for ai-rulez 5.0.

Only 4.x is migrated. A 2.x or 3.x project must first be migrated to 4.0 with
ai-rulez 4.x.

The rewrite keeps config.toml comments and order. YAML and JSON configs are
converted to config.toml, legacy mcp.* files are merged into it, [lint.budget]
becomes [lint.ratchet], and the 4.x defaults v5 changes (agents_md, the managed
.gitignore block, Source-Hash headers) are pinned so the output does not move,
unless --adopt-defaults is given.

Exit codes: 0 migrated or nothing to do, 1 a project could not be migrated,
2 --check found a project that still needs migration.`,
		Example: `  ai-rulez migrate v5 --dry-run
  ai-rulez migrate v5
  ai-rulez migrate v5 --recursive --check --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return exitStatus(runMigrateV5(cmd.OutOrStdout()))
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&migrateAdoptDefaults, "adopt-defaults", false, "take the v5 defaults instead of pinning the 4.x ones")
	flags.BoolVar(&migrateWrite, "write", false, "also rewrite frontmatter aliases in .ai-rulez markdown files")
	specRecursive.Bool(flags, &migrateRecursive, "migrate every project found below the current directory")
	return cmd
}

func newMigrateOKFCmd() *cobra.Command {
	return &cobra.Command{
		Use:   migrateOKFTarget,
		Short: "Convert the .ai-rulez/ content tree to an OKF bundle, in place",
		Long: `Convert the .ai-rulez/ content tree, in place, to an OKF bundle: each file gains
type, title and x-ai-rulez frontmatter (its former frontmatter moves under
x-ai-rulez.metadata) and every directory gets an index.md. Bodies are kept byte
for byte and the generated output does not change. It is idempotent; --dry-run
and --check write nothing.`,
		Example: `  ai-rulez migrate okf --dry-run
  ai-rulez migrate okf --check --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return exitStatus(runMigrateOKFChecked(cmd.OutOrStdout()))
		},
	}
}

func init() {
	MigrateCmd.AddCommand(newMigrateV5Cmd(), newMigrateOKFCmd())
	flags := MigrateCmd.PersistentFlags()
	specDryRun.Bool(flags, &migrateDryRun, "show the change list without writing anything")
	flags.BoolVar(&migrateCheck, "check", false, "write nothing and exit 2 when a project still needs migration")
	addFormatFlag(flags, &migrateFormat, formatText, formatText, formatText, formatJSON)
}

// runMigrateV5 migrates the 4.x project(s) below the working directory and
// returns the process exit code. --config-dir (global) names the directory.
func runMigrateV5(out io.Writer) int {
	if migrateFormat != formatText && migrateFormat != formatJSON {
		renderStderr(fmt.Errorf("unsupported --format %q: use text or json", migrateFormat))
		return 1
	}

	report, err := migrate.Run(migrate.Options{
		Root:          ".",
		ConfigDirName: configDir,
		Recursive:     migrateRecursive,
		DryRun:        migrateDryRun,
		Check:         migrateCheck,
		AdoptDefaults: migrateAdoptDefaults,
		Write:         migrateWrite,
	})
	if err != nil {
		renderStderr(err)
		return 1
	}

	if migrateFormat == formatJSON {
		if err := writeRawJSON(out, report); err != nil {
			renderStderr(err)
			return 1
		}
	} else {
		printMigrateReport(out, report)
	}

	switch {
	case report.Failed():
		return 1
	case migrateCheck && report.Pending():
		return exitDrift
	}
	return 0
}

func runMigrateOKFChecked(out io.Writer) int {
	if migrateFormat != formatText && migrateFormat != formatJSON {
		renderStderr(fmt.Errorf("unsupported --format %q: use text or json", migrateFormat))
		return 1
	}
	return runMigrateOKF(cmdContext(), out)
}

func printMigrateReport(out io.Writer, r *migrate.Report) {
	w := reportWriter{out}
	for _, p := range r.Projects {
		w.printf("%s (%s): %s\n", p.Path, p.ConfigDir, p.Status)
		for _, c := range p.Changes {
			w.printf("  [%s] %s: %s\n", c.Rule, c.File, c.Detail)
		}
		for _, warning := range p.Warnings {
			w.printf("  warning: %s\n", warning)
		}
		if p.Error != "" {
			w.printf("  error: %s\n", p.Error)
		}
	}
	verb := "migrated"
	if r.DryRun || r.Check {
		verb = "would migrate"
	}
	w.printf("%s %d, unchanged %d, errors %d\n", verb, r.Summary.Migrated, r.Summary.Unchanged, r.Summary.Errors)
}
