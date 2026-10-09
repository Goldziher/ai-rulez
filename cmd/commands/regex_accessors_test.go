package commands

import (
	"regexp"
	"testing"
)

// Every regular expression this package compiles lazily with sync.OnceValue is
// compiled here once, so a pattern that does not parse fails the test run instead
// of panicking at its first use in production. tests/archlint fails when an
// accessor is missing from this list.
func TestRegexAccessorsCompile(t *testing.T) {
	accessors := map[string]func() *regexp.Regexp{
		"allowCodeRe":            allowCodeRe,
		"approveCodePattern":     approveCodePattern,
		"approveReviewerPattern": approveReviewerPattern,
	}
	for name, get := range accessors {
		if get() == nil {
			t.Errorf("%s compiled to nil", name)
		}
	}
}
