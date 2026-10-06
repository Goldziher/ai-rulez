package evals

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nativeRunner is a fake runner that declares the native activation surface and
// answers from a function.
type nativeRunner struct {
	*fakeRunner
}

func (nativeRunner) Capabilities() []string { return []string{CapabilityActivation} }
func (nativeRunner) Surfaces() []string     { return []string{SurfaceNative} }

// scripted answers each prompt from counts[caseID]: how often each skill loaded
// over the request's runs.
func scripted(counts map[string]map[string]int) nativeRunner {
	return nativeRunner{&fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			resp.Results = append(resp.Results, Result{
				Case: c.ID, Arm: ArmWith, Runs: req.Runs, FiredCounts: counts[c.ID],
				InputTokens: 1000 * req.Runs, OutputTokens: 100 * req.Runs, CostUSD: 0.01 * float64(req.Runs),
			})
		}
		return resp, nil
	}}}
}

func nativeOptions(cfg string, r Runner) *ActivationOptions {
	return &ActivationOptions{
		ConfigDir: cfg, Skills: []string{"deploy-staging"}, Surface: SurfaceNative, Runner: r, Harness: "claude", Model: "haiku",
		Runs: 5, Date: "2026-10-06", ToolVersion: "test",
	}
}

func TestRunActivationNative_ScoresRatesBorderlineAndConfusion(t *testing.T) {
	// Arrange: fires 5/5 (passed), stolen 1/5 of the target and 4/5 by the sibling
	// (failed, stolen), the near miss fires once (borderline), unrelated never.
	cfg := activationProject(t)
	runner := scripted(map[string]map[string]int{
		"fires":             {"deploy-staging": 5},
		"stolen":            {"deploy-staging": 1, "release-notes": 4},
		"fires.near-miss-1": {"deploy-staging": 1, "release-notes": 3, firedNone: 1},
		"unrelated":         {firedNone: 5},
	})

	// Act
	report, err := RunActivation(context.Background(), nativeOptions(cfg, runner))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, SurfaceNative, report.Surface)
	assert.Equal(t, 5, report.Runs)
	assert.True(t, report.Failed, "the stolen prompt fails the skill")
	assert.InDelta(t, 0.20, report.Cost.ActualUSD, 1e-9, "four prompts of five runs at $0.01")
	require.Len(t, report.Skills, 1)
	skill := report.Skills[0]
	assert.Equal(t, RunRan, skill.Status)
	assert.False(t, skill.Passing)
	byCase := map[string]ActivationPrompt{}
	for _, p := range skill.Prompts {
		byCase[p.Case] = p
	}
	assert.Equal(t, PromptPassed, byCase["fires"].Status)
	assert.Equal(t, 1.0, byCase["fires"].Rate)
	assert.Equal(t, PromptFailed, byCase["stolen"].Status)
	assert.Equal(t, "release-notes", byCase["stolen"].Winner)
	assert.Equal(t, PromptBorderline, byCase["fires.near-miss-1"].Status, "one fire in five is one run from failing the 0.2 ceiling")
	assert.Equal(t, PromptPassed, byCase["unrelated"].Status)
	require.NotNil(t, byCase["fires"].Interval)
	assert.InDelta(t, 0.5655, byCase["fires"].Interval.Low, 0.001)
	assert.Equal(t, []StolenBy{{Skill: "release-notes", Prompts: 1, Share: 0.5}}, skill.StolenBy)
	assert.Equal(t, map[string]map[string]int{"deploy-staging": {"deploy-staging": 6, "release-notes": 4}}, report.Confusion)
	require.NotNil(t, skill.Recall)
	assert.Equal(t, 0.5, skill.Recall.Value, "one of two positive prompts passes its threshold")
	require.NotNil(t, skill.RunRecall)
	assert.Equal(t, 0.6, skill.RunRecall.Value, "six fires in ten positive runs")
	assert.Equal(t, 5, runner.reqs[0].Runs)
	assert.Equal(t, ModeActivation, runner.reqs[0].Mode)
	assert.Len(t, runner.reqs[0].Skills, 2, "the installed set is the competing set")
	assert.Equal(t, "deploy-staging", runner.reqs[0].Cases[0].Target)
}

func TestRunActivationNative_RefusesRunnersWithoutTheSurface(t *testing.T) {
	cfg := activationProject(t)
	tests := []struct {
		name    string
		runner  Runner
		wantErr string
	}{
		{name: "no capability", runner: goodRunner(), wantErr: "does not support activation mode"},
		{name: "wrong surface", runner: surfaceRunner{goodRunner(), []string{"other"}}, wantErr: "does not support the native surface"},
		{name: "no surface", runner: surfaceRunner{goodRunner(), nil}, wantErr: "declares no surface"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := RunActivation(context.Background(), nativeOptions(cfg, tt.runner))

			// Assert
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

type surfaceRunner struct {
	*fakeRunner
	surfaces []string
}

func (surfaceRunner) Capabilities() []string { return []string{CapabilityActivation} }
func (s surfaceRunner) Surfaces() []string   { return s.surfaces }

func TestRunActivationNative_NeverSendsARefusedRunnerAnything(t *testing.T) {
	cfg := activationProject(t)
	plain := goodRunner()

	_, err := RunActivation(context.Background(), nativeOptions(cfg, plain))

	require.Error(t, err)
	assert.Zero(t, plain.calls, "a runner that did not declare the capability is never called")
}

func TestRunActivationNative_DryRunCallsNoRunner(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	plain := goodRunner()
	opts := nativeOptions(cfg, plain)
	opts.DryRun = true

	// Act
	report, err := RunActivation(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.Zero(t, plain.calls)
	assert.True(t, report.DryRun)
	assert.False(t, report.Failed)
	require.NotNil(t, report.Estimate)
	assert.Equal(t, 4*5, report.Estimate.AgentRuns, "four prompts (a near miss included) five times")
	assert.Greater(t, report.Estimate.CostHighUSD, report.Estimate.CostUSD)
	assert.Greater(t, report.Estimate.CostUSD, report.Estimate.CostLowUSD)
	assert.Equal(t, RunDryRun, report.Skills[0].Status)
}

func TestRunActivationNative_MaxCostRefusesBeforeRunning(t *testing.T) {
	tests := []struct {
		name string
		mode string
		max  float64
		want string
	}{
		{name: "high figure over the cap", mode: CostModeHigh, max: 0.0001, want: "the high estimate"},
		{name: "expected figure over the cap", mode: CostModeExpected, max: 0.0001, want: "the expected estimate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := activationProject(t)
			runner := scripted(nil)
			opts := nativeOptions(cfg, runner)
			opts.MaxCostUSD, opts.MaxCostMode = tt.max, tt.mode

			// Act
			_, err := RunActivation(context.Background(), opts)

			// Assert
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
			assert.Zero(t, runner.calls)
		})
	}
}

func TestRunActivationNative_MaxCostWithUnknownModelRefuses(t *testing.T) {
	cfg := activationProject(t)
	opts := nativeOptions(cfg, scripted(nil))
	opts.Model, opts.MaxCostUSD = "mystery-9", 1

	_, err := RunActivation(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorContains(t, err, "no built-in price")
}

func TestRunActivationNative_StoresRecordAndReplaysItUntilForced(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	runner := scripted(map[string]map[string]int{
		"fires": {"deploy-staging": 5}, "stolen": {"deploy-staging": 5}, "fires.near-miss-1": {firedNone: 5}, "unrelated": {firedNone: 5},
	})
	store := NewStore()
	opts := nativeOptions(cfg, runner)
	opts.Store = store

	// Act
	first, err := RunActivation(context.Background(), opts)
	require.NoError(t, err)
	second, err := RunActivation(context.Background(), opts)
	require.NoError(t, err)
	opts.Force = true
	third, err := RunActivation(context.Background(), opts)
	require.NoError(t, err)

	// Assert
	assert.False(t, first.Failed)
	rec, ok := store.Get("deploy-staging")
	require.True(t, ok)
	require.NotNil(t, rec.Activation)
	assert.Equal(t, SurfaceNative, rec.Activation.Surface)
	assert.True(t, rec.Activation.Passing)
	assert.Equal(t, 5, rec.Activation.Runs)
	assert.NotEmpty(t, rec.Activation.CacheKey)
	require.NotNil(t, rec.Activation.Estimate)
	assert.Equal(t, 20, rec.Activation.Estimate.AgentRuns)
	assert.Equal(t, 5000*4, rec.Activation.Estimate.ActualInputTokens)
	assert.Equal(t, RunCached, second.Skills[0].Status, "same inputs are replayed, not paid for again")
	assert.False(t, second.Failed)
	assert.Equal(t, 2, runner.calls, "the run and the forced run only")
	assert.Equal(t, RunRan, third.Skills[0].Status)
}

func TestRunActivationNative_ASiblingsNewDescriptionRepeatsTheRun(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	runner := scripted(map[string]map[string]int{"fires": {"deploy-staging": 5}, "stolen": {"deploy-staging": 5}})
	opts := nativeOptions(cfg, runner)
	opts.Store = NewStore()
	_, err := RunActivation(context.Background(), opts)
	require.NoError(t, err)
	sibling := filepath.Join(cfg, "skills", "release-notes", "SKILL.md")
	require.NoError(t, os.WriteFile(sibling, []byte("---\nname: release-notes\ndescription: Deploy notes for every staging release\n---\nbody\n"), 0o600))

	// Act
	report, err := RunActivation(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, RunRan, report.Skills[0].Status, "the competition changed, so the cached result no longer applies")
	assert.Equal(t, 2, runner.calls)
}

func TestRunActivationNative_PromptErrorsLeaveTheSkillUnscored(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	runner := nativeRunner{&fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			res := Result{Case: c.ID, Arm: ArmWith, Runs: req.Runs, FiredCounts: map[string]int{"deploy-staging": req.Runs}}
			if c.ID == "unrelated" {
				res = Result{Case: c.ID, Arm: ArmWith, Error: "rate limited"}
			}
			resp.Results = append(resp.Results, res)
		}
		return resp, nil
	}}}
	opts := nativeOptions(cfg, runner)
	opts.Store = NewStore()

	// Act
	report, err := RunActivation(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.True(t, report.Failed)
	skill := report.Skills[0]
	assert.Contains(t, skill.Error, "no usable result")
	assert.False(t, skill.Passing)
	_, stored := opts.Store.Get("deploy-staging")
	assert.False(t, stored, "an infrastructure failure is never recorded")
}

func TestRunActivationNative_RunnerErrorIsASkillError(t *testing.T) {
	cfg := activationProject(t)
	runner := nativeRunner{&fakeRunner{fn: func(*Request) (*Response, error) { return nil, assert.AnError }}}

	report, err := RunActivation(context.Background(), nativeOptions(cfg, runner))

	require.NoError(t, err)
	assert.Equal(t, RunError, report.Skills[0].Status)
	assert.True(t, report.Failed)
}

func TestResponseValidate_Activation(t *testing.T) {
	req := &Request{Mode: ModeActivation, Skills: []SkillRef{{ID: "a"}, {ID: "b"}}, Cases: []Case{{ID: "c"}}}
	tests := []struct {
		name    string
		result  Result
		wantErr string
	}{
		{name: "valid", result: Result{Case: "c", Arm: ArmWith, Runs: 5, FiredCounts: map[string]int{"a": 4, "b": 1, firedNone: 0}}},
		{name: "no runs", result: Result{Case: "c", Arm: ArmWith}, wantErr: "runs must be >= 1"},
		{name: "unknown skill", result: Result{Case: "c", Arm: ArmWith, Runs: 5, FiredCounts: map[string]int{"z": 1}}, wantErr: "not in the installed set"},
		{name: "count above runs", result: Result{Case: "c", Arm: ArmWith, Runs: 3, FiredCounts: map[string]int{"a": 4}}, wantErr: "outside 0..runs"},
		{name: "negative count", result: Result{Case: "c", Arm: ArmWith, Runs: 3, FiredCounts: map[string]int{"a": -1}}, wantErr: "outside 0..runs"},
		{name: "without arm", result: Result{Case: "c", Arm: ArmWithout, Runs: 3}, wantErr: "only the \"with\" arm"},
		{name: "error result needs no runs", result: Result{Case: "c", Arm: ArmWith, Error: "boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{Version: ProtocolVersion, Results: []Result{tt.result}}

			err := resp.Validate(req)

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestNativePromptStatus(t *testing.T) {
	tests := []struct {
		name   string
		expect bool
		k, n   int
		want   string
	}{
		{name: "positive all", expect: true, k: 5, n: 5, want: PromptPassed},
		{name: "positive at the floor is borderline", expect: true, k: 4, n: 5, want: PromptBorderline},
		{name: "positive below the floor", expect: true, k: 3, n: 5, want: PromptFailed},
		{name: "positive two of three fails", expect: true, k: 2, n: 3, want: PromptFailed},
		{name: "positive three of three", expect: true, k: 3, n: 3, want: PromptPassed},
		{name: "negative one of three", expect: false, k: 1, n: 3, want: PromptFailed},
		{name: "negative none", expect: false, k: 0, n: 5, want: PromptPassed},
		{name: "negative at the ceiling is borderline", expect: false, k: 1, n: 5, want: PromptBorderline},
		{name: "negative above the ceiling", expect: false, k: 2, n: 5, want: PromptFailed},
		{name: "a single run is never borderline", expect: true, k: 1, n: 1, want: PromptPassed},
		{name: "ten runs positive 8", expect: true, k: 8, n: 10, want: PromptBorderline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nativePromptStatus(tt.expect, tt.k, tt.n))
		})
	}
}

func TestMostFired(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   string
	}{
		{name: "none fired", counts: nil, want: activationNone},
		{name: "only none", counts: map[string]int{firedNone: 5}, want: activationNone},
		{name: "a clear winner", counts: map[string]int{"a": 1, "b": 3}, want: "b"},
		{name: "a tie goes to the target", counts: map[string]int{"a": 2, "t": 2}, want: "t"},
		{name: "a tie without the target goes to the first id", counts: map[string]int{"b": 2, "a": 2}, want: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mostFired(tt.counts, "t"))
		})
	}
}

func TestCommandRunner_Handshake(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the runner is a POSIX shell command")
	}
	tests := []struct {
		name     string
		command  string
		wantCaps []string
		wantSurf []string
		wantErr  string
	}{
		{name: "declares activation", command: `printf '{"version":1,"capabilities":["activation"],"surfaces":["native"]}'`, wantCaps: []string{"activation"}, wantSurf: []string{"native"}},
		{name: "an old runner declares nothing", command: `printf '{"version":1,"results":[]}'`},
		{name: "an old runner that runs cases is rejected", command: `printf '{"version":1,"results":[{"case":"x","arm":"with","triggered":true}]}'`, wantErr: "does not implement the handshake"},
		{name: "wrong protocol version", command: `printf '{"version":9}'`, wantErr: "protocol version 9"},
		{name: "not json", command: `printf 'hello'`, wantErr: "invalid JSON"},
		{name: "failing command", command: `exit 3`, wantErr: "runner command failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := &CommandRunner{Command: tt.command}

			// Act
			d, err := r.Handshake(context.Background())

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCaps, d.Capabilities)
			assert.Equal(t, tt.wantSurf, d.Surfaces)
		})
	}
}

func TestCommandRunner_HandshakeSendsAProbeWithoutWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the runner is a POSIX shell script")
	}
	// Arrange
	dir := t.TempDir()
	captured := filepath.Join(dir, "probe.json")
	script := filepath.Join(dir, "runner.sh")
	body := "#!/bin/sh\ncat > " + captured + "\nprintf '{\"version\":1,\"capabilities\":[\"activation\"],\"surfaces\":[\"native\"]}'\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700))

	// Act
	err := RequireSurface(context.Background(), &CommandRunner{Command: script}, CapabilityActivation, SurfaceNative)

	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(captured)
	require.NoError(t, err)
	var probe Request
	require.NoError(t, json.Unmarshal(data, &probe))
	assert.Equal(t, ModeCapabilities, probe.Mode)
	assert.Empty(t, probe.Cases)
	assert.Empty(t, probe.Skill.ID)
}

func TestRunActivationNative_CommandRunnerEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the runner is a POSIX shell script")
	}
	// Arrange: a runner script that answers the probe, then fires the target on every prompt.
	cfg := activationProject(t)
	script := filepath.Join(t.TempDir(), "runner.sh")
	body := `#!/bin/sh
input=$(cat)
case "$input" in
*'"mode":"capabilities"'*) printf '{"version":1,"capabilities":["activation"],"surfaces":["native"]}' ;;
*) printf '{"version":1,"results":[{"case":"fires","arm":"with","runs":2,"fired_counts":{"deploy-staging":2}},{"case":"stolen","arm":"with","runs":2,"fired_counts":{"deploy-staging":2}},{"case":"fires.near-miss-1","arm":"with","runs":2,"fired_counts":{"none":2}},{"case":"unrelated","arm":"with","runs":2,"fired_counts":{"none":2}}],"cost_usd":0.02}' ;;
esac
`
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700))
	opts := nativeOptions(cfg, &CommandRunner{Command: script})
	opts.Runs = 2

	// Act
	report, err := RunActivation(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.False(t, report.Failed)
	assert.InDelta(t, 0.02, report.Cost.ActualUSD, 1e-9)
	assert.Equal(t, RunRan, report.Skills[0].Status)
}

func TestRunActivation_UnknownSurface(t *testing.T) {
	_, err := RunActivation(context.Background(), &ActivationOptions{ConfigDir: t.TempDir(), Surface: "psychic"})
	assert.ErrorContains(t, err, "unknown surface")
}

func TestEstimateActivation(t *testing.T) {
	// Arrange
	req := &Request{
		Skills: []SkillRef{{ID: "a", Description: "deploy things to staging"}, {ID: "b", Description: "write release notes"}},
		Cases:  []Case{{ID: "one", Prompt: "deploy the billing service"}, {ID: "two", Prompt: "write a poem"}},
	}
	price := Price{InPerMTok: 1, OutPerMTok: 5}

	// Act
	est := EstimateActivation(EstimateParams{}, req, 5, price, mustCounter(t))

	// Assert
	assert.Equal(t, 10, est.AgentRuns)
	assert.Greater(t, est.InputTokens, 10*harnessOverheadTokens)
	assert.Equal(t, 10*activationOutputTokens, est.OutputTokens)
	assert.Less(t, est.CostLowUSD, est.CostUSD)
	assert.Greater(t, est.CostHighUSD, est.CostUSD)
	assert.Less(t, est.CostHighUSD/est.CostLowUSD, 2.0, "an activation range is tight: one turn, no tool loop")
	custom := EstimateActivation(EstimateParams{OverheadTokens: 25000}, req, 5, price, mustCounter(t))
	assert.Greater(t, custom.InputTokens, est.InputTokens, "a measured overhead replaces the default")
}

// costBlindRunner declares native activation and says it reports no cost.
type costBlindRunner struct{ nativeRunner }

func (costBlindRunner) ReportsCost() bool { return false }

func TestRunActivationNative_MaxCostIsRefusedForARunnerThatReportsNoCost(t *testing.T) {
	tests := []struct {
		name    string
		max     float64
		dry     bool
		wantErr string
	}{
		{name: "a cap cannot be enforced", max: 5, wantErr: "reports no cost"},
		{name: "no cap runs", max: 0},
		{name: "a dry run needs no enforcement", max: 5, dry: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := activationProject(t)
			opts := nativeOptions(cfg, costBlindRunner{scripted(nil)})
			opts.MaxCostUSD, opts.DryRun = tt.max, tt.dry

			// Act
			_, err := RunActivation(context.Background(), opts)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), "--max-cost")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCodexNative_ReportsNoCost(t *testing.T) {
	var r Runner = &CodexNative{}
	blind, ok := r.(CostReporter)
	require.True(t, ok)
	assert.False(t, blind.ReportsCost())
}
