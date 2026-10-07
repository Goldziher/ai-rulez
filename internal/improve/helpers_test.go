package improve

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/stretchr/testify/require"
)

const (
	skillBody = "---\nname: deploy\ndescription: Deploy things to staging. Use when asked to deploy.\n---\n\n# Deploy\n\nRun the deploy.\n"

	trainCases = `cases:
  - id: train-basic
    prompt: Deploy the billing service to staging
    expect_trigger: true
    assertions:
      - {type: contains, value: staging-eu}
  - id: train-two
    prompt: Ship the api to staging please
    expect_trigger: true
`
	heldCases = `cases:
  - id: held-one
    prompt: CANARY-PROMPT-ONE roll out checkout
    expect_trigger: true
    tags: [holdout]
    assertions:
      - {type: contains, value: CANARY-ASSERT-ONE}
  - id: held-two
    prompt: CANARY-PROMPT-TWO push the worker
    expect_trigger: true
    tags: [holdout]
    assertions:
      - {type: contains, value: CANARY-ASSERT-TWO}
  - id: held-neg
    prompt: CANARY-PROMPT-NEG explain what a deploy is
    expect_trigger: false
    tags: [holdout]
`
)

var canaries = []string{"CANARY-PROMPT-ONE", "CANARY-PROMPT-TWO", "CANARY-PROMPT-NEG", "CANARY-ASSERT-ONE", "CANARY-ASSERT-TWO"}

// project builds a config dir with one skill and its eval cases.
func project(t *testing.T) (root, configDir string) {
	t.Helper()
	return projectIn(t, ".ai-rulez")
}

// projectIn is project with the config directory at rel below the project root.
func projectIn(t *testing.T, rel string) (root, configDir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // the report MAC key lives here, never in the real home
	root = t.TempDir()
	configDir = filepath.Join(root, filepath.FromSlash(rel))
	files := map[string]string{
		"config.toml":                         "version = \"5.0\"\nname = \"t\"\n",
		"skills/deploy/SKILL.md":              skillBody,
		"skills/deploy/evals/train.eval.yaml": trainCases,
		"skills/deploy/evals/held.eval.yaml":  heldCases,
	}
	for name, body := range files {
		p := filepath.Join(configDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	return root, configDir
}

// fakeEval is a deterministic eval runner: a case passes when pass(case, skill text) says so.
type fakeEval struct {
	mu    sync.Mutex
	pass  func(caseID, skill string) bool
	trig  func(caseID, skill string, expect bool) bool
	cost  float64
	calls []string // skill digests per call
}

func (f *fakeEval) Name() string { return "fake" }

func (f *fakeEval) Run(_ context.Context, req *evals.Request) (*evals.Response, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req.Skill.Dir)
	f.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(req.Skill.Dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	skill := string(data)
	resp := &evals.Response{Version: evals.ProtocolVersion}
	for i := range req.Cases {
		c := &req.Cases[i]
		pass := f.pass == nil || f.pass(c.ID, skill)
		cost := f.cost
		if cost == 0 {
			cost = 0.01 * float64(len(req.Cases))
		}
		trig := c.Expects()
		if f.trig != nil {
			trig = f.trig(c.ID, skill, c.Expects())
		}
		resp.Results = append(resp.Results, evals.Result{Case: c.ID, Arm: evals.ArmWith, Triggered: &trig, Passed: &pass, CostUSD: cost / float64(len(req.Cases))})
	}
	return resp, nil
}

func (f *fakeEval) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// optimizer builds a fake optimizer: edit receives the workspace skill dir and the request.
func optimizer(t *testing.T, edit func(dir string, req *OptimizerRequest, spec runner.Spec)) *runner.Fake {
	t.Helper()
	return &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		var req OptimizerRequest
		if err := json.Unmarshal(spec.Stdin, &req); err != nil {
			return runner.Result{Status: runner.StatusError, Err: err}
		}
		edit(filepath.Join(spec.Dir, req.Skill.Dir), &req, spec)
		out, _ := json.Marshal(OptimizerResponse{Version: 1, Summary: "edited", Changed: []string{"SKILL.md"}, CostUSD: 0.01})
		return runner.Result{Status: runner.StatusOK, ExitCode: 0, Stdout: out}
	}}
}

func appendSkill(t *testing.T, dir, text string) {
	t.Helper()
	p := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, append(data, []byte(text)...), 0o600))
}

func baseOptions(root, configDir string, ev evals.Runner, exec runner.Runner) Options {
	return Options{
		ConfigDir: configDir, RepoDir: root, SkillID: "deploy", OptimizerArgv: []string{"opt"}, Exec: exec,
		HostEnv: []string{"PATH=/usr/bin", "SECRET_TOKEN=hunter2", "LANG=C"}, Eval: ev, Harness: "claude", Runs: 1,
		MaxCostUSD: 5, MinGain: 0.05, MaxRounds: 2, MaxHoldoutEvals: 2, Date: "2026-01-02", ToolVersion: "test",
	}
}

func mustPrepare(t *testing.T, o *Options) *Plan {
	t.Helper()
	p, err := Prepare(context.Background(), o)
	require.NoError(t, err)
	return p
}

func contains(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func mustSkills(t *testing.T, configDir string) []evals.Skill {
	t.Helper()
	skills, err := evals.FindSkills(configDir)
	require.NoError(t, err)
	return skills
}
