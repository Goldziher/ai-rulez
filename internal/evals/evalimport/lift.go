package evalimport

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

// lifted is one assertion extracted from a criterion.
type lifted struct {
	criterion string
	assertion evals.Assertion
}

// The patterns are deliberately narrow: a clearly mechanical sentence, with the
// path or the text quoted, so a wrong lift cannot silently change what is graded.
// A criterion that does not match stays a rubric criterion only.
var (
	quoted = `["'` + "`" + `]([^"'` + "`" + `\n]{1,200})["'` + "`" + `]`
	// file "x" exists / is created / is present
	liftFileExists = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)^(?:the\s+)?(?:file|path)\s+` + quoted + `\s+(?:exists|is\s+created|is\s+present|gets\s+created)\.?$`)
	})
	// file "x" does not exist / is not created
	liftFileAbsent = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)^(?:the\s+)?(?:file|path)\s+` + quoted + `\s+(?:does\s+not\s+exist|is\s+not\s+created|is\s+absent)\.?$`)
	})
	// the output/answer/response contains "y"
	liftContains = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)^(?:the\s+)?(?:output|answer|response|final\s+answer)\s+(?:contains|includes|mentions)\s+` + quoted + `\.?$`)
	})
	// the output/answer/response does not contain "y"
	liftNotContains = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)^(?:the\s+)?(?:output|answer|response|final\s+answer)\s+(?:does\s+not|doesn'?t)\s+(?:contain|include|mention)\s+` + quoted + `\.?$`)
	})
)

// liftAssertions returns the assertions the criteria state in a fixed phrasing.
func liftAssertions(criteria []criterion) []lifted {
	var out []lifted
	for _, c := range criteria {
		text := strings.TrimSpace(c.body())
		if a, ok := liftOne(text); ok {
			out = append(out, lifted{criterion: c.label(), assertion: a})
		}
	}
	return out
}

func liftOne(text string) (evals.Assertion, bool) {
	if m := liftFileAbsent().FindStringSubmatch(text); len(m) > 1 && evals.CheckRelPath(m[1]) == "" {
		no := false
		return evals.Assertion{Type: evals.AssertFileExists, Path: m[1], Exists: &no}, true
	}
	if m := liftFileExists().FindStringSubmatch(text); len(m) > 1 && evals.CheckRelPath(m[1]) == "" {
		return evals.Assertion{Type: evals.AssertFileExists, Path: m[1]}, true
	}
	if m := liftNotContains().FindStringSubmatch(text); m != nil {
		return evals.Assertion{Type: evals.AssertNotContains, Value: m[1]}, true
	}
	if m := liftContains().FindStringSubmatch(text); m != nil {
		return evals.Assertion{Type: evals.AssertContains, Value: m[1]}, true
	}
	return evals.Assertion{}, false
}

// describeAssertion renders an assertion for the report.
func describeAssertion(a evals.Assertion) string {
	if a.Type == evals.AssertFileExists {
		if a.Exists != nil && !*a.Exists {
			return fmt.Sprintf("file_exists %q (absent)", a.Path)
		}
		return fmt.Sprintf("file_exists %q", a.Path)
	}
	return fmt.Sprintf("%s %q", a.Type, a.Value)
}
