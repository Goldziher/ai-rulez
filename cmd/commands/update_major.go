package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/pinupdate"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/versionpatch"
	"github.com/samber/oops"
)

// majorUpdate is `update --major`: it lists the sources that have a newer major
// version than their constraint allows and, with --write-config, takes it.
func majorUpdate(ctx context.Context, _ string, env pinupdate.Env, p *pinupdate.Prepared, report *updateReport) error {
	report.Major = append(report.Major, majorItems(p.Srcs, p.Rows)...)
	sortMajor(report.Major)
	if len(report.Major) == 0 || !updateWriteConfig || updateDryRun {
		return finishUpdate(report, nil)
	}
	original, mode, err := patchMajor(p.Cfg, report.Major)
	if err != nil {
		return fail(err)
	}
	rollback := func() {
		if rerr := safefs.WriteFileAtomicMode(p.Cfg.ConfigPath(), original, mode); rerr != nil {
			renderStderr(oops.Wrapf(rerr, "restore the config file; it still holds the new version constraints"))
		}
		for i := range report.Major {
			report.Major[i].Written = false
		}
	}
	// The pins follow the new constraints: evaluate again against the patched config.
	names := make([]string, 0, len(report.Major))
	for _, m := range report.Major {
		names = append(names, m.Name)
	}
	fresh, err := pinupdate.Prepare(ctx, env, updateKind, names)
	if err != nil {
		rollback()
		return fail(err)
	}
	applied := runPrepared(ctx, env, fresh, report)
	if applied != nil {
		rollback() // before the report is printed, so it never claims a write that was undone
	}
	return finishUpdate(report, applied)
}

// majorItems lists the sources whose latest tag is a newer major than their
// constraint allows, with the constraint that would take it.
func majorItems(srcs []versionSrc, rows []tagresolve.Row) []majorItem {
	var out []majorItem
	for i := range rows {
		row := &rows[i]
		if !row.MajorAvailable || row.Latest == nil || blockedStatus(row.Status) {
			continue
		}
		if to, ok := majorConstraint(row.Latest.Tag, srcs[i].Want.TagPrefix); ok {
			out = append(out, majorItem{Kind: row.Kind, Name: row.Name, From: row.Constraint, To: to, Latest: row.Latest.Tag})
		}
	}
	return out
}

// sortMajor orders the major items by kind, then name.
func sortMajor(items []majorItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
}

// blockedStatus reports the statuses where a constraint cannot be trusted enough to rewrite it.
func blockedStatus(status string) bool {
	switch status {
	case tagresolve.StatusTagMoved, tagresolve.StatusUnsatisfied, tagresolve.StatusInvalid:
		return true
	}
	return false
}

// majorConstraint is the constraint that takes the major version of tag: "^2.0" for v2.3.1.
func majorConstraint(tag, prefix string) (string, bool) {
	v, ok := semver.ParseTag(tag, prefix)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("^%d.0", v.Major), true
}

// patchMajor rewrites the version line of every item in config.toml (only that
// value changes) and returns the original bytes and mode for a rollback. The
// file keeps its permission bits: it is the project's file, not a private one.
func patchMajor(cfg *config.Config, items []majorItem) (original []byte, mode os.FileMode, err error) {
	file := cfg.ConfigPath()
	original, mode, err = safefs.ReadRegularKeepMode(file)
	if err != nil {
		return nil, 0, oops.With("path", file).Wrapf(err, "read %s", filepath.Base(file))
	}
	patched := original
	tables := map[string]string{lockfile.KindInclude: "includes", lockfile.KindSkill: "installed_skills", lockfile.KindSource: "skill_sources",
		lockfile.KindMarketplace: "installed_marketplaces", lockfile.KindPlugin: "installed_plugins"}
	for _, it := range items {
		patched, err = versionpatch.SetConstraint(patched, tables[it.Kind], it.Name, it.To)
		if err != nil {
			return nil, 0, oops.With("source", it.Name).Wrapf(err, "cannot update the constraint of %s %q", it.Kind, it.Name)
		}
	}
	if err := safefs.WriteFileAtomicMode(file, patched, mode); err != nil {
		return nil, 0, oops.With("path", file).Wrapf(err, "write %s", filepath.Base(file))
	}
	for i := range items {
		items[i].Written = true
	}
	return original, mode, nil
}
