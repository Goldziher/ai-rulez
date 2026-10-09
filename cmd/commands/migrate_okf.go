package commands

import (
	"context"
	"io"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// migrateOKFTarget is the migrate target that converts a project's content tree
// to an OKF bundle.
const migrateOKFTarget = "okf"

type migrateOKFReport struct {
	ConfigDir string                   `json:"config_dir"`
	DryRun    bool                     `json:"dry_run"`
	Changes   []migrateOKFChangeReport `json:"changes"`
	Summary   migrateOKFSummary        `json:"summary"`
}

type migrateOKFChangeReport struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

type migrateOKFSummary struct {
	Converted int `json:"converted"`
	Indexes   int `json:"indexes"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
}

// runMigrateOKF converts the content tree of the project in the current
// directory to an OKF bundle. It returns the process exit code.
func runMigrateOKF(ctx context.Context, out io.Writer) int {
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutLocal())
	if err != nil {
		renderStderr(err)
		return 1
	}
	write := !migrateDryRun && !migrateCheck
	changes, err := okfbridge.MigrateDir(ctx, cfg.ConfigDir, okfbridge.MigrateOptions{Write: write})
	if err != nil {
		renderStderr(err)
		return 1
	}
	report := migrateOKFReport{ConfigDir: cfg.ConfigDir, DryRun: !write, Changes: []migrateOKFChangeReport{}}
	pending := false
	for _, c := range changes {
		report.Changes = append(report.Changes, migrateOKFChangeReport{Path: c.Path, Action: c.Action, Detail: c.Detail})
		pending = pending || c.Pending()
		switch c.Action {
		case okfbridge.ActionConverted:
			report.Summary.Converted++
		case okfbridge.ActionIndex:
			report.Summary.Indexes++
		case okfbridge.ActionSkipped:
			report.Summary.Skipped++
		default:
			report.Summary.Unchanged++
		}
	}
	if migrateFormat == formatJSON {
		if err := writeRawJSON(out, report); err != nil {
			renderStderr(err)
			return 1
		}
	} else {
		printMigrateOKFReport(out, &report)
	}
	if migrateCheck && pending {
		return exitDrift
	}
	return 0
}

func printMigrateOKFReport(out io.Writer, r *migrateOKFReport) {
	w := reportWriter{out}
	for _, c := range r.Changes {
		if c.Action == okfbridge.ActionUnchanged && c.Detail == "" {
			continue
		}
		w.printf("[%s] %s", c.Action, c.Path)
		if c.Detail != "" {
			w.printf(": %s", c.Detail)
		}
		w.printf("\n")
	}
	verb := "converted"
	if r.DryRun {
		verb = "would convert"
	}
	w.printf("%s %d, indexes %d, unchanged %d, skipped %d\n", verb, r.Summary.Converted, r.Summary.Indexes, r.Summary.Unchanged, r.Summary.Skipped)
}
