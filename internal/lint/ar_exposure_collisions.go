package lint

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// CodeExposureAreaCollision is AR9W2: two start-of-session listings of different areas overlap.
const CodeExposureAreaCollision = "AR9W2"

// exposureListingBytes bounds the skill or agent file read for the line of its description, as the exposure
// inventory bounds the files it reads.
const exposureListingBytes = 1 << 20

// DefaultExposureCollisionMinScore is the AR9W2 threshold when the rule is turned on by a severity and
// [exposure] collision_min_score is unset; `ai-rulez exposure collisions` defaults to the same.
const DefaultExposureCollisionMinScore = 0.1

// ExposureCollision is a pair of skills or agents listed at session start, of different areas, whose listing
// words overlap: Score is their Jaccard index and Shared the common words, sorted. First and Second are
// repository-relative slash paths; Kind is "skill" or "agent" and Name the qualified name.
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
		RuleInfo{CodeExposureAreaCollision, "exposure-area-collision", SeverityOff, "two skills or agents listed at session start, of different areas, overlap in the words of their listings at or over [exposure] collision_min_score (setting it turns the rule on at warning)"},
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

// checkExposureCollisions reports each pair of start-of-session listings of different areas whose word overlap
// is at least collision_min_score (DefaultExposureCollisionMinScore when the rule is on without it), on the
// description line of the pair's first listing (AR9W2). An invalid table is AR9W5's and leaves the rule silent.
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
	root := r.rootAbs()
	collisions := r.exposure.Collisions(minScore)
	for i := range collisions {
		c := &collisions[i]
		abs := filepath.Join(root, filepath.FromSlash(c.First))
		r.add(CodeExposureAreaCollision, abs, descriptionLine(abs),
			"%s %q of area %q and %s %q of area %q list overlapping words at session start (score %.2f, at least %.2f): %s",
			c.FirstKind, c.FirstName, c.FirstArea, c.SecondKind, c.SecondName, c.SecondArea, c.Score, minScore,
			strings.Join(c.Shared, ", "))
	}
}

// descriptionLine is the line of the description key in the frontmatter of the file at abs, else 1.
func descriptionLine(abs string) int {
	data, err := safefs.ReadFileLimited(abs, exposureListingBytes)
	if err != nil {
		return 1
	}
	return parseDoc(string(data)).lineOf("description:", 1)
}
