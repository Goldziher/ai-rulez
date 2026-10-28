package lint

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Session exposure and areas (AR9W, docs/exposure.md). Every rule is inert unless the configuration has an
// [exposure] table, except AR9W4 (ar_exposure_gate.go), which reads only the item.
const (
	// AnalyzerExposure is the analyzer name of the session exposure and area codes.
	AnalyzerExposure = "exposure"

	CodeExposureStartBudget = "AR9W3"
	CodeExposureMapInvalid  = "AR9W5"
)

// ExposureStart is what a Claude Code session started at the repository root
// loads, as `ai-rulez exposure` measures it over the native inventory with the
// [exposure] plugins and context_window.
type ExposureStart struct {
	// ContextWindow is the context window the listing budget was computed for.
	ContextWindow int
	// StartArtifacts and StartTokens are the artifacts and estimated tokens in
	// context at session start.
	StartArtifacts int
	StartTokens    int
	// ListingSkills and ListingChars are the start-of-session skills and the
	// characters of their listing lines; ListingBudgetChars is Claude Code's
	// listing budget for ContextWindow.
	ListingSkills      int
	ListingChars       int
	ListingBudgetChars int
}

// ExposureSource reads the session exposure of the project (the Claude Code
// inventory classified with the [exposure] area map). The exposure rules call
// it only when the configuration has an [exposure] table; without one handed
// in (WithExposure), they stay silent.
type ExposureSource interface {
	// StartExposure is the start-of-session exposure (AR9W3, AR964
	// claude-skill-listing-total); nil when the inventory cannot be built.
	StartExposure() *ExposureStart
	// Collisions are the pairs at or over minScore, highest score first; none
	// when the area map cannot be used (AR9W5 reports it).
	Collisions(minScore float64) []ExposureCollision
}

// WithExposure hands the exposure rules the session exposure of the project.
func WithExposure(src ExposureSource) Option { return func(r *runner) { r.exposure = src } }

// exposureStart is the start-of-session exposure, nil without an [exposure]
// table or a source.
func (r *runner) exposureStart() *ExposureStart {
	if r.cfg == nil || r.cfg.Exposure == nil || r.exposure == nil {
		return nil
	}
	return r.exposure.StartExposure()
}

// exposureStartBudgetSeverity turns AR9W3 on at error when [exposure]
// start_budget is set.
func (r *runner) exposureStartBudgetSeverity() {
	if r.cfg != nil && r.cfg.Exposure != nil && r.cfg.Exposure.StartBudget > 0 {
		r.sev[CodeExposureStartBudget] = SeverityError
	}
}

func registerArExposure(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeExposureStartBudget, "exposure-start-budget", SeverityOff, "the estimated tokens a Claude Code session loads at start exceed [exposure] start_budget (setting it turns the rule on at error)"},
		RuleInfo{CodeExposureMapInvalid, "exposure-map-invalid", SeverityError, "the [exposure] table or its area map is invalid: a duplicate or malformed area name, a bad path claim, a skill_areas value naming no area, an unreadable areas_file, or both areas_file and inline areas"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeExposureStartBudget: {
			Why:  "Everything a session loads before it touches a file (the root CLAUDE.md, unconditional rules, the skill and agent listings, MCP server names) is paid for by every session, whatever it works on. A budget turns growth of that surface into a decision instead of drift. The estimate is the one `ai-rulez exposure` reports.",
			Bad:  "`[exposure] start_budget = 4000` while the root CLAUDE.md and the skill listing add up to about 5200 estimated tokens",
			Good: "Move area-specific guidance into a subdirectory CLAUDE.md or a `paths:`-gated rule, so it loads on demand, or raise start_budget on purpose",
		},
		CodeExposureMapInvalid: {
			Why:  "The area map decides which area owns a skill and which words name an area. A map with a problem is not used at all, so the exposure rules that read it stay silent until it is fixed; this finding is the only sign that they are off.",
			Bad:  "Two `[[area]]` tables named `billing`, a path claim `\"services/billing/\"` with a trailing slash, or `[skill_areas] deploy = \"ops\"` with no area `ops`",
			Good: "Unique lower-kebab names, repository-relative claims such as `\"services/billing_*\"` or `\"!services/billing_legacy\"`, and skill_areas values that name an area",
		},
	})
	SetAnalyzer(CodeExposureMapInvalid, AnalyzerExposure, ScopeBundle)
	SetAnalyzer(CodeExposureStartBudget, AnalyzerExposure, ScopeBundle)
	s.addRunCheck(checkExposureMap, AnalyzerExposure)
	s.addRunCheck(checkExposureStartBudget, AnalyzerExposure)
}

// checkExposureStartBudget reports start-of-session tokens over [exposure]
// start_budget (AR9W3).
func checkExposureStartBudget(r *runner) {
	if r.cfg == nil || r.cfg.Exposure == nil || r.cfg.Exposure.StartBudget <= 0 || r.sev[CodeExposureStartBudget] == SeverityOff {
		return
	}
	start := r.exposureStart()
	if start == nil || start.StartTokens <= r.cfg.Exposure.StartBudget {
		return
	}
	budget := r.cfg.Exposure.StartBudget
	path, line := r.exposureLine("start_budget")
	r.add(CodeExposureStartBudget, path, line,
		"a session started at the repository root loads about %d estimated tokens (%d artifacts), %d past the [exposure] start_budget of %d; run `ai-rulez exposure` for the heaviest artifacts",
		start.StartTokens, start.StartArtifacts, start.StartTokens-budget, budget)
}

// checkExposureMap reports an invalid [exposure] table and area map (AR9W5).
func checkExposureMap(r *runner) {
	if r.cfg == nil || r.cfg.Exposure == nil {
		return
	}
	for _, problem := range r.cfg.Exposure.Validate() {
		path, line := r.exposureLine("")
		r.add(CodeExposureMapInvalid, path, line, "%s", problem)
	}
	_, problems := areas.Load(r.cfg.Exposure, r.rootAbs())
	for _, p := range problems {
		if p.File == "" {
			path, line := r.exposureLine(p.Key)
			if lines := r.docs[path].lines; p.LineIn(lines) > 0 {
				line = p.LineIn(lines)
			}
			r.add(CodeExposureMapInvalid, path, line, "exposure: %s", p.Message)
			continue
		}
		if _, ok := r.docs[p.File]; !ok {
			if data, err := safefs.ReadFileLimited(p.File, areas.MaxFileBytes); err == nil {
				r.docs[p.File] = doc{lines: strings.Split(string(data), "\n")}
			}
		}
		r.add(CodeExposureMapInvalid, p.File, p.Line, "exposure: %s", p.Message)
	}
}

// exposureLine finds the config line holding key inside the [exposure] tables, else the [exposure] header.
func (r *runner) exposureLine(key string) (path string, line int) {
	return r.configLineWhere(func(lines []string) int {
		header, inside := 0, false
		for i, l := range lines {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "[") {
				name := strings.Trim(t, "[] ")
				inside = name == "exposure" || strings.HasPrefix(name, "exposure.")
				if inside && header == 0 {
					header = i + 1
				}
				continue
			}
			if inside && key != "" && strings.Contains(l, key) {
				return i + 1
			}
		}
		return header
	})
}
