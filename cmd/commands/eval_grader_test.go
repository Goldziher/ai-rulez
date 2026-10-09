package commands

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetGraderFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		evalFlags.grader, evalFlags.allowLLM, evalFlags.graderMaxCost = evals.GraderRunner, false, defaultGraderMaxCost
	}
	reset()
	t.Cleanup(reset)
}

const graderCases = `cases:
  - id: fires
    prompt: do it
    expect_trigger: true
    rubric: The answer says it deployed.
`

// graderProject is evalProject with one rubric case.
func graderProject(t *testing.T) string {
	t.Helper()
	root := evalProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "skills", "deploy", "evals", "a.eval.yaml"), []byte(graderCases), 0o600))
	return root
}

func TestEvalRun_GraderFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		set     func()
		wantErr string
	}{
		{"unknown grader", func() { evalFlags.grader = "oracle" }, "unknown --grader"},
		{"negative cap", func() { evalFlags.grader, evalFlags.graderMaxCost = evals.GraderBuiltin, -1 }, "--grader-max-cost"},
		{"NaN cap", func() { evalFlags.grader, evalFlags.graderMaxCost = evals.GraderBuiltin, math.NaN() }, "finite"},
		{"infinite cap", func() { evalFlags.grader, evalFlags.graderMaxCost = evals.GraderBuiltin, math.Inf(1) }, "finite"},
		{"builtin needs cases mode", func() {
			evalFlags.grader, evalFlags.mode, evalFlags.surface = evals.GraderBuiltin, evals.ModeActivation, evals.SurfaceRetrieval
		}, "--grader needs --mode cases"},
		{"builtin without consent", func() { evalFlags.grader = evals.GraderBuiltin; evalFlags.runnerCommand = "true" }, "needs --allow-llm"},
		{"builtin without the network switched on", func() {
			evalFlags.grader, evalFlags.allowLLM, evalFlags.runnerCommand = evals.GraderBuiltin, true, "true"
		}, "allow_network is not enabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetEvalFlags(t)
			resetGraderFlags(t)
			graderProject(t)
			tt.set()

			_, err := runEval(evalRunCmd, nil)

			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestEvalRun_BuiltinGraderRefusesBeforeTheRunnerStarts(t *testing.T) {
	resetEvalFlags(t)
	resetGraderFlags(t)
	root := graderProject(t)
	marker := filepath.Join(t.TempDir(), "started")
	evalFlags.runnerCommand = "touch " + marker
	evalFlags.grader = evals.GraderBuiltin

	_, err := runEval(evalRunCmd, nil)

	require.Error(t, err)
	assert.NoFileExists(t, marker, "the runner was never started")
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName))
}

func TestEvalRun_DryRunWithTheBuiltinGraderSendsNothing(t *testing.T) {
	resetEvalFlags(t)
	resetGraderFlags(t)
	graderProject(t)
	evalFlags.grader, evalFlags.estimate = evals.GraderBuiltin, true
	evalFlags.runnerCommand = "exit 9"
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	failed, err := runEval(evalRunCmd, nil)

	require.NoError(t, err)
	assert.False(t, failed)
	assert.Contains(t, out.String(), "Dry run")
}

// TestEvalRun_BuiltinGraderEndToEndAgainstALocalModelServer grades a runner transcript
// through the real model layer, pointed at a local OpenAI-compatible server.
func TestEvalRun_BuiltinGraderEndToEndAgainstALocalModelServer(t *testing.T) {
	resetEvalFlags(t)
	resetGraderFlags(t)
	root := graderProject(t)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body := `{"id":"x","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"{\"score\":0.95,\"rationale\":\"it says it deployed\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":20,"total_tokens":140}}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body)) //nolint:errcheck // test server
	}))
	defer server.Close()
	// The user config (never the repository) turns the network on and names the local server.
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("LOCAL_GRADER_KEY", "not-a-real-key")
	require.NoError(t, os.MkdirAll(filepath.Join(xdg, "ai-rulez"), 0o750))
	userConfig := "[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o-mini\"\nallow_network = true\nallow_plain_http = true\n" +
		"plain_http_hosts = [\"" + server.Listener.Addr().String() + "\"]\nbase_url = \"" + server.URL + "/v1\"\napi_key_env = \"LOCAL_GRADER_KEY\"\ncache = false\n"
	require.NoError(t, os.WriteFile(filepath.Join(xdg, "ai-rulez", "config.toml"), []byte(userConfig), 0o600))
	reply := `{"version":1,"results":[{"case":"fires","arm":"with","triggered":true,"output":"I deployed the service.","cost_usd":0.01}]}`
	evalFlags.runnerCommand = writeRunnerScript(t, reply)
	evalFlags.grader, evalFlags.allowLLM, evalFlags.format = evals.GraderBuiltin, true, evals.FormatJSON
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)
	evalRunCmd.SetErr(&bytes.Buffer{})

	failed, err := runEval(evalRunCmd, nil)

	require.NoError(t, err)
	assert.False(t, failed)
	assert.Equal(t, 1, requests, "one judge call for the one rubric")
	var report evals.RunReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Contains(t, report.Grader, "builtin:openai/gpt-4o-mini")
	require.NotNil(t, report.Skills[0].Cases[0].RubricScore)
	assert.Equal(t, 0.95, *report.Skills[0].Cases[0].RubricScore)
	assert.Equal(t, "it says it deployed", report.Skills[0].Cases[0].RubricNote)
	assert.FileExists(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName))
}
