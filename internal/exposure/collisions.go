package exposure

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// SchemaCollisions is the schema_version of a CollisionReport
// (schema/exposure-collisions.v1.schema.json).
const SchemaCollisions = 1

// DefaultCollisionMinScore is the word overlap at which `exposure collisions`
// reports a pair, and AR9W2's when it is on without [exposure]
// collision_min_score.
const DefaultCollisionMinScore = config.ExposureDefaultCollisionMinScore

// DefaultCollisionLimit is the number of pairs `exposure collisions` prints,
// and AR9W2 reports one by one.
const DefaultCollisionLimit = config.ExposureDefaultCollisionLimit

// Collision is two start-of-session listings (skills or agents) of different
// areas whose words overlap: Score is the Jaccard index of their word sets and
// Shared the words they have in common, sorted. First is the artifact that
// comes first in the inventory.
type Collision struct {
	Score      float64  `json:"score"`
	Shared     []string `json:"shared"`
	First      string   `json:"first"`
	FirstKind  Kind     `json:"first_kind"`
	FirstName  string   `json:"first_name"`
	FirstArea  string   `json:"first_area"`
	Second     string   `json:"second"`
	SecondKind Kind     `json:"second_kind"`
	SecondName string   `json:"second_name"`
	SecondArea string   `json:"second_area"`
}

// CollisionReport ranks the collisions of an inventory: what
// `ai-rulez exposure collisions` prints.
type CollisionReport struct {
	SchemaVersion int        `json:"schema_version"`
	Root          string     `json:"root"`
	Plugins       PluginMode `json:"plugins"`
	MinScore      float64    `json:"min_score"`
	// ArtifactsCompared is the number of start-of-session skills and agents.
	ArtifactsCompared int `json:"artifacts_compared"`
	// Total is the number of pairs at or over MinScore, before the limit.
	Total      int         `json:"total"`
	Collisions []Collision `json:"collisions"`
}

// NewCollisionReport classifies a copy of the inventory with the map and ranks
// its collisions at or over minScore, keeping the first limit (all when limit
// is 0 or less).
func NewCollisionReport(inv *Inventory, m *areas.Map, minScore float64, limit int) *CollisionReport {
	classified := *inv
	classified.Artifacts = append([]Artifact(nil), inv.Artifacts...)
	classified.Classify(m)
	compared, all := FindCollisions(classified.Artifacts, minScore)
	kept := all
	if limit > 0 && len(kept) > limit {
		kept = kept[:limit]
	}
	return &CollisionReport{
		SchemaVersion:     SchemaCollisions,
		Root:              inv.Root,
		Plugins:           inv.Plugins.Mode,
		MinScore:          minScore,
		ArtifactsCompared: compared,
		Total:             len(all),
		Collisions:        kept,
	}
}

// FindCollisions compares every pair of classified artifacts (see
// Inventory.Classify) that are skills or agents in context at session start
// (PhaseStart, and PhaseUnknown as Inventory.Start counts it): a pair of
// different areas (Unclassified being one) that shares a word and whose Jaccard
// index is at least minScore collides. The words of an artifact are the
// lowercase runs of an ASCII letter and two or more ASCII letters or digits in
// its qualified name and listing text, less a list of stopwords. The result is
// ranked by score, highest first, then by the first and the second path, and
// compared is the number of listings compared.
func FindCollisions(arts []Artifact, minScore float64) (compared int, collisions []Collision) {
	type profile struct {
		a     *Artifact
		words map[string]bool
	}
	var listed []profile
	for i := range arts {
		if a := &arts[i]; (a.Phase == PhaseStart || a.Phase == PhaseUnknown) && (a.Kind == KindSkill || a.Kind == KindAgent) {
			listed = append(listed, profile{a, listingWords(a)})
		}
	}
	collisions = []Collision{}
	for i := range listed {
		for j := i + 1; j < len(listed); j++ {
			a, b := listed[i], listed[j]
			if a.a.Area == b.a.Area {
				continue
			}
			shared := sharedWords(a.words, b.words)
			if len(shared) == 0 {
				continue
			}
			score := float64(len(shared)) / float64(len(a.words)+len(b.words)-len(shared))
			if score < minScore {
				continue
			}
			collisions = append(collisions, Collision{
				Score: score, Shared: shared,
				First: a.a.Path, FirstKind: a.a.Kind, FirstName: a.a.QualifiedName, FirstArea: a.a.Area,
				Second: b.a.Path, SecondKind: b.a.Kind, SecondName: b.a.QualifiedName, SecondArea: b.a.Area,
			})
		}
	}
	sort.SliceStable(collisions, func(i, j int) bool {
		x, y := &collisions[i], &collisions[j]
		if x.Score != y.Score {
			return x.Score > y.Score
		}
		if x.First != y.First {
			return x.First < y.First
		}
		return x.Second < y.Second
	})
	return len(listed), collisions
}

var collisionWordRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`[a-z][a-z0-9]{2,}`) })

// listingWords is the word set of a listing: the qualified name and the
// listing text (which repeats it), lowercased as Python's str.lower does.
func listingWords(a *Artifact) map[string]bool {
	text := strings.ToLower(strings.ReplaceAll(a.QualifiedName+" "+a.ListingText, "\u0130", "i\u0307"))
	words := map[string]bool{}
	for _, w := range collisionWordRe().FindAllString(text, -1) {
		if !isStopword(w) {
			words[w] = true
		}
	}
	return words
}

func sharedWords(a, b map[string]bool) []string {
	var shared []string
	for w := range a {
		if b[w] {
			shared = append(shared, w)
		}
	}
	sort.Strings(shared)
	return shared
}

func sortedWords(words map[string]bool) []string {
	out := make([]string, 0, len(words))
	for w := range words {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// isStopword reports the words too common in listings to tell two apart.
func isStopword(w string) bool {
	switch w {
	case "the", "and", "for", "with", "from", "into", "that", "this", "these", "those", "when", "use", "used", "using",
		"not", "any", "all", "can", "are", "was", "has", "have", "its", "their", "your", "you", "our", "also", "via",
		"per", "etc", "each", "such", "than", "then", "them", "they", "will", "would", "should", "only", "other",
		"task", "tasks", "user", "users", "need", "needs", "needed", "instead", "without", "within", "about", "over",
		"under", "before", "after":
		return true
	}
	return false
}
