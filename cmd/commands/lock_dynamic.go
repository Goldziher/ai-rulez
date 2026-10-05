package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

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
	for _, kind := range []string{lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource, lockfile.KindServed} {
		if f.Find(kind, name) != nil {
			return true
		}
	}
	return false
}

func lockedEntries(f *lockfile.File) []lockfile.Entry {
	var all []lockfile.Entry
	for _, list := range [][]lockfile.Entry{f.Include, f.Skill, f.Source, f.Served} {
		all = append(all, list...)
	}
	return all
}

// usesDynamicSkills reports whether the project has anything to pin for served skills.
func usesDynamicSkills(cfg *config.Config) bool {
	return len(cfg.SkillSources) > 0 || cfg.LockEnforced() || cfg.DeliveryConfigured(cfg.Content) || cfg.RolesServeSkills()
}

// mergeDynamicLock carries the source and served pins into next: kept from
// current unless this run refreshes them, in which case the sources are
// re-resolved and the served digests recomputed. It returns the problems that
// prevent writing a lock.
func mergeDynamicLock(cfg *config.Config, current, next *lockfile.File, kind string, wanted map[string]bool) []string {
	if current != nil {
		next.Source, next.Served = append([]lockfile.Entry(nil), current.Source...), append([]lockfile.Entry(nil), current.Served...)
	}
	if !usesDynamicSkills(cfg) {
		next.Source, next.Served = nil, nil
		return nil
	}
	refresh := func(k string) bool { return kind == "" || kind == k }
	if !refresh(lockfile.KindSource) && !refresh(lockfile.KindServed) {
		return nil
	}
	setup := &mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true, Offline: includes.SkipFetch}
	sources, served, refused, err := setup.LockRecords(context.Background())
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	for _, r := range refused {
		problems = append(problems, fmt.Sprintf("served %s: %s %s", r.Name, r.Code, r.Reason))
	}
	pick := func(name string) bool { return len(wanted) == 0 || wanted[name] }
	if refresh(lockfile.KindSource) {
		for _, e := range sources {
			if pick(e.Name) {
				next.Set(lockfile.KindSource, e)
			}
		}
		next.Source = keepConfigured(next.Source, sourceNames(cfg))
	}
	if refresh(lockfile.KindServed) {
		keep := map[string]bool{}
		for _, e := range served {
			keep[e.Name] = true
			if pick(e.Name) {
				next.Set(lockfile.KindServed, e)
			}
		}
		next.Served = keepConfigured(next.Served, keep)
	}
	sort.Strings(problems)
	return problems
}

func sourceNames(cfg *config.Config) map[string]bool {
	out := map[string]bool{}
	for i := range cfg.SkillSources {
		out[cfg.SkillSources[i].Name] = true
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
	var out []string
	for _, p := range skillsource.CheckLock(cfg.SkillSources, lock, "") {
		out = append(out, "  "+p.String())
	}
	if lock != nil && (len(lock.Served) > 0 || cfg.LockEnforced()) {
		includes.SkipFetch = true
		setup := &mcp.ServeSetup{Version: Version, WorkDir: cfg.BaseDir, NoWatch: true}
		problems, err := setup.ServedProblems(context.Background())
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
