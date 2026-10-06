package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rankingGolden = "testdata/find_skill_ranking.golden.json"

// TestFindSkillRanking_Golden pins find_skill's lexical ranking (order and
// scores) so the ranker can move packages without changing a single result.
// Regenerate with UPDATE_GOLDEN=1 only for an intended ranking change.
func TestFindSkillRanking_Golden(t *testing.T) {
	t.Parallel()
	// Arrange
	cat := rankCatalog(t,
		[3]string{"db-migrations", "Plan and run database schema changes safely", "schema change,alembic migration"},
		[3]string{"git-workflow", "Branching, commit messages and pull request conventions", "open a pull request,rebase"},
		[3]string{"pdf-processing", "Extract text and fill forms in PDF documents", "fill a form"},
		[3]string{"refund-policy", "Process customer refund requests and chargebacks", "customer wants money back"},
		[3]string{"incident-response", "Run an incident, triage, mitigate and write the postmortem", "production is down,outage"},
		[3]string{"data-migration", "Moves records between stores", ""},
		[3]string{"deploy-staging", "Deploy a service to the staging cluster", "deploy to staging,roll out"},
		[3]string{"deploy-prod", "Deploy a service to production with approvals", "ship to production"},
	)
	queries := []string{
		"I need to run a schema migration on the users table",
		"open a pull request for this branch",
		"the customer wants their money back",
		"production is down",
		"fill in this PDF form",
		"refunds", "migrations", "moving record", "deploy", "deploy to production",
		"the of and", "zebra crossing", "",
	}

	// Act
	got := map[string][]string{}
	for _, q := range queries {
		rows := []string{}
		for _, h := range bm25Rank(cat.Skills(), q) {
			rows = append(rows, fmt.Sprintf("%s %.6f", h.Skill.Name, h.Score))
		}
		got[q] = rows
	}

	// Assert
	if os.Getenv("UPDATE_GOLDEN") != "" {
		data, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(rankingGolden), 0o755))
		require.NoError(t, os.WriteFile(rankingGolden, append(data, '\n'), 0o644))
	}
	raw, err := os.ReadFile(rankingGolden)
	require.NoError(t, err)
	var want map[string][]string
	require.NoError(t, json.Unmarshal(raw, &want))
	assert.Equal(t, want, got)
}
