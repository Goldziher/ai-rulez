package commands

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
)

// lockAcceptFindings is `lock --accept-findings`.
var lockAcceptFindings bool

// scanNewPins runs the security scan (AR001-AR009, as `update` does) over the
// tree of every remote entry that `lock` is about to pin to something new: an
// entry with no pin yet, or a different commit or digest. An unchanged pin was
// scanned when it was pinned. It returns how many sources were refused (error
// findings without --accept-findings) and prints each refusal.
func scanNewPins(cfg *config.Config, current, next *lockfile.File) (refused int) {
	wants := map[string]lockfile.Want{}
	for _, w := range includes.Lockable(cfg) {
		wants[w.Kind+"\x00"+w.Name] = w
	}
	specs := map[string]skillsource.Spec{}
	for i := range cfg.SkillSources {
		spec := skillsource.FromConfig(&cfg.SkillSources[i])
		specs[spec.Name] = spec
	}
	groups := []struct {
		kind    string
		entries []lockfile.Entry
	}{{lockfile.KindInclude, next.Include}, {lockfile.KindSkill, next.Skill}, {lockfile.KindSource, next.Source}}
	for _, g := range groups {
		for i := range g.entries {
			e := &g.entries[i]
			if unchangedPin(current, g.kind, e) {
				continue
			}
			dir := ""
			if g.kind == lockfile.KindSource {
				if spec, ok := specs[e.Name]; ok {
					dir = skillsource.CachedTreeDir(spec, e.Commit, "")
				}
			} else if w, ok := wants[g.kind+"\x00"+e.Name]; ok {
				dir = includes.CachedTreeDir(cfg, w)
			}
			if dir == "" {
				continue // not in the local cache (a local source, or nothing fetched): nothing to scan
			}
			sum := scanTreeDir(cfg, e.Name, dir, lockAcceptFindings)
			if sum.Errors == 0 {
				continue
			}
			printPinScan(g.kind, e.Name, sum)
			if sum.Refused {
				refused++
			}
		}
	}
	return refused
}

func unchangedPin(current *lockfile.File, kind string, e *lockfile.Entry) bool {
	if current == nil {
		return false
	}
	old := current.Find(kind, e.Name)
	return old != nil && old.Commit == e.Commit && old.Digest == e.Digest
}

// printPinScan reports the findings that stop (or, accepted, ride along with) a pin.
func printPinScan(kind, name string, sum *scanSummary) {
	verdict := "accepted with --accept-findings"
	if sum.Refused {
		verdict = "refused"
	}
	fmt.Fprintf(os.Stderr, "%s %s: the security scan found %d error(s), %d warning(s): %s\n", kind, name, sum.Errors, sum.Warnings, verdict)
	for _, f := range sum.Findings {
		fmt.Fprintf(os.Stderr, "  %s %s %s:%d %s\n", f.Code, f.Severity, safeText(f.File), f.Line, safeText(f.Message))
	}
}
