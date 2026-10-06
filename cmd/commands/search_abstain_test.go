package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const abstainCases = `version: 1
k: 3
cases:
  - {id: p1, query: "reimburse me", expect: [refund-policy]}
  - {id: p2, query: "cluster rollout", expect: [deploy-staging]}
  - {id: p3, query: "pull request conventions", expect: [git-workflow]}
  - {id: n1, query: "weather tomorrow", expect: []}
  - {id: n2, query: "sourdough starter", expect: []}
`

// indexedSearchProject builds the fake-provider index and leaves the flag state clean.
func indexedSearchProject(t *testing.T) {
	t.Helper()
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	code, _, errOut := runIndex(t)
	require.Equal(t, 0, code, errOut)
	resetSearch(t)
}

func TestSearchEval_CalibratesTheAbstentionThreshold(t *testing.T) {
	// Arrange
	indexedSearchProject(t)
	setSearchFlag(t, "eval", writeCases(t, abstainCases))
	setSearchFlag(t, "mode", "vector")
	setSearchFlag(t, "format", "json")

	// Act
	code, out, errOut := execSearch(t)

	// Assert
	require.Equal(t, 0, code, errOut)
	var res struct {
		Calibration *struct {
			Mode               string  `json:"mode"`
			MinSim             float64 `json:"min_sim"`
			PositivesKept      int     `json:"positives_kept"`
			NegativesAbstained int     `json:"negatives_abstained"`
		} `json:"calibration"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.NotNil(t, res.Calibration, out)
	assert.Equal(t, "vector", res.Calibration.Mode)
	assert.Positive(t, res.Calibration.MinSim)
	assert.Equal(t, 3, res.Calibration.PositivesKept)
	assert.Equal(t, 2, res.Calibration.NegativesAbstained)
	validateAgainst(t, "../../schema/search-eval.v1.schema.json", []byte(out))
}

func TestSearchEval_TextShowsTheCalibration(t *testing.T) {
	indexedSearchProject(t)
	setSearchFlag(t, "eval", writeCases(t, abstainCases))
	setSearchFlag(t, "mode", "vector")

	code, out, errOut := execSearch(t)

	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "calibration (vector): vector_min_sim = ")
	assert.Contains(t, out, "answers 3 of 3 positive cases and abstains on 2 of 2 negative cases")
}

func TestSearchQuery_AbstainsBelowTheConfiguredThreshold(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantAbstained bool
	}{
		{"nothing resembles the query", "weather tomorrow", true},
		{"a skill is close", "reimburse me", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			indexedSearchProject(t)
			cfg := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "ai-rulez", "config.toml")
			body, err := os.ReadFile(cfg) //nolint:gosec // a test path
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cfg, append(body, []byte("vector_min_sim = 0.6\n")...), 0o600))
			setSearchFlag(t, "format", "json")

			// Act
			code, out, errOut := execSearch(t, tt.query)

			// Assert
			require.Equal(t, 0, code, errOut)
			var doc searchDoc
			require.NoError(t, json.Unmarshal([]byte(out), &doc))
			assert.Equal(t, tt.wantAbstained, doc.Abstained)
			assert.Equal(t, !tt.wantAbstained, len(doc.Results) > 0)
			validateAgainst(t, "../../schema/search.v1.schema.json", []byte(out))
		})
	}
}

func TestSearchQuery_TextExplainsAnAbstention(t *testing.T) {
	indexedSearchProject(t)
	cfg := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "ai-rulez", "config.toml")
	body, err := os.ReadFile(cfg) //nolint:gosec // a test path
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg, append(body, []byte("vector_min_sim = 0.6\n")...), 0o600))

	code, out, _ := execSearch(t, "weather tomorrow")

	require.Equal(t, 0, code)
	assert.Contains(t, out, "no served skill is a confident match")
	assert.Contains(t, out, "vector_min_sim 0.600")
}
