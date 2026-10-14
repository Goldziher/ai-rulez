package commands

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJudgeSummaryCountsCallsAndTokensWithoutABudget(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n", "sub/AGENTS.md": "Keep it short.\n"})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--max-cost", "0", "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	var doc struct {
		Summary struct {
			Calls   int     `json:"calls"`
			Tokens  int     `json:"tokens"`
			CostUSD float64 `json:"cost_usd"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, 2, doc.Summary.Calls)
	assert.Positive(t, doc.Summary.Tokens)
	assert.Positive(t, doc.Summary.CostUSD, "a priced model has a cost even without a cap")
}

func TestJudgeCapMessagesKeepTheirPrecision(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--max-cost", "0.0004")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "--max-cost $0.0004", "a small cap is not shown as $0.00")
}

func TestJudgeAnnouncesASmallCapExactly(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--max-cost", "0.005")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Contains(t, rig.stderr.String(), "spend capped at $0.0050")
}
