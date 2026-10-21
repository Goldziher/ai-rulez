package lint

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// CodeAcceptUnused is AR9I0, the [[lint.accept]] entry that accepted nothing.
// AR9I is the lint configuration block of the allocation table in
// docs/strict-validation.md.
const CodeAcceptUnused = "AR9I0"

// routeAccept names [[lint.accept]] in the AR740 finding of a refused attempt.
const routeAccept = "[[lint.accept]]"

func registerAccept(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeAcceptUnused, "accept-unused", SeverityWarning, "a [[lint.accept]] entry accepted no finding: the finding was fixed, the path or match no longer fits, or the rule is not reported there; remove the entry"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeAcceptUnused: {
			Why:  "An acceptance outlives the finding it was written for when the file is fixed, renamed or reworded. A stale entry would silently accept a different finding that later appears at the same place, so an entry that matches nothing is reported and has to be removed.",
			Bad:  "`[[lint.accept]] rule = \"AR007\" path = \".claude/skills/foo/SKILL.md\" match = \"Bash\"` after the skill stopped granting `Bash`",
			Good: "Remove the entry, or fix its `path` or `match` so it names the finding it accepts",
		},
	})
}

// acceptEntry is a compiled [[lint.accept]] entry.
type acceptEntry struct {
	index int
	cfg   config.LintAccept
	code  string
	path  []globMatcher
	used  bool
}

func compileAccepts(entries []config.LintAccept) []*acceptEntry {
	var out []*acceptEntry
	for i, e := range entries {
		rule, ok := lookupRule(strings.TrimSpace(e.Rule))
		path := compileGlobs([]string{e.Path})
		if !ok || len(path) == 0 {
			continue // ValidateSettings reports it when the configuration loads
		}
		out = append(out, &acceptEntry{index: i, cfg: e, code: rule.Code, path: path})
	}
	return out
}

// accepts reports whether the entry covers a finding of code in the file abs,
// reported on line with message. match is a case-sensitive substring of the
// message and line the exact line; an entry that sets both needs both.
func (r *runner) accepts(e *acceptEntry, code, abs string, line int, message string) bool {
	if e.code != code || !r.pathMatches(abs, e.path) {
		return false
	}
	if e.cfg.Match != "" && !strings.Contains(message, e.cfg.Match) {
		return false
	}
	return e.cfg.Line == 0 || e.cfg.Line == line
}

// applyAccepts marks the findings that [[lint.accept]] accepts and reports the
// entries that accepted nothing (AR9I0). An accepted finding stays in the
// report with its reason and does not count toward the exit code, like a
// baseline entry; a code the organization policy protects is never accepted.
func (r *runner) applyAccepts() {
	entries := compileAccepts(r.lc.Accept)
	if len(entries) == 0 {
		return
	}
	for i := range r.findings {
		f := &r.findings[i]
		abs := f.File
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.cwd, filepath.FromSlash(abs))
		}
		for _, e := range entries {
			if !r.accepts(e, f.Code, abs, f.Line, f.Message) {
				continue
			}
			e.used = true
			if r.refuseSuppression(f.Code, routeAccept) {
				continue
			}
			m := f.meta()
			m.Accepted, m.AcceptedByConfig, m.AcceptReason = true, true, strings.TrimSpace(e.cfg.Reason)
		}
	}
	for _, e := range entries {
		if !e.used && r.acceptJudgeable(e) {
			r.add(CodeAcceptUnused, r.configFilePath(), 1,
				"lint.accept[%d] (%s in %s) accepted no finding; remove it or fix its path or match", e.index, e.cfg.Rule, e.cfg.Path)
		}
	}
}

// acceptJudgeable reports whether this run could have produced the finding the
// entry accepts. An entry whose rule is off or ignored, whose analyzer did not
// run, or whose path was not linted in a partial run (--path, --glob) says
// nothing about this run, so it is not reported.
func (r *runner) acceptJudgeable(e *acceptEntry) bool {
	if r.sev[e.code] == SeverityOff || r.ignore[e.code] {
		return false
	}
	if r.sel != nil && !r.sel[AnalyzerFor(e.code).Name] {
		return false
	}
	// The entry is judged when its path matches a file this run linted, or
	// matches no file of the project at all (the file is gone). A file that
	// exists but was not linted (a --path or --glob run) says nothing.
	if r.targets == nil || len(r.targets.Files) == 0 {
		return true // nothing narrowed the run
	}
	for i := range r.items {
		if r.pathMatches(r.items[i].abs, e.path) {
			return true
		}
	}
	for i := range r.targets.Files {
		if r.pathMatches(r.targets.Files[i].Abs, e.path) {
			return true
		}
	}
	for _, rel := range r.tree.Paths() {
		if r.pathMatches(filepath.Join(r.tree.Top, filepath.FromSlash(rel)), e.path) {
			return false
		}
	}
	return true
}

// validateAccepts reports entries that would silently match nothing or accept
// more than one finding's worth.
func validateAccepts(entries []config.LintAccept) []string {
	var problems []string
	for i, e := range entries {
		at := fmt.Sprintf("lint.accept[%d]", i)
		rule := strings.TrimSpace(e.Rule)
		switch {
		case rule == "":
			problems = append(problems, at+": rule is empty; name the code or name of the finding to accept")
		case strings.ContainsAny(rule, "*?["):
			problems = append(problems, fmt.Sprintf("%s: rule %q must name one code or name, not a pattern", at, e.Rule))
		default:
			if _, ok := lookupRule(rule); !ok {
				problems = append(problems, fmt.Sprintf("%s: unknown rule %q", at, e.Rule))
			}
		}
		switch {
		case strings.TrimSpace(e.Path) == "":
			problems = append(problems, at+": path is empty; name the file the acceptance covers")
		case malformedGlob(e.Path):
			problems = append(problems, fmt.Sprintf("%s: path glob %q is malformed", at, e.Path))
		}
		if strings.TrimSpace(e.Reason) == "" {
			problems = append(problems, at+": reason is required; say why the finding is acceptable")
		}
		if e.Line < 0 {
			problems = append(problems, fmt.Sprintf("%s: line %d must be 1 or more", at, e.Line))
		}
		if strings.TrimSpace(e.Match) == "" && e.Line == 0 {
			problems = append(problems, at+": set match or line; an entry with neither would accept every finding of the rule in the file (use [[lint.ignore_rules]] for that)")
		}
	}
	return problems
}
