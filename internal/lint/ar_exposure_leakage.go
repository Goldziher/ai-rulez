package lint

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// The leakage rules: area words in the text a session sees at its start (AR9W0, AR9W1) and the skill_areas keys
// that name no skill (AR9W6).
const (
	CodeExposureAreaTerm      = "AR9W0"
	CodeExposureAreaAmbiguous = "AR9W1"
	CodeExposureSkillUnknown  = "AR9W6"
)

// ExposureLeakage is the leakage of the start-of-session text of a project, as the session exposure finds it.
type ExposureLeakage struct {
	// Checked counts the start-of-session artifacts read (an artifact of unknown phase included).
	Checked int
	// Leaks are the area words found: by artifact in inventory order, a body's violations by line, then a
	// listing's warnings.
	Leaks []ExposureLeak
	// UnknownSkills are the skill_areas entries whose key names no skill of the inventory, by key.
	UnknownSkills []ExposureSkillArea
	// AreasFile is the absolute path of the areas file the map was read from, "" for the [exposure] table of
	// the configuration.
	AreasFile string
	// Unchecked are the start-of-session artifacts whose text could not be read (over the read limit, a
	// symlink to outside the root), so no leak in them can be found. They are reported, not findings.
	Unchecked []ExposureUnchecked
}

// ExposureUnchecked is a start-of-session artifact whose text was not read. Path is relative to the source's
// Root and slash separated; Kind is the artifact kind and Name its qualified name.
type ExposureUnchecked struct {
	Path, Kind, Name, Reason string
}

// ExposureLeak is one area word in the start-of-session text. Path is relative to the source's Root and slash
// separated; Kind is the artifact kind (claude_md, rule, skill, agent, mcp_server, plugin) and Name its
// qualified name. Message is the finding message, a public contract a [[lint.accept]] match is a substring of.
type ExposureLeak struct {
	// Ambiguous marks a listing that names only ambiguous words (AR9W1); otherwise the word is a term (AR9W0).
	Ambiguous bool
	Area      string
	Term      string
	Kind      string
	Name      string
	Path      string
	Scope     string
	Line      int
	Excerpt   string
	Message   string
}

// ExposureSkillArea is a skill_areas entry.
type ExposureSkillArea struct {
	Skill, Area string
}

// LeakageResult is the leakage view of a run (Report.Leakage).
type LeakageResult struct {
	// Checked counts the start-of-session artifacts read.
	Checked int
	// Leaks are the AR9W0 and AR9W1 findings the run reported, in the source's order.
	Leaks []LeakFinding
	// Unchecked are the start-of-session artifacts whose text could not be read.
	Unchecked []ExposureUnchecked
	// Stale are the messages of the AR9I0 findings of the [[lint.accept]] entries for AR9W0 or AR9W1.
	Stale []string
}

// LeakFinding is a leak as the run reported it.
type LeakFinding struct {
	ExposureLeak
	// Code is AR9W0 or AR9W1 and Severity the finding's.
	Code     string
	Severity Severity
	// Accepted is set when a [[lint.accept]] entry accepts the finding, with the entry's reason.
	Accepted bool
	Reason   string
}

// leakageRun is what the leakage rules reported in one run.
type leakageRun struct {
	ran       bool
	checked   int
	leaks     []leakRecord
	unchecked []ExposureUnchecked
}

// leakRecord is a leak reported as a finding in file (the finding's display path).
type leakRecord struct {
	leak ExposureLeak
	code string
	file string
}

// leakageResult is what Report.Leakage reads after the run.
type leakageResult struct {
	leakageRun
	stale []string
}

func registerArExposureLeakage(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeExposureAreaTerm, "exposure-area-term", SeverityError, "a term of an area that is not shared appears in text a Claude Code session sees at its start (a root CLAUDE.md, an unconditional root rule, a skill or agent listing line, an MCP server name, an enabled plugin name), outside path literals"},
		RuleInfo{CodeExposureAreaAmbiguous, "exposure-area-term-ambiguous", SeverityWarning, "a skill or agent listed at session start names no term of any area but some ambiguous words, so the area it belongs to cannot be told"},
		RuleInfo{CodeExposureSkillUnknown, "exposure-skill-area-unknown", SeverityWarning, "a skill_areas key of the area map names no skill of the session inventory"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeExposureAreaTerm: {
			Why:  "What a session sees at its start is in the context of every task, whatever area it is about. A word that names one area there pulls every session toward that area and costs tokens in all the others; the knowledge belongs in the area's own directory, where it loads on demand. The message is `<kind> \"<name>\" at session start names term \"<term>\" of area \"<area>\"`, so a `[[lint.accept]]` match can name the artifact or the term.",
			Bad:  "A root CLAUDE.md line `Run the invoice reconciler before a release`, with `invoice` a term of the area `billing`",
			Good: "Move the line to `services/billing/CLAUDE.md`, name the location instead (`services/billing/`, a path literal), or accept it with a reason: `[[lint.accept]] rule = \"AR9W0\" path = \"CLAUDE.md\" match = 'names term \"invoice\"'`",
		},
		CodeExposureAreaAmbiguous: {
			Why:  "A skill or agent listed at session start whose description holds only words several areas use (`account`, `query`) may belong to one of them. Its listing loads in every session while the map cannot say whose it is.",
			Bad:  "`.claude/skills/lookup/SKILL.md` with `description: Looks up an account by query`, both ambiguous words",
			Good: "`description: Looks up a billing customer account` (a term decides), or `[skill_areas] lookup = \"billing\"` and the skill moved under the area's directory",
		},
		CodeExposureSkillUnknown: {
			Why:  "A skill_areas entry outlives the skill it places when the skill is renamed, moved into a plugin or deleted. The stale entry places nothing, and would silently place a different skill that later takes the name.",
			Bad:  "`[skill_areas] deploy-staging = \"platform\"` after the skill was renamed `ship-staging`",
			Good: "Rename the key (`ship-staging = \"platform\"`), or remove it; a plugin skill is keyed `plugin:skill`",
		},
	})
	for _, code := range []string{CodeExposureAreaTerm, CodeExposureAreaAmbiguous, CodeExposureSkillUnknown} {
		SetAnalyzer(code, AnalyzerExposure, ScopeBundle)
	}
	s.addRunCheck(checkExposureLeakage, AnalyzerExposure)
}

// checkExposureLeakage reports the area words of the start-of-session text (AR9W0, AR9W1) and the skill_areas
// keys that name no skill (AR9W6). Each leak finding depends on the source's inputs, so --changed keeps every
// leak when the area map, the configuration, the root settings or the marketplace changed.
func checkExposureLeakage(r *runner) {
	if r.exposure == nil {
		return
	}
	lk, ok := r.exposure.Leakage()
	if !ok {
		return
	}
	r.leakage.ran, r.leakage.checked, r.leakage.unchecked = true, lk.Checked, lk.Unchecked
	root := r.exposureRoot()
	for i := range lk.Leaks {
		l := &lk.Leaks[i]
		code := CodeExposureAreaTerm
		if l.Ambiguous {
			code = CodeExposureAreaAmbiguous
		}
		abs := filepath.Join(root, filepath.FromSlash(l.Path))
		before := len(r.findings)
		r.add(code, abs, l.Line, "%s", l.Message)
		if len(r.findings) == before {
			continue
		}
		r.exposureDeps(before, abs, nil)
		r.leakage.leaks = append(r.leakage.leaks, leakRecord{leak: *l, code: code, file: r.findings[before].File})
	}
	for _, s := range lk.UnknownSkills {
		path, line := r.skillAreasLine(lk.AreasFile, s.Skill)
		before := len(r.findings)
		r.add(CodeExposureSkillUnknown, path, line, "skill_areas assigns %q to %q, but no skill of the session inventory is named %q; rename or remove the entry", s.Skill, s.Area, s.Skill)
		r.exposureDeps(before, path, nil)
	}
}

// skillAreasLine locates a skill_areas key: in the areas file when the map comes from one, else in the
// [exposure.skill_areas] table of the configuration.
func (r *runner) skillAreasLine(areasFile, key string) (path string, line int) {
	if areasFile == "" {
		return r.configLineWhere(func(lines []string) int { return areas.SkillLine(lines, key) })
	}
	d, ok := r.docs[areasFile]
	if !ok {
		data, err := safefs.ReadFileLimited(areasFile, areas.MaxFileBytes)
		if err != nil {
			return areasFile, 1
		}
		d = doc{lines: strings.Split(string(data), "\n")}
		r.docs[areasFile] = d
	}
	return areasFile, max(areas.SkillLine(d.lines, key), 1)
}

// leakageView is the leakage view of the run for Report.Leakage: the leaks reported and the messages of the
// AR9I0 findings of the [[lint.accept]] entries for the leakage rules.
func (r *runner) leakageView() *leakageResult {
	out := &leakageResult{leakageRun: r.leakage}
	for _, s := range r.staleAccepts {
		if s.code == CodeExposureAreaTerm || s.code == CodeExposureAreaAmbiguous {
			out.stale = append(out.stale, s.message)
		}
	}
	return out
}

// Leakage is the `ai-rulez exposure leakage` view of the run: every AR9W0 and AR9W1 finding, in the order the
// source found them, with its severity and acceptance, and the [[lint.accept]] entries of those rules that
// accepted nothing (AR9I0). A leak the run did not report (the rule off or ignored, an ignored path, an inline
// ignore) is left out. It is nil when the rules read no leakage: no source, no area map, or the exposure
// analyzer did not run.
func (rep *Report) Leakage() *LeakageResult {
	x := rep.leakage
	if x == nil || !x.ran {
		return nil
	}
	type key struct {
		code, file string
		line       int
		message    string
	}
	found := make(map[key]*Finding, len(rep.Findings))
	for i := range rep.Findings {
		f := &rep.Findings[i]
		found[key{f.Code, f.File, f.Line, f.Message}] = f
	}
	out := &LeakageResult{
		Checked:   x.checked,
		Leaks:     []LeakFinding{},
		Unchecked: append([]ExposureUnchecked{}, x.unchecked...),
		Stale:     append([]string{}, x.stale...),
	}
	for i := range x.leaks {
		rec := &x.leaks[i]
		f, ok := found[key{rec.code, rec.file, max(rec.leak.Line, 1), rec.leak.Message}]
		if !ok {
			continue
		}
		lf := LeakFinding{ExposureLeak: rec.leak, Code: rec.code, Severity: f.Severity}
		if f.Meta != nil && f.Meta.Accepted {
			lf.Accepted, lf.Reason = true, f.Meta.AcceptReason
		}
		out.Leaks = append(out.Leaks, lf)
	}
	return out
}
