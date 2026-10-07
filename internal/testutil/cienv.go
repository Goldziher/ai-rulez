package testutil

import (
	"os"
	"strings"
	"testing"
)

// ciEnvPrefixes name the variables a CI runner (GitHub Actions in particular)
// sets for every step, CI itself included.
var ciEnvPrefixes = []string{"GITHUB_", "ACTIONS_", "RUNNER_"}

// ScrubCIEnv unsets, for the rest of the test, every variable a CI runner sets
// (CI, GITHUB_*, ACTIONS_*, RUNNER_*). Code that reads them (the provenance
// builder from GITHUB_WORKFLOW_REF, the OIDC token request) otherwise behaves
// one way on a laptop and another in CI. A test that wants one sets it with
// t.Setenv after calling this. The variables come back when the test ends.
func ScrubCIEnv(tb testing.TB) {
	tb.Helper()
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !isCIVar(key) {
			continue
		}
		tb.Setenv(key, "") // registers the restore
		if err := os.Unsetenv(key); err != nil {
			tb.Fatalf("unset %s: %v", key, err)
		}
	}
}

func isCIVar(key string) bool {
	if key == "CI" {
		return true
	}
	for _, p := range ciEnvPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}
