package lint

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// CodeExposureAreaCollision is AR9W2: two start-of-session listings of different areas overlap.
const CodeExposureAreaCollision = "AR9W2"

// exposureListingBytes bounds the skill or agent file read for the line of its description, as the exposure
// inventory bounds the files it reads.
const exposureListingBytes = 1 << 20

// DefaultExposureCollisionMinScore is the AR9W2 threshold when the rule is turned on by a severity and
// [exposure] collision_min_score is unset; `ai-rulez exposure collisions` defaults to the same.
const DefaultExposureCollisionMinScore = config.ExposureDefaultCollisionMinScore

// DefaultExposureCollisionLimit is the most pairs AR9W2 reports one by one, the highest scores; the rest are
// counted in one summary finding. `ai-rulez exposure collisions` prints as many by default.
const DefaultExposureCollisionLimit = config.ExposureDefaultCollisionLimit

// ExposureCollision is a pair of skills or agents listed at session start, of different areas, whose listing
// words overlap: Score is their Jaccard index and Shared the common words, sorted. First and Second are slash
// paths relative to the source's Root; Kind is "skill" or "agent" and Name the qualified name.
type ExposureCollision struct {
	Score      float64
	Shared     []string
	First      string
	FirstKind  string
	FirstName  string
	FirstArea  string
	Second     string
	SecondKind string
	SecondName string
	SecondArea string
}

func registerArExposureCollisions(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeExposureAreaCollision, "exposure-area-collision", SeverityOff, "two skills or agents listed at session start, of different areas, overlap in the words of their listings at or over [exposure] collision_min_score (setting it turns the rule on at warning); the 20 highest pairs are reported, the rest in one summary finding"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeExposureAreaCollision: {
			Why:  "The model picks a skill or agent by its listing. Two listings of different areas that share most of their words compete for the same prompts, so a session working on one area can load the other's instructions. `ai-rulez exposure collisions` ranks every pair.",
			Bad:  "A billing skill `charge` described as \"Look up customer invoice details by name\" next to a search skill `rank` described as \"Look up customer ranker details by name\"",
			Good: "Descriptions that lead with what sets each apart: \"Refund or void a billing invoice\" and \"Tune the search ranker weights\"",
		},
	})
	SetAnalyzer(CodeExposureAreaCollision, AnalyzerExposure, ScopeBundle)
	s.addRunCheck(checkExposureCollisions, AnalyzerExposure)
}

// exposureCollisionSeverity turns AR9W2 on at warning when [exposure] collision_min_score is set.
func (r *runner) exposureCollisionSeverity() {
	if r.cfg != nil && r.cfg.Exposure != nil && r.cfg.Exposure.CollisionMinScore > 0 {
		r.sev[CodeExposureAreaCollision] = SeverityWarning
	}
}

// checkExposureCollisions reports the pairs of start-of-session listings of different areas whose word overlap
// is at least collision_min_score (DefaultExposureCollisionMinScore when the rule is on without it), on the
// description line of the pair's first listing (AR9W2). The DefaultExposureCollisionLimit highest pairs are
// reported one by one and the rest in one summary finding on the config, at the same severity. Each finding
// depends on the listings it names and on the source's inputs (the area map, the configuration), so --changed
// keeps it when either listing or the map changed. An invalid table is AR9W5's and leaves the rule silent.
func checkExposureCollisions(r *runner) {
	if r.exposure == nil || r.cfg == nil || r.cfg.Exposure == nil || r.sev[CodeExposureAreaCollision] == SeverityOff {
		return
	}
	exp := r.cfg.Exposure
	if len(exp.Validate()) > 0 {
		return
	}
	minScore := exp.CollisionMinScore
	if minScore <= 0 {
		minScore = DefaultExposureCollisionMinScore
	}
	root := r.exposureRoot()
	abs := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	collisions := r.exposure.Collisions(minScore)
	shown := collisions[:min(len(collisions), DefaultExposureCollisionLimit)]
	for i := range shown {
		c := &shown[i]
		first, second := abs(c.First), abs(c.Second)
		before := len(r.findings)
		r.add(CodeExposureAreaCollision, first, descriptionLine(first),
			"%s %q of area %q and %s %q of area %q list overlapping words at session start (score %.2f, at least %.2f): %s",
			c.FirstKind, c.FirstName, c.FirstArea, c.SecondKind, c.SecondName, c.SecondArea, c.Score, minScore,
			strings.Join(c.Shared, ", "))
		r.exposureDeps(before, first, []string{second})
	}
	rest := collisions[len(shown):]
	if len(rest) == 0 {
		return
	}
	var listings []string
	for i := range rest {
		listings = append(listings, abs(rest[i].First), abs(rest[i].Second))
	}
	path, line := r.exposureLine("collision_min_score")
	before := len(r.findings)
	r.add(CodeExposureAreaCollision, path, line,
		"and %d more %s of start-of-session listings of different areas overlap at or over %.2f, past the %d reported one by one; see `ai-rulez exposure collisions`",
		len(rest), plural(len(rest), "pair", "pairs"), minScore, len(shown))
	r.exposureDeps(before, path, listings)
}

// descriptionLine is the line of the description key in the frontmatter of the file at abs, else 1.
func descriptionLine(abs string) int {
	data, err := safefs.ReadFileLimited(abs, exposureListingBytes)
	if err != nil {
		return 1
	}
	return parseDoc(string(data)).lineOf("description:", 1)
}
