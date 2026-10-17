package config

import "testing"

// TestRegexAccessorsCompile forces every lazily compiled pattern, so a bad one
// fails here and not in production.
func TestRegexAccessorsCompile(t *testing.T) {
	_ = githubShorthand()
}
