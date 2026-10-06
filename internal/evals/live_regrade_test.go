package evals

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/stretchr/testify/require"
)

// Live re-grade of the transcripts a earlier TestLiveBuiltinGraderAgainstClaudeOwnGrading run
// saved, with another judge model: the answers are reused, so only the (cheap) judge calls
// are paid for. Gated like the comparison. AI_RULEZ_LIVE_REGRADE is the saved
// grader-comparison.json; AI_RULEZ_LIVE_GRADER_MODEL the model (default gemini-2.5-flash);
// AI_RULEZ_LIVE_GRADER_IN and _OUT its list prices in USD per million tokens.
func TestLiveRegradeSavedTranscripts(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_LLM") != "1" {
		t.Skip("set AI_RULEZ_LIVE_LLM=1 to run the live grader comparison")
	}
	saved, configDir := os.Getenv("AI_RULEZ_LIVE_REGRADE"), os.Getenv("AI_RULEZ_LIVE_EVALS_PROJECT")
	if saved == "" || configDir == "" || os.Getenv(liveGraderKey) == "" {
		t.Skipf("set AI_RULEZ_LIVE_REGRADE, AI_RULEZ_LIVE_EVALS_PROJECT and %s", liveGraderKey)
	}
	model := os.Getenv("AI_RULEZ_LIVE_GRADER_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}
	data, err := os.ReadFile(saved)
	require.NoError(t, err)
	var report liveGraderReport
	require.NoError(t, json.Unmarshal(data, &report))

	rubrics := map[string]string{}
	skills, err := FindSkills(configDir)
	require.NoError(t, err)
	for i := range skills {
		cases, problems := LoadCases(&skills[i])
		require.Empty(t, problems)
		for _, c := range Expand(cases) {
			rubrics[skills[i].ID+"/"+c.ID] = c.RubricText()
		}
	}

	cfg := llm.Config{Provider: "gemini", Model: model, APIKeyEnv: liveGraderKey, AllowNetwork: true, Cache: livePtr(false), MaxRetries: 2,
		TimeoutSeconds: 120, MaxCostUSD: 0.30, PriceInputPerMTok: envFloat("AI_RULEZ_LIVE_GRADER_IN", 0.30), PriceOutputPerMTok: envFloat("AI_RULEZ_LIVE_GRADER_OUT", 2.50)}
	managed, err := llm.New(cfg, llm.Options{Getenv: os.Getenv})
	require.NoError(t, err)
	t.Cleanup(func() { _ = managed.Close() })
	grader := &JudgeGrader{Client: managed, Model: model, Spent: func() float64 { return managed.Spent().CostUSD }}

	out := &liveGraderReport{Backend: managed.Backend, GraderModel: model, PassMark: DefaultRubricMinScore}
	for _, row := range report.Rows {
		rubric, ok := rubrics[row.Skill+"/"+row.Case]
		require.True(t, ok, "%s/%s", row.Skill, row.Case)
		grade, err := grader.Grade(context.Background(), rubric, row.Answer)
		require.NoError(t, err)
		row.Score, row.Rationale = grade.Score, grade.Rationale
		row.BuiltinPass = grade.Score >= DefaultRubricMinScore
		row.Agree = row.BuiltinPass == row.ClaudePass
		out.Rows = append(out.Rows, row)
	}
	summarizeLive(out, grader, managed.Spent().Calls)
	out.ClaudeCostUSD = report.ClaudeCostUSD

	encoded, err := json.MarshalIndent(out, "", "  ")
	require.NoError(t, err)
	t.Logf("regrade with %s: agreement %.2f, kappa %.2f, builtin pass %.2f, grader cost $%.4f", model, out.Agreement, out.Kappa, out.BuiltinPass, out.GraderCostUSD)
	if dir := os.Getenv("AI_RULEZ_LIVE_OUT"); dir != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "grader-regrade-"+model+".json"), encoded, 0o600))
	}
}
