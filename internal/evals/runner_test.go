package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleRequest(t *testing.T) *Request {
	t.Helper()
	skillDir := filepath.Join(t.TempDir(), "deploy")
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "evals"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: deploy\n---\nbody"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "evals", "secret.eval.yaml"), []byte("x"), 0o600))
	return &Request{
		Version: ProtocolVersion, Harness: "claude", Ablation: true,
		Skill: SkillRef{ID: "deploy", Dir: skillDir, Digest: "sha256:abc"},
		Cases: []Case{
			{ID: "fires", Prompt: "deploy it", ExpectTrigger: bp(true), Tags: []string{"smoke"}, Assertions: []Assertion{
				{Type: AssertContains, Value: "a.b (c)\nd"},
				{Type: AssertNotContains, Value: "error"},
				{Type: AssertRegex, Value: `done \d+`, Path: "out.txt"},
				{Type: AssertFileExists, Path: "x.log", Exists: bp(false)},
			}, Rubric: "names the cluster"},
			{ID: "quiet", Prompt: "unrelated", ExpectTrigger: bp(false)},
			{ID: "needs-exec", Prompt: "p", ExpectTrigger: bp(true), Assertions: []Assertion{{Type: AssertCommandExit, Command: "true"}}},
			{ID: "fixtures", Prompt: "p", ExpectTrigger: bp(true), Files: []Fixture{{Path: "a", Content: "b"}}},
		},
	}
}

func TestCommandRunner_PipesRequestAndReadsResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the runner is a POSIX shell script")
	}
	req := sampleRequest(t)
	dir := t.TempDir()
	captured := filepath.Join(dir, "request.json")
	script := filepath.Join(dir, "runner.sh")
	// Saves stdin, then answers for every case in the request via a canned reply.
	reply := `{"version":1,"results":[{"case":"fires","arm":"with","triggered":true,"output":"ok"},{"case":"fires","arm":"without","output":"ok"},{"case":"quiet","arm":"with","triggered":false}]}`
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncat > "+captured+"\necho \"$AI_RULEZ_EVAL_PROTOCOL $AI_RULEZ_EVAL_SKILL\" >&2\nprintf '%s' '"+reply+"'\n"), 0o700))

	var stderr bytes.Buffer
	runner := &CommandRunner{Command: script, Stderr: &stderr}
	resp, err := runner.Run(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Results, 3)
	assert.Equal(t, "1 deploy", strings.TrimSpace(stderr.String()))

	var got Request
	data, err := os.ReadFile(captured)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "deploy", got.Skill.ID)
	assert.True(t, got.Ablation)
	assert.Len(t, got.Cases, 4)
	assert.Equal(t, "deploy it", got.Cases[0].Prompt)
	assert.NotContains(t, string(data), `"File"`, "authoring paths are not part of the protocol")
}

// printJSON returns a shell command that prints doc verbatim under both sh and
// cmd.exe, which keeps single quotes in an echo argument.
func printJSON(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.json")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))
	if runtime.GOOS == "windows" {
		return `type "` + path + `"`
	}
	return "cat '" + path + "'"
}

func TestCommandRunner_Errors(t *testing.T) {
	req := sampleRequest(t)
	_, err := (&CommandRunner{}).Run(context.Background(), req)
	assert.ErrorContains(t, err, "--runner-command")

	_, err = (&CommandRunner{Command: "exit 4"}).Run(context.Background(), req)
	assert.ErrorContains(t, err, "runner command failed")

	_, err = (&CommandRunner{Command: "echo not-json"}).Run(context.Background(), req)
	assert.ErrorContains(t, err, "invalid JSON")

	_, err = (&CommandRunner{Command: printJSON(t, `{"version":2,"results":[]}`)}).Run(context.Background(), req)
	assert.ErrorContains(t, err, "protocol version")

	_, err = (&CommandRunner{Command: printJSON(t, `{"version":1,"results":[{"case":"ghost","arm":"with"}]}`)}).Run(context.Background(), req)
	assert.ErrorContains(t, err, "unknown case")

	_, err = (&CommandRunner{Command: printJSON(t, `{"version":1,"results":[{"case":"fires","arm":"sideways"}]}`)}).Run(context.Background(), req)
	assert.ErrorContains(t, err, "arm must be")
}

func TestCommandRunner_TimeoutNamesTheLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the runner is a POSIX shell script")
	}
	req := sampleRequest(t)

	_, err := (&CommandRunner{Command: "sleep 5", Timeout: 200 * time.Millisecond}).Run(context.Background(), req)

	require.Error(t, err)
	assert.ErrorContains(t, err, "timed out after")
	assert.NotContains(t, err.Error(), "signal: killed")
}

func TestBuildClaudePlugin_TranslatesCases(t *testing.T) {
	req := sampleRequest(t)
	dir := t.TempDir()
	tr, err := BuildClaudePlugin(dir, req)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".claude-plugin", "plugin.json"))
	assert.FileExists(t, filepath.Join(dir, "skills", "deploy", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, "skills", "deploy", "evals", "secret.eval.yaml"), "cases are not part of the plugin under test")

	assert.Contains(t, tr.Skipped["needs-exec"], "command_exit")
	assert.Contains(t, tr.Skipped["fixtures"], "fixtures")
	assert.Equal(t, map[string]string{"fires": "fires", "quiet": "quiet"}, tr.Dirs)

	read := func(parts ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{dir, "evals"}, parts...)...))
		require.NoError(t, err)
		return string(data)
	}
	prompt := read("fires", "prompt.md")
	assert.Contains(t, prompt, "deploy it")
	assert.Contains(t, prompt, "- Skill")
	assert.Contains(t, prompt, "- smoke")

	trigger := read("fires", "graders", "trigger.md")
	assert.Contains(t, trigger, "type: tool_used")
	// the id is anchored: "deploy" must not match "deploy-prod" or "redeploy"
	match := regexp.MustCompile(`input_match: (.+)`).FindStringSubmatch(trigger)
	require.NotNil(t, match, trigger)
	re := regexp.MustCompile(strings.Trim(match[1], `"'`))
	for _, hit := range []string{`deploy`, `{"skill":"deploy"}`, `{"skill":"ai-rulez-eval-deploy:deploy"}`} {
		assert.True(t, re.MatchString(hit), hit)
	}
	for _, miss := range []string{`deploy-prod`, `{"skill":"redeploy"}`, `{"skill":"plugin:deploy.v2"}`, `{"skill":"deployer"}`} {
		assert.False(t, re.MatchString(miss), miss)
	}
	assert.NotContains(t, trigger, "max: 0")

	quiet := read("quiet", "graders", "trigger.md")
	assert.Contains(t, quiet, "max: 0")
	assert.Contains(t, quiet, "arm: both")

	assert.Contains(t, read("fires", "graders", "assert-01.md"), `a\.b \(c\)\nd`)
	assert.Contains(t, read("fires", "graders", "assert-02.md"), "match: not_contains")
	third := read("fires", "graders", "assert-03.md")
	assert.Contains(t, third, "path: out.txt")
	assert.Contains(t, third, `done \d+`)
	fourth := read("fires", "graders", "assert-04.md")
	assert.Contains(t, fourth, "type: file_exists")
	assert.Contains(t, fourth, "exists: false")
	rubric := read("fires", "graders", "rubric.md")
	assert.Contains(t, rubric, "type: llm")
	assert.Contains(t, rubric, "names the cluster")
}

func claudeRunJSON(trigger, outcome bool, cost float64, errMsg string) map[string]any {
	run := map[string]any{"score": 1, "costUsd": cost, "judgeCostUsd": 0.01, "error": nil, "graders": []map[string]any{
		{"name": "trigger", "passed": trigger, "withOnly": true},
		{"name": "assert-01", "passed": outcome},
	}}
	if errMsg != "" {
		run["error"] = errMsg
	}
	return run
}

func TestParseClaudeResult_MajorityVotesAndInversion(t *testing.T) {
	req := sampleRequest(t)
	tr := &ClaudeTranslation{
		Dirs:     map[string]string{"fires": "fires", "quiet": "quiet"},
		Skipped:  map[string]string{"needs-exec": "unsupported", "fixtures": "unsupported"},
		Inverted: map[string]bool{"quiet": true},
	}
	doc, err := json.Marshal(map[string]any{"costUsd": 0.5, "cases": []map[string]any{
		{"name": "fires", "arms": map[string]any{
			"with":    []any{claudeRunJSON(true, true, 0.1, ""), claudeRunJSON(true, false, 0.1, ""), claudeRunJSON(false, true, 0.1, "")},
			"without": []any{claudeRunJSON(false, false, 0.1, ""), claudeRunJSON(false, false, 0.1, "")},
		}},
		// the inverted trigger grader passes when the skill stayed quiet; here it failed in 2 of 3 runs
		{"name": "quiet", "arms": map[string]any{
			"with": []any{claudeRunJSON(false, true, 0, ""), claudeRunJSON(false, true, 0, ""), claudeRunJSON(true, true, 0, "")},
		}},
	}})
	require.NoError(t, err)

	resp, err := ParseClaudeResult(doc, req, tr)
	require.NoError(t, err)
	require.NoError(t, resp.Validate(&Request{Cases: []Case{{ID: "fires"}, {ID: "quiet"}, {ID: "needs-exec"}, {ID: "fixtures"}}}))

	byKey := map[string]Result{}
	for _, r := range resp.Results {
		byKey[r.Case+"/"+r.Arm] = r
	}
	fires := byKey["fires/with"]
	assert.True(t, *fires.Triggered, "2 of 3 runs triggered")
	assert.True(t, *fires.Passed, "2 of 3 runs passed the outcome graders")
	assert.InDelta(t, 0.33, fires.CostUSD, 1e-9)
	assert.False(t, *byKey["fires/without"].Passed)
	quiet := byKey["quiet/with"]
	assert.True(t, *quiet.Triggered, "inverted grader failed in 2 of 3 runs, so the skill fired")
	assert.True(t, byKey["needs-exec/with"].Skipped)
	assert.NotContains(t, byKey, "quiet/without")

	score, _ := Score(req.Cases, resp, ScoreOptions{})
	assert.Equal(t, 2, score.Skipped)
	assert.InDelta(t, 0.5, score.PassRate, 1e-9) // fires passes, quiet triggered
}

func TestParseClaudeResult_AllRunsErroredAndBadShape(t *testing.T) {
	req := &Request{Cases: []Case{{ID: "a", ExpectTrigger: bp(true)}}, Ablation: false}
	tr := &ClaudeTranslation{Dirs: map[string]string{"a": "a"}, Skipped: map[string]string{}, Inverted: map[string]bool{}}
	doc, _ := json.Marshal(map[string]any{"cases": []map[string]any{{"name": "a", "arms": map[string]any{"with": []any{claudeRunJSON(true, true, 0, "sandbox refused")}}}}})
	resp, err := ParseClaudeResult(doc, req, tr)
	require.NoError(t, err)
	assert.Equal(t, "sandbox refused", resp.Results[0].Error)

	_, err = ParseClaudeResult([]byte(`{"something":"else"}`), req, tr)
	assert.ErrorContains(t, err, "unrecognized format")
	_, err = ParseClaudeResult([]byte(`nope`), req, tr)
	assert.Error(t, err)
}

func TestClaudePluginEval_RunBuildsCommandLineAndParsesOutput(t *testing.T) {
	req := sampleRequest(t)
	req.MaxCostUSD = 1.5
	req.Model = "haiku"
	var gotBin string
	var gotArgs []string
	runner := &ClaudePluginEval{Bin: "claude-x", Runs: 2, JudgeModel: "sonnet", ExtraArgs: []string{"--trust-plugin"},
		Exec: func(_ context.Context, bin string, args []string, _, _ io.Writer) error {
			gotBin, gotArgs = bin, args
			target := args[2]
			assert.FileExists(t, filepath.Join(target, "evals", "fires", "prompt.md"))
			out := args[indexOf(args, "--json")+1]
			doc, _ := json.Marshal(map[string]any{"cases": []map[string]any{
				{"name": "fires", "arms": map[string]any{"with": []any{claudeRunJSON(true, true, 0.2, "")}, "without": []any{claudeRunJSON(false, false, 0.1, "")}}},
			}})
			return os.WriteFile(out, doc, 0o600)
		}}
	resp, err := runner.Run(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "claude-x", gotBin)
	joined := strings.Join(gotArgs, " ")
	assert.Contains(t, joined, "plugin eval")
	assert.Contains(t, joined, "--no-publish")
	assert.Contains(t, joined, "--ablation with-without")
	assert.Contains(t, joined, "--runs 2")
	assert.Contains(t, joined, "--model haiku")
	assert.Contains(t, joined, "--judge-model sonnet")
	assert.Contains(t, joined, "--max-cost-usd 1.5000")
	assert.Contains(t, joined, "--threshold 0")
	assert.Contains(t, joined, "--trust-plugin")
	assert.NotEmpty(t, resp.Results)

	req.Harness = "codex"
	_, err = runner.Run(context.Background(), req)
	assert.ErrorContains(t, err, "only drives the claude harness")
}

func TestClaudePluginEval_FailureWithoutResultFile(t *testing.T) {
	runner := &ClaudePluginEval{Exec: func(context.Context, string, []string, io.Writer, io.Writer) error {
		return os.ErrPermission
	}}
	_, err := runner.Run(context.Background(), sampleRequest(t))
	assert.ErrorContains(t, err, "claude plugin eval failed")
}

func indexOf(list []string, item string) int {
	for i, v := range list {
		if v == item {
			return i
		}
	}
	return -1
}

func TestResponseValidate_RejectsBadCosts(t *testing.T) {
	req := &Request{Cases: []Case{{ID: "a"}}}
	for name, resp := range map[string]*Response{
		"negative result cost":   {Version: ProtocolVersion, Results: []Result{{Case: "a", Arm: ArmWith, CostUSD: -1}}},
		"negative response cost": {Version: ProtocolVersion, CostUSD: -5},
		"negative tokens":        {Version: ProtocolVersion, Results: []Result{{Case: "a", Arm: ArmWith, InputTokens: -1}}},
		"nan cost":               {Version: ProtocolVersion, CostUSD: math.NaN()},
	} {
		t.Run(name, func(t *testing.T) { assert.Error(t, resp.Validate(req)) })
	}
}
