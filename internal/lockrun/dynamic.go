package lockrun

import (
	"context"
	"fmt"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

// The lock's part for dynamic skill loading: the commit and tree digest of every
// [[skill_sources]] entry (kind "source"), and the digest of every skill the
// skills server would serve (kind "served"), which `[lock] enforce` checks at
// serve time. Both live in the same ai-rulez.lock as includes and installed
// skills.

// KnownKind reports whether kind is an entry kind `lock --kind` accepts.
func KnownKind(kind string) bool {
	switch kind {
	case lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource, lockfile.KindServed:
		return true
	}
	return false
}

// HasName reports whether f pins a remote source or served skill called name.
func HasName(f *lockfile.File, name string) bool {
	for _, kind := range []string{lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource} {
		if f.Find(kind, name) != nil {
			return true
		}
	}
	for i := range f.Served {
		e := &f.Served[i]
		if e.Name == name {
			return true
		}
	}
	return false
}

// Entries lists the entries for display; a served skill of a view other than
// the default one carries the view in its name.
func Entries(f *lockfile.File) []lockfile.Entry {
	var all []lockfile.Entry
	for _, list := range [][]lockfile.Entry{f.Include, f.Skill, f.Source, f.Served} {
		all = append(all, list...)
	}
	for i := range all {
		if all[i].View != "" {
			all[i].Name += " (view " + all[i].View + ")"
		}
	}
	return all
}

// DynamicRun is what one `lock` run refreshes of the source and served pins: an
// entry kind (all when empty), the names to limit it to (all when empty), the
// extra serve views to pin, and whether a refused skill fails the run.
type DynamicRun struct {
	Kind   string
	Wanted map[string]bool
	Extras []mcp.ServeSetup
	Strict bool
	// Version is the ai-rulez version the served digests are computed with.
	Version string
	// Collector keeps the warnings of the views' loads (nil: their own).
	Collector *diag.Collector
	// LoadOptions are added to the views' configuration loads (an embedding
	// service's workspace and host).
	LoadOptions []config.LoadOption
}

// DynamicResult is what refreshing the dynamic pins found: problems that stop
// the lock, how many of them are scan refusals (Strict), and the skills left
// unpinned because the scan refuses them.
type DynamicResult struct {
	Problems []string
	Refused  int
	Unpinned []mcp.Refusal
	// Refusals are the refused skills that are among Problems (Strict).
	Refusals []mcp.Refusal
}

// ScanOnly reports whether every problem is a served skill the security scan
// refuses under Strict: findings, not a failure to run.
func (r DynamicResult) ScanOnly() bool { return len(r.Problems) > 0 && r.Refused == len(r.Problems) }

// MergeDynamic carries the source and served pins into next: kept from current
// unless this run refreshes them, in which case the sources are re-resolved and
// the served digests recomputed, one set per serve view. A skill the security
// scan refuses is a problem only with Strict; otherwise it is left unpinned
// (Unpinned) and the other skills are pinned.
func MergeDynamic(ctx context.Context, cfg *config.Config, current, next *lockfile.File, run DynamicRun) (out DynamicResult) {
	kind, wanted, extras := run.Kind, run.Wanted, run.Extras
	if current != nil {
		next.Source, next.Served = append([]lockfile.Entry(nil), current.Source...), append([]lockfile.Entry(nil), current.Served...)
	}
	if !UsesDynamicSkills(cfg, extras) {
		next.Source, next.Served = nil, nil
		return out
	}
	refresh := func(k string) bool { return kind == "" || kind == k }
	if !refresh(lockfile.KindSource) && !refresh(lockfile.KindServed) {
		return out
	}
	// The views load under the lock run's policy, so a refresh re-resolves the
	// same sources the run's own load did.
	policy := cfg.LockPolicy
	setup := &mcp.ServeSetup{Version: run.Version, WorkDir: cfg.BaseDir, NoWatch: true, Offline: policy.Offline, LockPolicy: &policy,
		Collector: run.Collector, LoadOptions: run.LoadOptions}
	if cfg.Host.Log != nil {
		setup.Log = cfg.Host.Log // the views report where the run's own load does
	}
	res, err := setup.LockViews(ctx, extras)
	if err != nil {
		out.Problems = []string{err.Error()}
		return out
	}
	for _, r := range res.Refused {
		if run.Strict {
			out.Problems = append(out.Problems, fmt.Sprintf("served %s: %s %s", r.Name, r.Code, r.Reason))
			out.Refusals = append(out.Refusals, r)
			out.Refused++
			continue
		}
		out.Unpinned = append(out.Unpinned, r)
		cfg.Log().Warn("Left a served skill unpinned: the security scan refuses it", "skill", r.Name, "view", viewLabel(r.View), "code", r.Code, "reason", r.Reason)
	}
	pick := func(name string) bool { return len(wanted) == 0 || wanted[name] }
	if refresh(lockfile.KindServed) {
		next.Served = mergeServed(next.Served, res, pick)
	}
	if refresh(lockfile.KindSource) {
		for i := range res.Sources {
			e := &res.Sources[i]
			if pick(e.Name) {
				next.Set(lockfile.KindSource, *e)
			}
		}
		next.Source = keepConfigured(next.Source, sourceNames(cfg, next.Served))
	}
	sort.Strings(out.Problems)
	return out
}

// UsesDynamicSkills reports whether cfg (with the extra views) has source or
// served pins to keep.
func UsesDynamicSkills(cfg *config.Config, extras []mcp.ServeSetup) bool {
	if len(extras) == 0 && pluginOnly(cfg) {
		return false
	}
	return len(extras) > 0 || len(cfg.SkillSources) > 0 || !cfg.LockEnforceOptedOut() || cfg.DeliveryConfigured(cfg.Content) || cfg.RolesServeSkills()
}

// pluginOnly reports a plugin source that configures no preset, such as a
// marketplace member: its skills ship in the bundles `generate --plugin` writes,
// and the skills server cannot serve it (serving renders skills with a preset),
// so lock pins its content and no served skills.
func pluginOnly(cfg *config.Config) bool {
	return cfg.Plugin != nil && len(cfg.Presets) == 0
}

// ServesNothing reports a plugin-only configuration (see pluginOnly) whose lock
// pins no served skill: there is no served view to compare.
func ServesNothing(cfg *config.Config, lock *lockfile.File, extras []mcp.ServeSetup) bool {
	return len(extras) == 0 && pluginOnly(cfg) && (lock == nil || len(lock.Served) == 0)
}

func viewLabel(view string) string {
	if view == "" {
		return "default"
	}
	return view
}

// mergeServed replaces the served pins of every view the run evaluated with the
// recomputed ones (limited to the named skills with `lock <name>`), keeps the pins
// of views it did not evaluate, and drops those of a view that can be rebuilt
// from its key but no longer exists (a removed role).
func mergeServed(existing []lockfile.Entry, res *mcp.LockResult, pick func(string) bool) []lockfile.Entry {
	evaluated := map[string]bool{}
	for _, v := range res.Views {
		evaluated[v] = true
	}
	fresh := map[string]bool{}
	next := &lockfile.File{Served: existing}
	for i := range res.Served {
		e := &res.Served[i]
		fresh[e.View+"\x00"+e.Name] = true
		if pick(e.Name) {
			next.Set(lockfile.KindServed, *e)
		}
	}
	out := next.Served[:0:0]
	for i := range next.Served {
		e := &next.Served[i]
		switch {
		case evaluated[e.View]:
			if fresh[e.View+"\x00"+e.Name] {
				out = append(out, *e)
			}
		case len(mcp.ViewKeySources(e.View)) == 0 && e.View != "":
			// a role or profile that no longer builds: its pins are stale
		default:
			out = append(out, *e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].View < out[j].View
	})
	return out
}

// sourceNames lists the skill sources the lock may keep: the configured ones and
// those a pinned view names with --source.
func sourceNames(cfg *config.Config, served []lockfile.Entry) map[string]bool {
	out := map[string]bool{}
	for i := range cfg.SkillSources {
		out[cfg.SkillSources[i].Name] = true
	}
	for i := range served {
		e := &served[i]
		for _, name := range mcp.ViewKeySources(e.View) {
			out[name] = true
		}
	}
	return out
}

func keepConfigured(entries []lockfile.Entry, keep map[string]bool) []lockfile.Entry {
	out := entries[:0:0]
	for i := range entries {
		e := &entries[i]
		if keep[e.Name] {
			out = append(out, *e)
		}
	}
	return out
}
