package evals

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The transcripts below are trimmed from real `claude -p --output-format
// stream-json` and `codex exec --json` runs (claude 2.1.285, codex 0.160).
const (
	claudeFiredStream = `{"type":"system","subtype":"init","session_id":"s"}
{"type":"assistant","message":{"content":[{"type":"text","text":"I'll use the skill."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"ai-rulez-activation:deploy-staging"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"Launching skill"}]}}
{"type":"result","subtype":"error_max_turns","is_error":true,"total_cost_usd":0.0122,"usage":{"input_tokens":10,"cache_creation_input_tokens":600,"cache_read_input_tokens":13796,"output_tokens":55}}
`
	claudeQuietStream = `{"type":"assistant","message":{"content":[{"type":"text","text":"Poems are fun."}]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.004,"usage":{"input_tokens":9,"cache_creation_input_tokens":0,"cache_read_input_tokens":9000,"output_tokens":20}}
`
	claudeAuthFailure = `{"type":"result","subtype":"success","is_error":true,"result":"Invalid API key","total_cost_usd":0,"usage":{}}
`
	codexSkillRead = `{"type":"thread.started","thread_id":"x"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Using the skill."}}
{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"/bin/zsh -lc \"sed -n '1,240p' /w/.agents/skills/deploy-staging/SKILL.md\"","status":"in_progress"}}
{"type":"item.started","item":{"id":"item_2","type":"command_execution","command":"kubectl get pods"}}
`
)

func TestParseClaudeActivation(t *testing.T) {
	ids := map[string]bool{"deploy-staging": true, "release-notes": true}
	tests := []struct {
		name      string
		stdout    string
		stderr    string
		runErr    error
		wantFired []string
		wantErr   string
		wantIn    int
	}{
		{name: "a skill call", stdout: claudeFiredStream, wantFired: []string{"deploy-staging"}, wantIn: 10 + 600 + 13796},
		{name: "no skill call", stdout: claudeQuietStream, wantIn: 9009},
		{name: "an error result", stdout: claudeAuthFailure, wantErr: "Invalid API key"},
		{name: "no result line", stdout: "", stderr: "not logged in\n", runErr: errors.New("exit 1"), wantErr: "not logged in"},
		{name: "a skill outside the installed set is not a fire",
			stdout: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"design"}}]}}` + "\n" + claudeQuietStream,
			wantIn: 9009},
		{name: "a non-skill tool is not a fire",
			stdout: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"skill":"deploy-staging"}}]}}` + "\n" + claudeQuietStream,
			wantIn: 9009},
		{name: "an unprefixed skill name",
			stdout:    `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"release-notes"}}]}}` + "\n" + claudeQuietStream,
			wantFired: []string{"release-notes"}, wantIn: 9009},
		{name: "garbage lines are skipped", stdout: "not json\n" + claudeQuietStream, wantIn: 9009},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			out := parseClaudeActivation([]byte(tt.stdout), []byte(tt.stderr), tt.runErr, ids)

			// Assert
			if tt.wantErr != "" {
				assert.Contains(t, out.err, tt.wantErr)
				return
			}
			require.Empty(t, out.err)
			var fired []string
			for id := range out.fired {
				fired = append(fired, id)
			}
			assert.ElementsMatch(t, tt.wantFired, fired)
			assert.Equal(t, tt.wantIn, out.input)
		})
	}
}

func TestParseClaudeActivation_UsageAndCost(t *testing.T) {
	out := parseClaudeActivation([]byte(claudeFiredStream), nil, nil, map[string]bool{"deploy-staging": true})

	assert.InDelta(t, 0.0122, out.costUSD, 1e-9)
	assert.Equal(t, 55, out.output)
}

// writeInstalledSet makes the skill directories an activation request points at.
func writeInstalledSet(t *testing.T, ids ...string) []SkillRef {
	t.Helper()
	root := t.TempDir()
	var refs []SkillRef
	for _, id := range ids {
		dir := filepath.Join(root, id)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "evals"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+id+"\ndescription: does "+id+"\n---\nbody"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "evals", "case.eval.yaml"), []byte("secret"), 0o600))
		refs = append(refs, SkillRef{ID: id, Dir: dir, Description: "does " + id})
	}
	return refs
}

func activationRequest(t *testing.T, runs int, prompts ...string) *Request {
	t.Helper()
	req := &Request{Version: ProtocolVersion, Mode: ModeActivation, Surface: SurfaceNative, Harness: "claude", Runs: runs, MaxTurns: 1,
		Skills: writeInstalledSet(t, "deploy-staging", "release-notes")}
	req.Skill = req.Skills[0]
	for i, p := range prompts {
		req.Cases = append(req.Cases, Case{ID: "case-" + string(rune('a'+i)), Prompt: p, ExpectTrigger: bp(true), Target: "deploy-staging", Runs: runs})
	}
	return req
}

func TestClaudeNative_RunsEveryPromptRepeatedlyWithTheWholeInstalledSet(t *testing.T) {
	// Arrange
	req := activationRequest(t, 3, "deploy it", "write a poem")
	var mu sync.Mutex
	var calls []string
	var argv [][]string
	var plugin []string
	adapter := &ClaudeNative{Concurrency: 2, Exec: func(_ context.Context, _ string, args []string, dir string, stdin []byte) ([]byte, []byte, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, string(stdin))
		argv = append(argv, args)
		for i, a := range args {
			if a == "--plugin-dir" && len(plugin) == 0 {
				entries, err := os.ReadDir(filepath.Join(args[i+1], "skills"))
				require.NoError(t, err)
				for _, e := range entries {
					plugin = append(plugin, e.Name())
					_, statErr := os.Stat(filepath.Join(args[i+1], "skills", e.Name(), "evals"))
					assert.True(t, os.IsNotExist(statErr), "a skill's evals directory is not installed")
				}
			}
		}
		assert.NotEmpty(t, dir)
		if string(stdin) == "deploy it" {
			return []byte(claudeFiredStream), nil, nil
		}
		return []byte(claudeQuietStream), nil, nil
	}}

	// Act
	resp, err := adapter.Run(context.Background(), req)

	// Assert
	require.NoError(t, err)
	assert.Len(t, calls, 6, "two prompts three times each")
	assert.ElementsMatch(t, []string{"deploy-staging", "release-notes"}, plugin, "the whole installed set is in the one plugin")
	require.NoError(t, resp.Validate(req))
	require.Len(t, resp.Results, 2)
	fired, quiet := resp.Results[0], resp.Results[1]
	assert.Equal(t, 3, fired.Runs)
	assert.Equal(t, map[string]int{"deploy-staging": 3}, fired.FiredCounts)
	assert.True(t, *fired.Triggered)
	assert.Equal(t, []string{"deploy-staging"}, fired.Fired)
	assert.Equal(t, map[string]int{firedNone: 3}, quiet.FiredCounts)
	assert.False(t, *quiet.Triggered)
	assert.InDelta(t, 3*0.0122, fired.CostUSD, 1e-9)
	for _, args := range argv {
		joined := strings.Join(args, " ")
		assert.Contains(t, joined, "--max-turns 1")
		assert.Contains(t, joined, "--tools Skill")
		assert.Contains(t, joined, "--output-format stream-json")
		assert.Contains(t, joined, "--no-session-persistence")
		assert.NotContains(t, joined, "--model", "no model was asked for")
	}
}

func TestClaudeNative_PassesTheModelAndExtraArgs(t *testing.T) {
	req := activationRequest(t, 1, "deploy it")
	req.Model = "haiku"
	var got []string
	adapter := &ClaudeNative{ExtraArgs: []string{"--effort", "low"}, Exec: func(_ context.Context, _ string, args []string, _ string, _ []byte) ([]byte, []byte, error) {
		got = args
		return []byte(claudeQuietStream), nil, nil
	}}

	_, err := adapter.Run(context.Background(), req)

	require.NoError(t, err)
	assert.Contains(t, strings.Join(got, " "), "--model haiku")
	assert.Contains(t, strings.Join(got, " "), "--effort low")
}

func TestClaudeNative_EnforcesTheBudgetItself(t *testing.T) {
	// Arrange: every run costs $0.0122 and the budget is $0.02, one worker.
	req := activationRequest(t, 5, "deploy it")
	req.MaxCostUSD = 0.02
	runs := 0
	adapter := &ClaudeNative{Concurrency: 1, Exec: func(context.Context, string, []string, string, []byte) ([]byte, []byte, error) {
		runs++
		return []byte(claudeFiredStream), nil, nil
	}}

	// Act
	resp, err := adapter.Run(context.Background(), req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, runs, "the third run is not started once the spend reached the budget")
	assert.Equal(t, 2, resp.Results[0].Runs, "the runs that did not start are not counted")
}

func TestClaudeNative_AllRunsFailingIsAnErrorResult(t *testing.T) {
	req := activationRequest(t, 2, "deploy it")
	adapter := &ClaudeNative{Exec: func(context.Context, string, []string, string, []byte) ([]byte, []byte, error) {
		return []byte(claudeAuthFailure), nil, nil
	}}

	resp, err := adapter.Run(context.Background(), req)

	require.NoError(t, err)
	assert.Contains(t, resp.Results[0].Error, "Invalid API key")
	require.NoError(t, resp.Validate(req), "an error result needs no counts")
}

func TestClaudeNative_RefusesOtherHarnessesAndCaseRequests(t *testing.T) {
	adapter := &ClaudeNative{Exec: func(context.Context, string, []string, string, []byte) ([]byte, []byte, error) { return nil, nil, nil }}
	req := activationRequest(t, 1, "x")

	req.Harness = "codex"
	_, err := adapter.Run(context.Background(), req)
	assert.ErrorContains(t, err, "only drives the claude harness")

	req.Harness, req.Mode = "claude", ""
	_, err = adapter.Run(context.Background(), req)
	assert.ErrorContains(t, err, "only answers activation requests")
}

func TestClaudeNative_DeclaresOnlyNativeActivation(t *testing.T) {
	adapter := &ClaudeNative{}

	assert.NoError(t, RequireSurface(context.Background(), adapter, CapabilityActivation, SurfaceNative))
	assert.Error(t, RequireSurface(context.Background(), adapter, CapabilityActivation, SurfaceRetrieval))
	assert.Equal(t, RunnerClaudeNative, adapter.Name())
	assert.Contains(t, adapter.Fingerprint(), "bin=claude")
}

func TestCodexWatch(t *testing.T) {
	ids := map[string]bool{"deploy-staging": true, "release-notes": true}
	tests := []struct {
		name      string
		lines     []string
		wantFired []string
		wantStop  bool
		wantErr   bool
	}{
		{name: "reads a skill, stop at once", lines: strings.Split(strings.TrimSpace(codexSkillRead), "\n"), wantFired: []string{"deploy-staging"}, wantStop: true},
		{name: "answers without a command", lines: []string{`{"type":"turn.started"}`, `{"type":"turn.completed","usage":{}}`}, wantStop: true},
		{name: "three commands and no skill", lines: []string{`{"type":"turn.started"}`,
			`{"type":"item.started","item":{"type":"command_execution","command":"ls"}}`,
			`{"type":"item.started","item":{"type":"command_execution","command":"pwd"}}`,
			`{"type":"item.started","item":{"type":"command_execution","command":"git status"}}`}, wantStop: true},
		{name: "two commands keep going", lines: []string{`{"type":"turn.started"}`,
			`{"type":"item.started","item":{"type":"command_execution","command":"ls"}}`,
			`{"type":"item.started","item":{"type":"command_execution","command":"pwd"}}`}},
		{name: "no events at all", lines: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := &codexWatch{ids: ids}
			stopped := false

			// Act
			for _, l := range tt.lines {
				if w.line([]byte(l)) {
					stopped = true
					break
				}
			}
			out := w.outcome(nil)

			// Assert
			assert.Equal(t, tt.wantStop, stopped)
			if tt.wantErr {
				assert.NotEmpty(t, out.err)
				return
			}
			var fired []string
			for id := range out.fired {
				fired = append(fired, id)
			}
			assert.ElementsMatch(t, tt.wantFired, fired)
		})
	}
}

func TestCodexNative_RunsThroughTheStartHookAndStopsEarly(t *testing.T) {
	// Arrange
	req := activationRequest(t, 2, "deploy it")
	req.Harness = "codex"
	var mu sync.Mutex
	stoppedEarly := 0
	var gotArgs []string
	var gotEnv []string
	var skillFile bool
	adapter := &CodexNative{Concurrency: 1, Runner: lineFake(func(spec runner.Spec, onLine runner.LineFunc) runner.Result {
		mu.Lock()
		defer mu.Unlock()
		gotArgs, gotEnv = spec.Argv[1:], spec.Env
		_, err := os.Stat(filepath.Join(spec.Dir, ".agents", "skills", "release-notes", "SKILL.md"))
		skillFile = err == nil
		assert.Equal(t, "deploy it", string(spec.Stdin))
		for _, l := range strings.Split(strings.TrimSpace(codexSkillRead), "\n") {
			if onLine([]byte(l)) {
				stoppedEarly++
				return runner.Result{Status: runner.StatusOK}
			}
		}
		return runner.Result{Status: runner.StatusOK}
	}), Env: ambient.MapEnv{Vars: map[string]string{"PATH": "/usr/bin", "CODEX_HOME": "/codex-login", "AWS_SECRET_ACCESS_KEY": "s3cret", "GITHUB_TOKEN": "t"}, Home: "/users/me"}}

	// Act
	resp, err := adapter.Run(context.Background(), req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, stoppedEarly)
	assert.True(t, skillFile, "the whole installed set is under .agents/skills")
	assert.Equal(t, map[string]int{"deploy-staging": 2}, resp.Results[0].FiredCounts)
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"exec --json", "--ephemeral", "--ignore-user-config", "-s read-only", "--skip-git-repo-check"} {
		assert.Contains(t, joined, want)
	}
	assert.Equal(t, "-", gotArgs[len(gotArgs)-1])
	var home string
	for _, kv := range gotEnv {
		if v, ok := strings.CutPrefix(kv, "HOME="); ok {
			home = v
		}
	}
	assert.NotEmpty(t, home)
	assert.NotEqual(t, os.Getenv("HOME"), home, "the model's shell does not see the real home")
	assert.NoError(t, RequireSurface(context.Background(), adapter, CapabilityActivation, SurfaceNative))
	assert.Equal(t, RunnerCodexNative, adapter.Name())
	assert.Contains(t, gotEnv, "CODEX_HOME=/codex-login", "the login directory is kept")
	assert.Contains(t, gotEnv, "PATH=/usr/bin")
	for _, kv := range gotEnv {
		assert.NotContains(t, kv, "s3cret", "credentials of the parent environment never reach the harness")
		assert.NotContains(t, kv, "GITHUB_TOKEN")
	}
}

// lineFake is a runner.LineRunner that answers from a function.
type lineFake func(spec runner.Spec, onLine runner.LineFunc) runner.Result

func (lineFake) Run(_ context.Context, spec runner.Spec) runner.Result {
	return runner.Result{Status: runner.StatusUnavailable, ExitCode: -1, Err: &runner.DeniedError{Argv: spec.Argv}}
}

func (f lineFake) RunLines(_ context.Context, spec runner.Spec, onLine runner.LineFunc) runner.Result {
	return f(spec, onLine)
}

func TestCodexNative_NeedsARunnerThatCanStream(t *testing.T) {
	req := activationRequest(t, 1, "x")
	req.Harness = "codex"
	buffered := runner.Func(func(context.Context, runner.Spec) runner.Result { return runner.Result{} })

	_, err := (&CodexNative{Runner: buffered}).Run(context.Background(), req)

	assert.ErrorContains(t, err, "can stream output")
}

func TestCodexNative_ACodexThatDiesWithoutEventsIsAnErrorResult(t *testing.T) {
	req := activationRequest(t, 1, "x")
	req.Harness = "codex"
	dead := lineFake(func(runner.Spec, runner.LineFunc) runner.Result {
		return runner.Result{Status: runner.StatusUnavailable, Err: assert.AnError}
	})

	resp, err := (&CodexNative{Runner: dead}).Run(context.Background(), req)

	require.NoError(t, err)
	assert.Contains(t, resp.Results[0].Error, "codex produced no events")
}

func TestCodexNative_RefusesAnotherHarness(t *testing.T) {
	req := activationRequest(t, 1, "x")
	req.Harness = "claude"

	_, err := (&CodexNative{}).Run(context.Background(), req)

	assert.ErrorContains(t, err, "only drives the codex harness")
}

func TestAggregateActivation(t *testing.T) {
	c := &Case{ID: "c", Target: "t"}
	tests := []struct {
		name     string
		outs     []activationOutcome
		wantRuns int
		wantErr  bool
		counts   map[string]int
	}{
		{name: "mixed runs", outs: []activationOutcome{{fired: map[string]bool{"t": true}}, {fired: map[string]bool{"t": true, "u": true}}, {fired: map[string]bool{}}},
			wantRuns: 3, counts: map[string]int{"t": 2, "u": 1, firedNone: 1}},
		{name: "partial errors count only usable runs", outs: []activationOutcome{{fired: map[string]bool{"t": true}}, {err: "boom"}}, wantRuns: 1, counts: map[string]int{"t": 1}},
		{name: "all errors", outs: []activationOutcome{{err: "boom"}}, wantErr: true},
		{name: "no outcomes", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := aggregateActivation(c, tt.outs)

			if tt.wantErr {
				assert.NotEmpty(t, res.Error)
				return
			}
			assert.Equal(t, tt.wantRuns, res.Runs)
			assert.Equal(t, tt.counts, res.FiredCounts)
		})
	}
}
