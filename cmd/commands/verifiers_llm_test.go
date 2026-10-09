package commands

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const verifiersLLMSpec = verifiersBase + "[[verifiers]]\nname = \"actionable\"\nrule = \"r\"\nseverity = \"error\"\nwhen_changed = [\"*.txt\"]\n" +
	"[verifiers.require.llm]\nchecklist = [\"Errors say what to do.\"]\n"

func verifiersLLMProject(t *testing.T, extra string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersLLMSpec+extra)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "r.md"), "# R\n\nErrors say what to do.\n")
	writeFile(t, filepath.Join(root, "a.txt"), "x\n")
	chdir(t, root)
	// Never read a real user config or key while testing.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "")
}

func TestRunVerifiers_LLMVerifierIsSkippedVisiblyNotPassed(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
		want  string
	}{
		{"no flag", func() {}, "pass --allow-llm"},
		{"flag without allow_network", func() { verifiersAllowLLM = true }, "allow_network is not enabled in the user config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVerifiersFlags(t)
			verifiersLLMProject(t, "")
			tt.setup()
			var out bytes.Buffer

			got := reported(runVerifiers(context.Background(), &out))

			assert.Equal(t, 0, got, out.String())
			assert.Contains(t, out.String(), "skipped")
			assert.Contains(t, out.String(), tt.want)
			assert.NotContains(t, out.String(), "1 passed", "a skipped llm verifier is never counted as a pass")
		})
	}
}

func TestRunVerifiers_RepositoryConfigCannotTurnOnTheNetwork(t *testing.T) {
	resetVerifiersFlags(t)
	verifiersLLMProject(t, "\n[llm]\nprovider = \"gemini\"\nmodel = \"gemini-2.5-flash\"\nallow_network = true\n")
	verifiersAllowLLM = true
	var out bytes.Buffer

	got := reported(runVerifiers(context.Background(), &out))

	assert.Equal(t, 0, got, out.String())
	assert.Contains(t, out.String(), "allow_network is not enabled in the user config")
}

func TestRunVerifiers_EstimateCallsNothingAndPrintsTheManifest(t *testing.T) {
	resetVerifiersFlags(t)
	verifiersLLMProject(t, "\n[llm]\nprovider = \"gemini\"\nmodel = \"gemini-2.5-flash\"\n")
	verifiersEstimate = true
	var out bytes.Buffer

	got := reported(runVerifiers(context.Background(), &out))

	assert.Equal(t, 0, got, out.String())
	assert.Contains(t, out.String(), "estimate: 1 call(s) to gemini/gemini-2.5-flash")
	assert.Contains(t, out.String(), "nothing was sent")
}

func TestRunVerifiers_BrokenLLMSettingsOnlyMatterWhenAnLLMVerifierRuns(t *testing.T) {
	tests := []struct {
		name        string
		llmVerifier bool
		want        int
	}{
		{"no llm verifier", false, 0},
		{"llm verifier needs the config", true, exitVerifiersCannotRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetVerifiersFlags(t)
			verifiersLLMProject(t, "")
			if !tt.llmVerifier {
				root, err := os.Getwd()
				require.NoError(t, err)
				writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase)
			}
			t.Setenv("AI_RULEZ_LLM_MAX_CALLS", "abc")
			var out bytes.Buffer

			// Act
			got := reported(runVerifiers(context.Background(), &out))

			// Assert
			assert.Equal(t, tt.want, got, out.String())
		})
	}
}

func TestRunVerifiers_RejectsANegativeOrNonFiniteMaxCost(t *testing.T) {
	for _, v := range []float64{-1, math.NaN(), math.Inf(1)} {
		t.Run(fmt.Sprint(v), func(t *testing.T) {
			resetVerifiersFlags(t)
			verifiersLLMProject(t, "")
			verifiersMaxCost = v
			var out bytes.Buffer

			got := reported(runVerifiers(context.Background(), &out))

			assert.Equal(t, exitVerifiersCannotRun, got)
		})
	}
}

func TestSuggestVerifiers_RefusesWithoutAModelAndEstimateCallsNothing(t *testing.T) {
	resetSuggestFlags(t)
	verifiersLLMProject(t, "\n[llm]\nprovider = \"gemini\"\nmodel = \"gemini-2.5-flash\"\n")
	var out bytes.Buffer

	off := reported(suggestVerifiers(context.Background(), "r", &out))
	verifiersEstimate = true
	est := reported(suggestVerifiers(context.Background(), "r", &out))

	assert.Equal(t, exitVerifiersCannotRun, off, "without --allow-llm there is nothing to suggest with")
	assert.Equal(t, 0, est)
	assert.Contains(t, out.String(), "estimate: 1 call to gemini/gemini-2.5-flash")
	assert.Contains(t, out.String(), "Verifier suggestions for rule \"r\"")
	entries, _ := os.ReadDir(filepath.Join(".ai-rulez", "verifiers"))
	assert.Empty(t, entries, "a dry run writes nothing")
}

func TestSuggestVerifiers_FlagChecks(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
	}{
		{"unknown kind", func() { suggestKind = "hook" }},
		{"write with estimate", func() { suggestWrite, verifiersEstimate = true, true }},
		{"unknown id", func() { verifiersEstimate = true }},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSuggestFlags(t)
			verifiersLLMProject(t, "")
			tt.setup()
			id := "r"
			if i == 2 {
				id = "ghost"
			}

			got := reported(suggestVerifiers(context.Background(), id, &bytes.Buffer{}))

			assert.Equal(t, exitVerifiersCannotRun, got)
		})
	}
}

func resetSuggestFlags(t *testing.T) {
	t.Helper()
	resetVerifiersFlags(t)
	reset := func() { suggestKind, suggestMaxProposals, suggestWrite, suggestFormat = "rule", 5, false, false }
	reset()
	t.Cleanup(reset)
}
