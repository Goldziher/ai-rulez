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

	CodeExposureMapInvalid = "AR9W5"
)

func registerArExposure(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeExposureMapInvalid, "exposure-map-invalid", SeverityError, "the [exposure] table or its area map is invalid: a duplicate or malformed area name, a bad path claim, a skill_areas value naming no area, an unreadable areas_file, or both areas_file and inline areas"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeExposureMapInvalid: {
			Why:  "The area map decides which area owns a skill and which words name an area. A map with a problem is not used at all, so the exposure rules that read it stay silent until it is fixed; this finding is the only sign that they are off.",
			Bad:  "Two `[[area]]` tables named `billing`, a path claim `\"services/billing/\"` with a trailing slash, or `[skill_areas] deploy = \"ops\"` with no area `ops`",
			Good: "Unique lower-kebab names, repository-relative claims such as `\"services/billing_*\"` or `\"!services/billing_legacy\"`, and skill_areas values that name an area",
		},
	})
	SetAnalyzer(CodeExposureMapInvalid, AnalyzerExposure, ScopeBundle)
	s.addRunCheck(checkExposureMap, AnalyzerExposure)
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
