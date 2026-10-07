package mcp

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// DynamicLockProblems verifies the source and served pins of lock against the
// configuration and the local cache. It never uses the network: the served
// skills are built from an offline context. extras are serve views to check on
// top of the default one and the ones the lock records. The version is the
// ai-rulez version the check reports as running.
func DynamicLockProblems(ctx context.Context, cfg *config.Config, lock *lockfile.File, version string, extras ...ServeSetup) []string {
	if !usesDynamicSkills(cfg, len(extras) > 0) {
		return nil
	}
	sources := append([]config.SkillSourceConfig(nil), cfg.SkillSources...)
	for i := range extras {
		for _, arg := range extras[i].Sources {
			if spec, err := skillsource.ParseArg(arg); err == nil {
				sources = append(sources, config.SkillSourceConfig{Name: spec.Name, URL: spec.URL, Ref: spec.Ref, Path: spec.Path})
			}
		}
	}
	var viewSources []string
	if lock != nil {
		for i := range lock.Served {
			viewSources = append(viewSources, ViewKeySources(lock.Served[i].View)...)
		}
	}
	var out []string
	for _, p := range skillsource.CheckLock(sources, lock, "", viewSources...) {
		out = append(out, "  "+p.String())
	}
	if lock != nil && (len(lock.Served) > 0 || len(extras) > 0 || cfg.LockEnforced()) {
		setup := &ServeSetup{Version: version, WorkDir: cfg.BaseDir, NoWatch: true, Collector: cfg.Diag}
		problems, err := setup.ServedProblems(ctx, extras...)
		if err != nil {
			out = append(out, "  served: "+err.Error())
		}
		for _, p := range problems {
			out = append(out, "  "+p)
		}
	}
	return out
}

// DynamicLockChanges reports the source and served pins that disagree with the
// configuration and the local cache, as changes for `lock --check` and `--diff`.
func DynamicLockChanges(ctx context.Context, cfg *config.Config, lock *lockfile.File, version string, extras ...ServeSetup) []contentlock.Change {
	var out []contentlock.Change
	for _, line := range DynamicLockProblems(ctx, cfg, lock, version, extras...) {
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

// usesDynamicSkills reports whether the project has anything to pin for served
// skills; hasExtraViews is true when the caller selects serve views of its own.
func usesDynamicSkills(cfg *config.Config, hasExtraViews bool) bool {
	return hasExtraViews || len(cfg.SkillSources) > 0 || !cfg.LockEnforceOptedOut() || cfg.DeliveryConfigured(cfg.Content) || cfg.RolesServeSkills()
}
