package policy

import (
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// denyDigests reports every content digest the repository's lock pins that the
// policy denies (sources.deny_digests). A denied include, installed skill or
// skill source is dropped from the run, so it is not fetched; a denied authored
// item cannot be dropped and is only reported, which still fails generation. The
// lock is what names the digests, so an absent lock has nothing to check:
// require_pinned or lock.enforce makes the lock mandatory.
func (a *applier) denyDigests() {
	pol := a.res.Policy.Sources.DenyDigests
	if len(pol) == 0 || a.cfg.ConfigDir == "" {
		return
	}
	lock, err := lockfile.Load(a.cfg.ConfigDir)
	if err != nil || lock == nil {
		return
	}
	denied := func(d string) bool { return slices.Contains(pol, d) }
	report := func(what string, subject config.PolicySubject) {
		a.violate(lint.CodeDigestDenied, "sources.deny_digests", subject.Digest,
			"%s %q: %s pins %s, which the policy denies (origin: %s)", what, subject.Name, lockfile.FileName, subject.Digest, a.origin("sources.deny_digests"))
		a.out.Violations[len(a.out.Violations)-1].Subject = &subject
	}
	dropped := map[string]map[string]bool{lockfile.KindInclude: {}, lockfile.KindSkill: {}, lockfile.KindSource: {}}
	for kind, entries := range map[string][]lockfile.Entry{lockfile.KindInclude: lock.Include, lockfile.KindSkill: lock.Skill, lockfile.KindSource: lock.Source} {
		for i := range entries {
			e := &entries[i]
			if denied(e.Digest) {
				report(kind, config.PolicySubject{Kind: kind, Name: e.Name, Digest: e.Digest})
				dropped[kind][e.Name] = true
			}
		}
	}
	for i := range lock.Served {
		e := &lock.Served[i]
		if denied(e.Digest) {
			report("served skill", config.PolicySubject{Kind: lockfile.KindServed, Name: e.Name, Domain: e.View, Digest: e.Digest})
		}
	}
	for i := range lock.Item {
		it := &lock.Item[i]
		if denied(it.Digest) {
			report(it.Kind, config.PolicySubject{Kind: it.Kind, Name: it.ID, Domain: it.Domain, Digest: it.Digest})
		}
	}
	cfg := a.cfg
	cfg.Includes = filterSlice(cfg.Includes, func(i config.IncludeConfig) bool { return !dropped[lockfile.KindInclude][i.Name] })
	cfg.InstalledSkills = filterSlice(cfg.InstalledSkills, func(s config.InstalledSkillConfig) bool { return !dropped[lockfile.KindSkill][s.Name] })
	cfg.SkillSources = filterSlice(cfg.SkillSources, func(s config.SkillSourceConfig) bool { return !dropped[lockfile.KindSource][s.Name] })
}
