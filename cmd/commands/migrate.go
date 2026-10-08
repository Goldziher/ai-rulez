package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

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
	migrateConfigDir     string
)

// MigrateCmd migrates a 4.x project to the 5.0 format.
var MigrateCmd = &cobra.Command{
	Use:   "migrate v5",
	Short: "Migrate a 4.x configuration to the 5.0 format, or a content tree to OKF",
	Long: `Rewrite a 4.x project for ai-rulez 5.0.

Only 4.x is migrated. A 2.x or 3.x project must first be migrated to 4.0 with
ai-rulez 4.x.

The rewrite keeps config.toml comments and order. YAML and JSON configs are
converted to config.toml, legacy mcp.* files are merged into it, [lint.budget]
becomes [lint.ratchet], and the 4.x defaults v5 changes (agents_md, the managed
.gitignore block, Source-Hash headers) are pinned so the output does not move,
unless --adopt-defaults is given.

Exit codes: 0 migrated or nothing to do, 1 a project could not be migrated,
2 --check found a project that still needs migration.

"migrate okf" converts the .ai-rulez/ content tree, in place, to an OKF bundle:
each file gains type, title and x-ai-rulez frontmatter (its former frontmatter
moves under x-ai-rulez.metadata) and every directory gets an index.md. Bodies
are kept byte for byte and the generated output does not change. It is
idempotent; --dry-run and --check write nothing.`,
	Example: `  ai-rulez migrate okf --dry-run
  ai-rulez migrate v5 --dry-run
  ai-rulez migrate v5
  ai-rulez migrate v5 --recursive --check --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(runMigrate(cmd.OutOrStdout(), args[0]))
	},
}

func init() {
	flags := MigrateCmd.Flags()
	flags.BoolVar(&migrateDryRun, "dry-run", false, "show the change list without writing anything")
	flags.BoolVar(&migrateCheck, "check", false, "write nothing and exit 2 when a project still needs migration")
	flags.BoolVar(&migrateAdoptDefaults, "adopt-defaults", false, "take the v5 defaults instead of pinning the 4.x ones")
	flags.BoolVar(&migrateWrite, "write", false, "also rewrite frontmatter aliases in .ai-rulez markdown files")
	flags.BoolVar(&migrateRecursive, "recursive", false, "migrate every project found below the current directory")
	flags.StringVar(&migrateFormat, "format", formatText, "output format: text or json")
	flags.StringVar(&migrateConfigDir, "config-dir", "", "config directory name to migrate (default .ai-rulez, then .config/ai-rulez)")
}

func runMigrate(out io.Writer, target string) int {
	if strings.EqualFold(target, migrateOKFTarget) {
		return runMigrateOKFChecked(out)
	}
	switch strings.TrimPrefix(strings.ToLower(target), "v") {
	case "5", "5.0":
	default:
		fmtError(fmt.Errorf("unsupported migration target %q: use v5 or okf (run `ai-rulez migrate v5`)", target))
		return 1
	}
	if migrateFormat != formatText && migrateFormat != formatJSON {
		fmtError(fmt.Errorf("unsupported --format %q: use text or json", migrateFormat))
		return 1
	}

	report, err := migrate.Run(migrate.Options{
		Root:          ".",
		ConfigDirName: migrateConfigDir,
		Recursive:     migrateRecursive,
		DryRun:        migrateDryRun,
		Check:         migrateCheck,
		AdoptDefaults: migrateAdoptDefaults,
		Write:         migrateWrite,
	})
	if err != nil {
		fmtError(err)
		return 1
	}

	if migrateFormat == formatJSON {
		if err := writeRawJSON(out, report); err != nil {
			fmtError(err)
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
		fmtError(fmt.Errorf("unsupported --format %q: use text or json", migrateFormat))
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
