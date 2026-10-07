package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const evalCases = `schema_version: 1
cases:
  - id: basic
    prompt: Deploy billing to staging
    expect_trigger: true
    assertions:
      - type: contains
        value: staging-eu
  - id: unrelated
    prompt: What is the capital of France?
    expect_trigger: false
`

// Command runners: one answers every case correctly, one misses both, one
// prints garbage.
const (
	runnerPass    = "#!/bin/sh\ncat > /dev/null\necho '{\"version\":1,\"results\":[{\"case\":\"basic\",\"arm\":\"with\",\"triggered\":true,\"output\":\"deployed to staging-eu\",\"cost_usd\":0.01},{\"case\":\"unrelated\",\"arm\":\"with\",\"triggered\":false,\"output\":\"Paris\"}],\"cost_usd\":0.01}'\n"
	runnerFail    = "#!/bin/sh\ncat > /dev/null\necho '{\"version\":1,\"results\":[{\"case\":\"basic\",\"arm\":\"with\",\"triggered\":true,\"output\":\"deployed to prod\"},{\"case\":\"unrelated\",\"arm\":\"with\",\"triggered\":true,\"output\":\"Paris\"}]}'\n"
	runnerGarbage = "#!/bin/sh\ncat > /dev/null\necho 'not json'\n"
)

func evalProject(t *testing.T) (root, runners string) {
	t.Helper()
	root = minimalProject(t, "")
	writeTree(t, root, map[string]string{".ai-rulez/skills/deploy/evals/deploy.eval.yaml": evalCases})
	runners = t.TempDir()
	writeExec(t, filepath.Join(runners, "pass.sh"), runnerPass)
	writeExec(t, filepath.Join(runners, "fail.sh"), runnerFail)
	writeExec(t, filepath.Join(runners, "garbage.sh"), runnerGarbage)
	return root, runners
}

type evalRunJSON struct {
	Skills []struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Passing bool   `json:"passing"`
		Score   *struct {
			PassRate float64 `json:"pass_rate"`
			Errors   int     `json:"errors"`
		} `json:"score"`
	} `json:"skills"`
	DryRun bool `json:"dry_run"`
	Failed bool `json:"failed"`
}

func TestEvalRunE2E(t *testing.T) {
	tests := []struct {
		name        string
		runner      string
		extra       []string
		wantExit    int
		wantStatus  string
		wantPassing bool
		wantResults bool
	}{
		{name: "dry run calls no runner and records nothing", runner: "garbage.sh", extra: []string{"--dry-run"}, wantStatus: "dry-run"},
		{name: "a passing skill exits 0 and is recorded", runner: "pass.sh", wantStatus: "ran", wantPassing: true, wantResults: true},
		{name: "a failing skill exits 2", runner: "fail.sh", wantExit: 2, wantStatus: "ran", wantResults: true},
		{name: "--no-write keeps the results file away", runner: "fail.sh", extra: []string{"--no-write"}, wantExit: 2, wantStatus: "ran"},
		{name: "an unreadable runner response is an errored run, exit 2", runner: "garbage.sh", wantExit: 2},
		{name: "an unknown runner cannot run", runner: "pass.sh", extra: []string{"--runner", "nope"}, wantExit: 1},
		{name: "a NaN cost cap is refused", runner: "pass.sh", extra: []string{"--max-cost", "NaN"}, wantExit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			root, runners := evalProject(t)
			args := append([]string{"eval", "run", "--runner-command", filepath.Join(runners, tt.runner), "--format", "json"}, tt.extra...)

			// Act
			res := env.run(root, args...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			_, statErr := os.Stat(filepath.Join(root, ".ai-rulez", "eval-results.json"))
			assert.Equal(t, tt.wantResults, statErr == nil, "results file")
			if tt.wantExit == 1 {
				return
			}
			requireJSONDoc(t, res)
			var doc evalRunJSON
			require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc))
			require.Len(t, doc.Skills, 1)
			if tt.wantStatus != "" {
				assert.Equal(t, tt.wantStatus, doc.Skills[0].Status)
			}
			assert.Equal(t, tt.wantPassing, doc.Skills[0].Passing)
		})
	}
}

// TestEvalResultsCacheAndReportE2E: a recorded passing run is replayed from the
// signed cache, `report evals` reads it, and a results file signed with
// another user's key is treated as unverified.
func TestEvalResultsCacheAndReportE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root, runners := evalProject(t)
	first := env.run(root, "eval", "run", "--runner-command", filepath.Join(runners, "pass.sh"), "--format", "json")
	require.Equal(t, 0, first.ExitCode, first.Stderr)

	// Act: the second run with unchanged inputs is answered from the signed cache.
	cached := env.run(root, "eval", "run", "--runner-command", filepath.Join(runners, "pass.sh"), "--format", "json")
	report := env.run(root, "report", "evals", "--format", "json")
	other := newIsoEnv(t)
	foreign := other.run(root, "report", "evals", "--format", "json")

	// Assert
	require.Equal(t, 0, cached.ExitCode, cached.Stderr)
	var doc evalRunJSON
	require.NoError(t, json.Unmarshal([]byte(cached.Stdout), &doc))
	require.Len(t, doc.Skills, 1)
	assert.Equal(t, "cached", doc.Skills[0].Status)

	require.Equal(t, 0, report.ExitCode, report.Stderr)
	rep := requireJSONDoc(t, report)
	skills, ok := rep["skills"].([]any)
	require.True(t, ok, report.Stdout)
	require.Len(t, skills, 1)
	row := skills[0].(map[string]any) //nolint:forcetypeassert // shape checked by the schema test
	assert.Equal(t, "deploy", row["id"])
	assert.Equal(t, float64(1), row["pass_rate"])
	assert.Nil(t, row["unverified"])

	require.Equal(t, 0, foreign.ExitCode, foreign.Stderr)
	frep := requireJSONDoc(t, foreign)
	frow := frep["skills"].([]any)[0].(map[string]any) //nolint:forcetypeassert // as above
	assert.Equal(t, true, frow["unverified"], "a record signed by another user's key is not trusted")
	assert.Nil(t, frow["pass_rate"])
}

func TestReportUsageE2E(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantOut  string
	}{
		{name: "a log with one use lists the skill as used", args: []string{"report", "usage", "usage.jsonl", "--format", "json"}, wantOut: "deploy"},
		{name: "a missing log cannot be read", args: []string{"report", "usage", "missing.jsonl"}, wantExit: 1},
		{name: "the log is required", args: []string{"report", "usage"}, wantExit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			root := minimalProject(t, "\n[usage]\nskills_index = true\n")
			require.Equal(t, 0, env.run(root, "generate", "--yes").ExitCode, "generate writes the skills index")
			writeTree(t, root, map[string]string{"usage.jsonl": `{"v":3,"ts":"2026-10-03T08:00:00Z","event":"skill_invoked","skill":"deploy","id":"deploy","event_id":"0123456789abcdef","invocation":"slash","harness":"claude"}` + "\n"})

			// Act
			res := env.run(root, tt.args...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			if tt.wantExit == 0 {
				requireJSONDoc(t, res)
				assert.Contains(t, res.Stdout, tt.wantOut)
			}
		})
	}
}
