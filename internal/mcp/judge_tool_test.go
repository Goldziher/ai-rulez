package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

func stubJudger(report *judge.Report, err error, got *handlers.JudgeParams) handlers.Judger {
	return func(_ context.Context, p handlers.JudgeParams) (*judge.Report, error) {
		if got != nil {
			*got = p
		}
		return report, err
	}
}

func judgeReport(mode, decision string) *judge.Report {
	return &judge.Report{
		SchemaVersion: judge.ReportSchemaVersion, Mode: mode,
		Run:     judge.RunInfo{Models: map[string]string{"security": "bedrock/m"}},
		Units:   []judge.UnitReport{{ID: "agents-md:AGENTS.md", Kind: judge.KindAgentsMD, Path: "AGENTS.md", Status: "pass", Findings: []*judge.Finding{}}},
		Summary: judge.Summary{Decision: decision, BySeverity: map[string]int{}, ByCategory: map[string]int{}},
	}
}

func TestJudgeToolPassesOnlyWhatTheUserMaySet(t *testing.T) {
	t.Parallel()
	var got handlers.JudgeParams
	srv := NewServer("test", WithAnyDirectory(), WithJudger(stubJudger(judgeReport("judge", "pass"), nil, &got)))

	res := call(t, connect(t, srv), "judge", map[string]any{
		"working_directory": t.TempDir(), "paths": []string{"AGENTS.md"}, "categories": []string{"security"},
		"fail_on": []string{"security:medium"}, "since": "HEAD", "impact": false, "allow_llm": true, "estimate": false,
		"skills":    []map[string]any{{"name": "style", "inline_markdown": "## C1 - a\nb"}, {"path": "rubrics/x"}},
		"skill_for": map[string]any{"quality": []map[string]any{{"name": "q"}}},
	})

	require.False(t, res.IsError, textOfResult(t, res))
	assert.Equal(t, []string{"AGENTS.md"}, got.Paths)
	assert.Equal(t, []string{"security:medium"}, got.FailOn)
	assert.Equal(t, "HEAD", got.Since)
	require.NotNil(t, got.Impact)
	assert.False(t, *got.Impact)
	assert.Nil(t, got.AttachRules, "unset stays unset, so the configuration decides")
	assert.True(t, got.AllowLLM)
	require.Len(t, got.Skills, 2)
	assert.Equal(t, "style", got.Skills[0].Name)
	assert.Equal(t, "rubrics/x", got.Skills[1].Path)
	assert.Equal(t, "q", got.SkillFor["quality"][0].Name)
	assert.Equal(t, "judge", structured(t, res)["mode"])
}

func TestJudgeToolRefusesAModelArgument(t *testing.T) {
	t.Parallel()
	srv := NewServer("test", WithAnyDirectory(), WithJudger(stubJudger(judgeReport("judge", "pass"), nil, nil)))
	session := connect(t, srv)

	for _, arg := range []string{"model", "provider", "max_cost", "base_url", "api_key_env"} {
		res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "judge", Arguments: map[string]any{arg: "x"}})
		assert.True(t, err != nil || res.IsError, "%s must not be a tool argument", arg)
	}
}

func TestJudgeToolTurnsAFailedOrIncompleteRunIntoAnErrorResultWithTheReport(t *testing.T) {
	t.Parallel()
	for _, decision := range []string{"fail", "incomplete"} {
		t.Run(decision, func(t *testing.T) {
			srv := NewServer("test", WithAnyDirectory(), WithJudger(stubJudger(judgeReport("judge", decision), nil, nil)))

			res := call(t, connect(t, srv), "judge", map[string]any{"working_directory": t.TempDir()})

			require.True(t, res.IsError)
			assert.Equal(t, decision, structured(t, res)["summary"].(map[string]any)["decision"])
		})
	}
	t.Run("an estimate is a success", func(t *testing.T) {
		srv := NewServer("test", WithAnyDirectory(), WithJudger(stubJudger(judgeReport("estimate", "pass"), nil, nil)))

		res := call(t, connect(t, srv), "judge", map[string]any{"working_directory": t.TempDir(), "estimate": true})

		assert.False(t, res.IsError)
	})
}

func TestJudgeToolWithoutAnEngineSaysSo(t *testing.T) {
	t.Parallel()
	res := call(t, connect(t, NewServer("test", WithAnyDirectory())), "judge", map[string]any{"working_directory": t.TempDir()})

	assert.True(t, res.IsError)
	assert.Contains(t, textOfResult(t, res), "judge engine")
}
