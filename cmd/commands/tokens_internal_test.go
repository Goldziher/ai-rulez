// Package-internal tests for the tokens command. runTokens is unexported on
// purpose: it returns the budget verdict instead of calling os.Exit, which is what
// makes the threshold mode testable without spawning a subprocess.
package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokensFixturePath is the committed fixture the generator tests also use.
func tokensFixturePath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "config", "tokens", ".ai-rulez", "config.yaml"))
	require.NoError(t, err)
	return path
}

// resetTokensFlags restores the package-level flag values between cases, which
// cobra would otherwise carry from one test into the next.
func resetTokensFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		tokensJSON = false
		tokensBudget = 0
		tokensCompareProfiles = nil
		tokensTokenizer = tokens.CounterCL100KBase
		profile = ""
		configDir = ""
	})
	tokensJSON = false
	tokensBudget = 0
	tokensCompareProfiles = nil
	tokensTokenizer = tokens.CounterCL100KBase
	profile = ""
	configDir = ""
}

func TestTokensCommand_FlagSurface(t *testing.T) {
	flags := TokensCmd.Flags()
	for name, shorthand := range map[string]string{
		"json":       "j",
		"budget":     "b",
		"profile":    "p",
		"config-dir": "n",
	} {
		flag := flags.Lookup(name)
		require.NotNil(t, flag, "tokens is missing --%s", name)
		assert.Equal(t, shorthand, flag.Shorthand, "tokens --%s", name)
	}
	assert.NotNil(t, flags.Lookup("compare-profiles"))
	assert.Equal(t, tokens.CounterCL100KBase, flags.Lookup("tokenizer").DefValue,
		"the default counter must be the offline tokenizer, not a byte ratio")
}

func TestRunTokens_TextReport(t *testing.T) {
	resetTokensFlags(t)
	profile = "backend"

	var out bytes.Buffer
	overBudget, err := runTokens(&out, []string{tokensFixturePath(t)})
	require.NoError(t, err)
	assert.False(t, overBudget)

	report := out.String()
	assert.Contains(t, report, `profile "backend"`)
	assert.Contains(t, report, "cl100k_base")
	assert.Contains(t, report, "always loaded")
	assert.Contains(t, report, "skill descriptions")
	assert.Contains(t, report, "skill bodies")
	assert.Contains(t, report, "Headline always-loaded surface")
	assert.Contains(t, report, "never predicts a session total",
		"the report has to state what it cannot know")
}

func TestRunTokens_Budget(t *testing.T) {
	tests := []struct {
		name       string
		budget     int
		overBudget bool
		contains   string
	}{
		{name: "within", budget: 1_000_000, overBudget: false, contains: "within budget"},
		{name: "exceeded", budget: 1, overBudget: true, contains: "Over budget"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTokensFlags(t)
			profile = "backend"
			tokensBudget = tt.budget

			var out bytes.Buffer
			overBudget, err := runTokens(&out, []string{tokensFixturePath(t)})
			require.NoError(t, err)
			assert.Equal(t, tt.overBudget, overBudget)
			assert.Contains(t, out.String(), tt.contains)
		})
	}
}

func TestRunTokens_JSON(t *testing.T) {
	resetTokensFlags(t)
	profile = "backend"
	tokensJSON = true

	var out bytes.Buffer
	_, err := runTokens(&out, []string{tokensFixturePath(t)})
	require.NoError(t, err)

	var report map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &report),
		"a single profile serializes as one object, not a list")
	assert.Equal(t, "backend", report["profile"])
	assert.NotEmpty(t, report["runtimes"])
	assert.NotEmpty(t, report["notes"])
}

// TestRunTokens_CompareProfiles covers the multi-profile path, which renders every
// profile in one process. Each profile needs its own Generator: collectOutputs
// writes SourceHash onto the config, so a shared one would carry a stale hash.
func TestRunTokens_CompareProfiles(t *testing.T) {
	resetTokensFlags(t)
	tokensCompareProfiles = []string{"backend", "full"}
	tokensJSON = true

	var out bytes.Buffer
	_, err := runTokens(&out, []string{tokensFixturePath(t)})
	require.NoError(t, err)

	var reports []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &reports))
	require.Len(t, reports, 2)
	assert.Equal(t, "backend", reports[0]["profile"])
	assert.Equal(t, "full", reports[1]["profile"])

	backend := reports[0]["headline_always"].(float64)
	full := reports[1]["headline_always"].(float64)
	assert.Greater(t, full, backend, "the full profile carries both domains")
}

func TestRunTokens_CompareProfilesTable(t *testing.T) {
	resetTokensFlags(t)
	tokensCompareProfiles = []string{"backend", "frontend"}

	var out bytes.Buffer
	_, err := runTokens(&out, []string{tokensFixturePath(t)})
	require.NoError(t, err)

	table := out.String()
	assert.Contains(t, table, "Token surface comparison")
	assert.Contains(t, table, "backend")
	assert.Contains(t, table, "frontend")
	assert.Contains(t, table, "headline")
}

// TestRunTokens_CompareComposedProfiles covers the case composition exists for:
// comparing a shared base against that base plus a role's extra domains. Each
// column is one --compare-profiles occurrence, which is why the flag is a string
// array rather than a comma-splitting slice — a comma composes here.
func TestRunTokens_CompareComposedProfiles(t *testing.T) {
	resetTokensFlags(t)
	tokensCompareProfiles = []string{"backend", "backend,frontend"}
	tokensJSON = true

	var out bytes.Buffer
	_, err := runTokens(&out, []string{tokensFixturePath(t)})
	require.NoError(t, err)

	var reports []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &reports))
	require.Len(t, reports, 2)
	assert.Equal(t, "backend", reports[0]["profile"])
	assert.Equal(t, "backend,frontend", reports[1]["profile"])
	assert.Greater(t, reports[1]["headline_always"].(float64), reports[0]["headline_always"].(float64),
		"composing the frontend profile in adds its rule to the always-loaded surface")
}

func TestTokensCommand_CompareProfilesDoesNotSplitOnComma(t *testing.T) {
	resetTokensFlags(t)
	flag := TokensCmd.Flags().Lookup("compare-profiles")
	require.NotNil(t, flag)
	require.NoError(t, flag.Value.Set("base,backend"))
	// A string array appends once it has been set, so clear the flag's own state
	// too or the next test to touch it inherits this value.
	t.Cleanup(func() { flag.Changed = false })
	assert.Equal(t, []string{"base,backend"}, tokensCompareProfiles,
		"a comma composes profiles, so it must not be read as a separator between columns")
}

func TestRunTokens_RejectsUnknownTokenizer(t *testing.T) {
	resetTokensFlags(t)
	tokensTokenizer = "gpt-9"

	_, err := runTokens(&bytes.Buffer{}, []string{tokensFixturePath(t)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gpt-9")
}

func TestHumanCount(t *testing.T) {
	tests := []struct {
		value    int
		expected string
	}{
		{value: 0, expected: "0"},
		{value: 7, expected: "7"},
		{value: 999, expected: "999"},
		{value: 1000, expected: "1,000"},
		{value: 22326, expected: "22,326"},
		{value: 1234567, expected: "1,234,567"},
		{value: -4879, expected: "-4,879"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, humanCount(tt.value))
	}
}
