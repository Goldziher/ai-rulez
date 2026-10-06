package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/spf13/cobra"
)

// exitUnpinned is the exit code of a `lock` that wrote the lock but left served
// skills unpinned because the security scan refuses them (see --strict).
const exitUnpinned = 3

// Flags that select the serve view `lock` pins besides the default one, the
// roles and the views the lock already records. They mirror `mcp --serve-skills`.
// --profile is shared with the output pins and also names a served view.
var (
	lockServeRole          string
	lockServeIncludeStatic bool
	lockServeSources       []string
	lockStrict             bool
	// lockUnpinned collects the refusals of this run, for the exit code.
	lockUnpinned []mcp.Refusal
)

func init() {
	f := LockCmd.Flags()
	f.StringVar(&lockServeRole, "role", "", "Also pin the skills this role serves, as a view of their own (see mcp --serve-skills --role)")
	f.BoolVar(&lockServeIncludeStatic, "include-static", false, "Also pin the view that serves static skills too (see mcp --serve-skills --include-static)")
	f.StringArrayVar(&lockServeSources, "source", nil, "Also pin the view with this extra skill source, repeatable (see mcp --serve-skills --source)")
	f.BoolVar(&lockStrict, "strict", false, "Fail without writing when the security scan refuses any served skill (default: leave that skill unpinned, pin the rest and exit 3)")
	LockCmd.PostRun = func(*cobra.Command, []string) {
		if len(lockUnpinned) > 0 {
			fmt.Fprintf(os.Stderr, "%d served skill(s) were left unpinned because the security scan refuses them; fix them, or use --strict to fail instead\n", len(lockUnpinned))
			os.Exit(exitUnpinned)
		}
	}
}

// lockExtraViews is the view the serve-view flags select, if any.
func lockExtraViews() []mcp.ServeSetup {
	if lockServeRole == "" && lockProfile == "" && !lockServeIncludeStatic && len(lockServeSources) == 0 {
		return nil
	}
	return []mcp.ServeSetup{{Role: lockServeRole, Profile: lockProfile, IncludeStatic: lockServeIncludeStatic, Sources: lockServeSources}}
}

// The lock command's part for dynamic skill loading: the commit and tree digest
// of every [[skill_sources]] entry (kind "source"), and the digest of every
// skill the skills server would serve (kind "served"), which `[lock] enforce`
// checks at serve time. Both live in the same ai-rulez.lock as includes and
// installed skills.

func knownLockKind(kind string) bool {
	switch kind {
	case lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource, lockfile.KindServed:
		return true
	}
	return false
}

func lockHasName(f *lockfile.File, name string) bool {
	for _, kind := range []string{lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource} {
		if f.Find(kind, name) != nil {
			return true
		}
	}
	for _, e := range f.Served {
		if e.Name == name {
			return true
		}
	}
	return false
}

// lockedEntries lists the entries for display; a served skill of a view other
// than the default one carries the view in its name.
func lockedEntries(f *lockfile.File) []lockfile.Entry {
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

// usesDynamicSkills reports whether the project has anything to pin for served skills.
func usesDynamicSkills(cfg *config.Config) bool {
	return len(lockExtraViews()) > 0 || len(cfg.SkillSources) > 0 || !cfg.LockEnforceOptedOut() || cfg.DeliveryConfigured(cfg.Content) || cfg.RolesServeSkills()
}

// mergeDynamicLock carries the source and served pins into next: kept from
// current unless this run refreshes them, in which case the sources are
// re-resolved and the served digests recomputed, one set per serve view. It
// returns the problems that prevent writing a lock. A skill the security scan
// refuses is such a problem only with --strict; otherwise it is left unpinned
// (see lockUnpinned) and the other skills are pinned.
func mergeDynamicLock(cfg *config.Config, current, next *lockfile.File, kind string, wanted map[string]bool) []string {
	problems, unpinned := mergeDynamicViews(cfg, current, next, dynamicRun{kind: kind, wanted: wanted, extras: lockExtraViews(), strict: lockStrict})
	lockUnpinned = append(lockUnpinned, unpinned...)
	return problems
}

// dynamicRun is what one `lock` run refreshes: an entry kind (all when empty),
// the names to limit it to (all when empty), the extra serve views to pin, and
// whether a refused skill fails the run.
type dynamicRun struct {
	kind   string
	wanted map[string]bool
	extras []mcp.ServeSetup
	strict bool
}

func mergeDynamicViews(cfg *config.Config, current, next *lockfile.File, run dynamicRun) (problems []string, unpinned []mcp.Refusal) {
	kind, wanted, extras, strict := run.kind, run.wanted, run.extras, run.strict
	if current != nil {
		next.Source, next.Served = append([]lockfile.Entry(nil), current.Source...), append([]lockfile.Entry(nil), current.Served...)
	}
	if !usesDynamicSkillsFor(cfg, extras) {
		next.Source, next.Served = nil, nil
		return nil, nil
	}
	refresh := func(k string) bool { return kind == "" || kind == k }
	if !refresh(lockfile.KindSource) && !refresh(lockfile.KindServed) {
		return nil, nil
	}
	setup := &mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true, Offline: includes.SkipFetch}
	res, err := setup.LockViews(context.Background(), extras)
	if err != nil {
		return []string{err.Error()}, nil
	}
	for _, r := range res.Refused {
		if strict {
			problems = append(problems, fmt.Sprintf("served %s: %s %s", r.Name, r.Code, r.Reason))
			continue
		}
		unpinned = append(unpinned, r)
		logger.Warn("Left a served skill unpinned: the security scan refuses it", "skill", r.Name, "view", viewLabel(r.View), "code", r.Code, "reason", r.Reason)
	}
	pick := func(name string) bool { return len(wanted) == 0 || wanted[name] }
	if refresh(lockfile.KindServed) {
		next.Served = mergeServed(next.Served, res, pick)
	}
	if refresh(lockfile.KindSource) {
		for _, e := range res.Sources {
			if pick(e.Name) {
				next.Set(lockfile.KindSource, e)
			}
		}
		next.Source = keepConfigured(next.Source, sourceNames(cfg, next.Served))
	}
	sort.Strings(problems)
	return problems, unpinned
}

func usesDynamicSkillsFor(cfg *config.Config, extras []mcp.ServeSetup) bool {
	return len(extras) > 0 || len(cfg.SkillSources) > 0 || !cfg.LockEnforceOptedOut() || cfg.DeliveryConfigured(cfg.Content) || cfg.RolesServeSkills()
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
	for _, e := range res.Served {
		fresh[e.View+"\x00"+e.Name] = true
		if pick(e.Name) {
			next.Set(lockfile.KindServed, e)
		}
	}
	out := next.Served[:0:0]
	for _, e := range next.Served {
		switch {
		case evaluated[e.View]:
			if fresh[e.View+"\x00"+e.Name] {
				out = append(out, e)
			}
		case len(mcp.ViewKeySources(e.View)) == 0 && e.View != "":
			// a role or profile that no longer builds: its pins are stale
		default:
			out = append(out, e)
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
	for _, e := range served {
		for _, name := range mcp.ViewKeySources(e.View) {
			out[name] = true
		}
	}
	return out
}

func keepConfigured(entries []lockfile.Entry, keep map[string]bool) []lockfile.Entry {
	out := entries[:0:0]
	for _, e := range entries {
		if keep[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

// checkDynamicLock verifies the source and served pins against the configuration
// and the local cache without the network.
func checkDynamicLock(cfg *config.Config, lock *lockfile.File) []string {
	if !usesDynamicSkills(cfg) {
		return nil
	}
	extras := lockExtraViews()
	sources := append([]config.SkillSourceConfig(nil), cfg.SkillSources...)
	for _, e := range extras {
		for _, arg := range e.Sources {
			if spec, err := skillsource.ParseArg(arg); err == nil {
				sources = append(sources, config.SkillSourceConfig{Name: spec.Name, URL: spec.URL, Ref: spec.Ref, Path: spec.Path})
			}
		}
	}
	var viewSources []string
	if lock != nil {
		for _, e := range lock.Served {
			viewSources = append(viewSources, mcp.ViewKeySources(e.View)...)
		}
	}
	var out []string
	for _, p := range skillsource.CheckLock(sources, lock, "", viewSources...) {
		out = append(out, "  "+p.String())
	}
	if lock != nil && (len(lock.Served) > 0 || len(extras) > 0 || cfg.LockEnforced()) {
		defer func(prev bool) { includes.SkipFetch = prev }(includes.SkipFetch)
		includes.SkipFetch = true
		setup := &mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true}
		problems, err := setup.ServedProblems(context.Background(), extras...)
		if err != nil {
			out = append(out, "  served: "+err.Error())
		}
		for _, p := range problems {
			out = append(out, "  "+p)
		}
	}
	return out
}

// dynamicLockChanges reports the source and served pins that disagree with the
// configuration and the local cache, as changes for `lock --check` and `--diff`.
func dynamicLockChanges(cfg *config.Config, lock *lockfile.File) []contentlock.Change {
	var out []contentlock.Change
	for _, line := range checkDynamicLock(cfg, lock) {
		kind, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		kind = strings.TrimSuffix(kind, ":")
		name, detail, found := strings.Cut(rest, ": ")
		if !found {
			name, detail = "", rest
		}
		scope := contentlock.ScopeRemote
		if kind == lockfile.KindServed {
			scope = contentlock.ScopeServed
		}
		out = append(out, contentlock.Change{Scope: scope, Change: contentlock.Changed, Kind: kind, ID: name, Detail: detail})
	}
	return out
}
