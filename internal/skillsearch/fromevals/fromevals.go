// Package fromevals derives skill search cases from the eval-runner cases of a
// project: they are free labelled queries. A prompt with expect_trigger true for
// skill S becomes a case that expects S; a near-miss prompt (or any prompt with
// expect_trigger false) becomes a case whose avoid list is S, i.e. S must not
// rank first for it.
package fromevals

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

// Tag marks a derived case.
const Tag = "from-evals"

// Derived is the outcome of Derive.
type Derived struct {
	Cases []skillsearch.Case
	// Problems are malformed eval files and cases that cannot be used; none stops the derivation.
	Problems []string
}

// Derive reads the eval cases under configDir (the .ai-rulez directory) and
// turns them into search cases, ordered by skill and case.
func Derive(configDir string) (*Derived, error) {
	skills, err := evals.FindSkills(configDir)
	if err != nil {
		return nil, fmt.Errorf("find skills: %w", err)
	}
	out := &Derived{}
	for i := range skills {
		cases, problems := evals.LoadCases(&skills[i])
		for _, p := range problems {
			out.Problems = append(out.Problems, p.String())
		}
		for n, c := range evals.Expand(cases) {
			prompt := strings.TrimSpace(c.Prompt)
			if prompt == "" || c.ExpectTrigger == nil {
				continue
			}
			id := c.ID
			if id == "" {
				id = fmt.Sprintf("case-%d", n+1)
			}
			sc := skillsearch.Case{ID: skills[i].ID + "/" + id, Query: prompt, Tags: derivedTags(c.Tags)}
			if c.Expects() {
				sc.Expect = []skillsearch.Relevant{{ID: skills[i].ID, Grade: 1}}
			} else {
				sc.Avoid = []string{skills[i].ID}
			}
			out.Cases = append(out.Cases, sc)
		}
	}
	return out, nil
}

func derivedTags(tags []string) []string {
	out := []string{Tag}
	for _, t := range tags {
		if t != Tag {
			out = append(out, t)
		}
	}
	return out
}
