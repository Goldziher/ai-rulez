package testutil

import (
	"os"
	"testing"
)

func TestScrubCIEnv_UnsetsTheCIVariablesForTheTestOnly(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantGone bool
	}{
		{name: "the CI flag", key: "CI", wantGone: true},
		{name: "a GitHub Actions context variable", key: "GITHUB_WORKFLOW_REF", wantGone: true},
		{name: "the Actions OIDC request URL", key: "ACTIONS_ID_TOKEN_REQUEST_URL", wantGone: true},
		{name: "a runner variable", key: "RUNNER_OS", wantGone: true},
		{name: "an unrelated variable", key: "AI_RULEZ_TEST_KEEP", wantGone: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			t.Setenv(tc.key, "from-the-runner")

			// Act
			t.Run("scrubbed", func(t *testing.T) {
				ScrubCIEnv(t)
				_, present := os.LookupEnv(tc.key)

				// Assert
				if present == tc.wantGone {
					t.Fatalf("%s present = %v after ScrubCIEnv, want %v", tc.key, present, !tc.wantGone)
				}
			})

			// Assert: the value is back once the test that scrubbed it ends.
			if got := os.Getenv(tc.key); got != "from-the-runner" {
				t.Fatalf("%s = %q after the scrubbing test ended, want it restored", tc.key, got)
			}
		})
	}
}
