package improve

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

// Split methods recorded in the plan.
const (
	SplitByTag      = "tag"
	SplitByFraction = "fraction"
)

// MinHoldoutCases is the fewest scored held-out cases `improve` accepts.
const MinHoldoutCases = 3

// Split is the deterministic division of a skill's authored eval cases into
// the cases the optimizer sees (Train) and the referee (Held). A near-miss
// expansion always stays with its parent because the split works on authored
// cases and expands each side afterwards.
type Split struct {
	Method   string
	Tag      string
	Fraction float64
	Train    []evals.Case
	Held     []evals.Case
}

// SplitCases divides authored cases. Cases carrying tag are held out; when none
// does, a case is held out when hash(id) mod 100 < fraction*100. The result
// depends only on the ids, the tags and the settings.
func SplitCases(cases []evals.Case, tag string, fraction float64) Split {
	s := Split{Method: SplitByTag, Tag: tag, Fraction: fraction}
	tagged := false
	for i := range cases {
		if slices.Contains(cases[i].Tags, tag) {
			tagged = true
			break
		}
	}
	if !tagged {
		s.Method = SplitByFraction
	}
	for i := range cases {
		c := cases[i]
		var held bool
		if tagged {
			held = slices.Contains(c.Tags, tag)
		} else {
			held = bucket(c.ID) < int(fraction*100)
		}
		if held {
			s.Held = append(s.Held, c)
		} else {
			s.Train = append(s.Train, c)
		}
	}
	return s
}

// bucket maps a case id to 0..99, stable across machines and releases.
func bucket(id string) int {
	sum := sha256.Sum256([]byte(id))
	return int(binary.BigEndian.Uint32(sum[:4]) % 100)
}

// Negatives counts expanded cases that must not trigger the skill.
func Negatives(expanded []evals.Case) int {
	n := 0
	for i := range expanded {
		if !expanded[i].Expects() {
			n++
		}
	}
	return n
}

// IDs lists the expanded case ids of authored cases.
func IDs(authored []evals.Case) []string {
	expanded := evals.Expand(authored)
	ids := make([]string, len(expanded))
	for i := range expanded {
		ids[i] = expanded[i].ID
	}
	return ids
}

// DuplicatePrompts lists train prompts that repeat a held-out prompt once
// whitespace and case are normalised: the optimizer could then see a held-out
// prompt through its train copy.
func DuplicatePrompts(train, held []evals.Case) []string {
	seen := map[string]bool{}
	for _, c := range evals.Expand(held) {
		seen[normalizePrompt(c.Prompt)] = true
	}
	var dup []string
	for _, c := range evals.Expand(train) {
		if seen[normalizePrompt(c.Prompt)] {
			dup = append(dup, c.ID)
		}
	}
	return dup
}

func normalizePrompt(p string) string {
	return strings.ToLower(strings.Join(strings.Fields(p), " "))
}
