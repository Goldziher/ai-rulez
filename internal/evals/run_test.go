package evals

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner answers from a function and counts calls; it never touches the
// network or a harness.
type fakeRunner struct {
	calls int
	reqs  []*Request
	fn    func(req *Request) (*Response, error)
}

func (f *fakeRunner) Name() string { return "fake" }

func (f *fakeRunner) Run(_ context.Context, req *Request) (*Response, error) {
	f.calls++
	f.reqs = append(f.reqs, req)
	return f.fn(req)
}

// goodRunner triggers on positive cases and answers "ok"; the without arm fails
// the outcome, so the ablation delta is positive.
func goodRunner() *fakeRunner {
	return &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Triggered: bp(c.Expects()), Output: "ok", CostUSD: 0.05, InputTokens: 100, OutputTokens: 20})
			if req.Ablation {
				resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWithout, Output: "bad", CostUSD: 0.04})
			}
		}
		return resp, nil
	}}
}

func writeSkill(t *testing.T, cfg, id, skillBody, cases string) {
	t.Helper()
	dir := filepath.Join(cfg, "skills", id)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "evals"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillBody), 0o600))
	if cases != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "evals", "main.eval.yaml"), []byte(cases), 0o600))
	}
}

const twoCases = `cases:
  - id: fires
    prompt: do the thing
    expect_trigger: true
    near_miss: [something similar]
    assertions:
      - type: contains
        value: ok
  - id: quiet
    prompt: unrelated
    expect_trigger: false
`

func projectWithSkills(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "---\nname: alpha\ndescription: a\n---\nbody alpha\n", twoCases)
	writeSkill(t, cfg, "beta", "---\nname: beta\ndescription: b\n---\nbody beta\n", twoCases)
	writeSkill(t, cfg, "nocases", "---\nname: nocases\ndescription: n\n---\n", "")
	return cfg
}

func baseOptions(cfg string, r Runner) *RunOptions {
	return &RunOptions{ConfigDir: cfg, Runner: r, Harness: "claude", Model: "haiku", Ablation: true, Date: "2026-10-05", Store: NewStore()}
}

func TestRun_ScoresRecordsAndIsDeterministic(t *testing.T) {
	cfg := projectWithSkills(t)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)

	assert.False(t, report.Failed)
	assert.Equal(t, 2, runner.calls)
	require.Len(t, report.Skills, 3)
	assert.Equal(t, RunNoCases, report.Skills[2].Status)
	alpha := report.Skills[0]
	assert.Equal(t, RunRan, alpha.Status)
	assert.True(t, alpha.Passing)
	assert.Equal(t, 3, alpha.CaseCount, "the near miss expands into its own case")
	assert.InDelta(t, 1.0, alpha.Score.PassRate, 1e-9)
	assert.InDelta(t, 1.0, *alpha.Score.AblationDelta, 1e-9)
	assert.Greater(t, alpha.Score.SkillTokens, 0)

	// the near-miss negative case saw the same request the runner got
	var ids []string
	for _, c := range runner.reqs[0].Cases {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"fires", "fires.near-miss-1", "quiet"}, ids)

	first, err := opts.Store.Marshal()
	require.NoError(t, err)
	rec, ok := opts.Store.Get("alpha")
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(rec.Digest, "sha256:"))
	assert.Equal(t, "2026-10-05", rec.Date)
	assert.Equal(t, "fake", rec.Runner)
	assert.Equal(t, rec.Digest, rec.LastPass.Digest)

	// identical inputs, fresh store: byte-identical results file
	opts2 := baseOptions(cfg, goodRunner())
	_, err = Run(context.Background(), opts2)
	require.NoError(t, err)
	second, err := opts2.Store.Marshal()
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

func TestRun_CacheSkipsUnchangedSkillsAndRerunsEditedOnes(t *testing.T) {
	cfg := projectWithSkills(t)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 2, runner.calls)

	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, runner.calls, "unchanged skills are served from the store")
	assert.Equal(t, RunCached, report.Skills[0].Status)
	assert.NotNil(t, report.Skills[0].Score)

	writeSkill(t, cfg, "beta", "---\nname: beta\ndescription: b\n---\nbody beta EDITED\n", twoCases)
	report, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 3, runner.calls)
	assert.Equal(t, RunCached, report.Skills[0].Status)
	assert.Equal(t, RunRan, report.Skills[1].Status)

	opts.Force = true
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 5, runner.calls)

	// a different model is a different cache key
	opts.Force = false
	opts.Model = "sonnet"
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 7, runner.calls)
}

func TestRun_FailingSkillKeepsLastPassAndFlagsStale(t *testing.T) {
	cfg := projectWithSkills(t)
	opts := baseOptions(cfg, goodRunner())
	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	passing, _ := opts.Store.Get("alpha")
	passDigest := passing.Digest

	_, stale := opts.Store.Stale("alpha", passDigest)
	assert.False(t, stale)

	writeSkill(t, cfg, "alpha", "---\nname: alpha\ndescription: a\n---\nchanged\n", twoCases)
	newDigest, err := SkillDigest(filepath.Join(cfg, "skills", "alpha"))
	require.NoError(t, err)
	last, stale := opts.Store.Stale("alpha", newDigest)
	assert.True(t, stale)
	assert.Equal(t, passDigest, last.Digest)

	// a failing re-run keeps the old pass mark
	failing := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			resp.Results = append(resp.Results, Result{Case: req.Cases[i].ID, Arm: ArmWith, Triggered: bp(false)})
		}
		return resp, nil
	}}
	opts.Runner = failing
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, report.Failed)
	rec, _ := opts.Store.Get("alpha")
	assert.False(t, rec.Passing)
	assert.Equal(t, passDigest, rec.LastPass.Digest)
	_, stale = opts.Store.Stale("alpha", rec.Digest)
	assert.True(t, stale, "the last passing run predates the current content")
}

func TestRun_ThresholdControlsPassing(t *testing.T) {
	cfg := projectWithSkills(t)
	half := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Triggered: bp(c.Expects()), Output: map[bool]string{true: "ok", false: "bad"}[c.ID != "fires"]})
		}
		return resp, nil
	}}
	opts := baseOptions(cfg, half)
	opts.Skills = []string{"alpha"}
	opts.Ablation = false
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, report.Skills[0].Passing)
	assert.True(t, report.Failed)

	threshold := 0.6
	opts.PassThreshold = &threshold
	opts.Force = true
	report, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, report.Skills[0].Passing)
	assert.False(t, report.Failed)
}

func TestRun_DryRunListsWorkWithoutRunning(t *testing.T) {
	cfg := projectWithSkills(t)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	opts.DryRun = true
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Zero(t, runner.calls)
	assert.True(t, report.DryRun)
	assert.Equal(t, RunDryRun, report.Skills[0].Status)
	// 2 skills x 3 cases x 2 arms
	assert.Equal(t, 12, report.Estimate.AgentRuns)
	assert.Greater(t, report.Estimate.CostUSD, 0.0)
	assert.Empty(t, opts.Store.Skills)

	// dry run needs no runner at all
	opts.Runner = nil
	report, err = Run(context.Background(), opts)
	require.NoError(t, err)

	// and is deterministic
	a, _ := json.Marshal(report)
	again, err := Run(context.Background(), opts)
	require.NoError(t, err)
	b, _ := json.Marshal(again)
	assert.JSONEq(t, string(a), string(b))
}

func TestRun_MaxCostPreflightAndBudgetStop(t *testing.T) {
	cfg := projectWithSkills(t)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	opts.MaxCostUSD = 0.0001
	_, err := Run(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds --max-cost")
	assert.Zero(t, runner.calls)

	// a generous estimate limit but actual spend exceeds it after the first skill
	opts.MaxCostUSD = 10
	spender := &fakeRunner{fn: func(req *Request) (*Response, error) {
		return &Response{Version: ProtocolVersion, Results: []Result{
			{Case: "fires", Arm: ArmWith, Triggered: bp(true), Output: "ok", CostUSD: 11},
		}}, nil
	}}
	opts.Runner = spender
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, spender.calls)
	assert.Equal(t, RunOverBudget, report.Skills[1].Status)
	assert.True(t, report.Failed)
	assert.InDelta(t, 10, spender.reqs[0].MaxCostUSD, 1e-9)
}

func TestRun_ChangedOnlyAndSkillFilter(t *testing.T) {
	cfg := projectWithSkills(t)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	opts.Changed = map[string]bool{"beta": true}
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, runner.calls)
	assert.Equal(t, RunNotChanged, report.Skills[0].Status)

	opts.Changed = nil
	opts.Skills = []string{"missing"}
	_, err = Run(context.Background(), opts)
	assert.ErrorContains(t, err, "unknown skill")
}

func TestRun_InvalidCasesAndRunnerErrors(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "broken", "x", "cases:\n  - id: a\n    prompt: p\n")
	writeSkill(t, cfg, "ok", "x", twoCases)
	runner := &fakeRunner{fn: func(*Request) (*Response, error) { return nil, errors.New("harness exploded") }}
	report, err := Run(context.Background(), baseOptions(cfg, runner))
	require.NoError(t, err)
	assert.True(t, report.Failed)
	assert.Equal(t, RunInvalid, report.Skills[0].Status)
	assert.Contains(t, report.Skills[0].Problems[0].Message, "expect_trigger is required")
	assert.Equal(t, RunError, report.Skills[1].Status)
	assert.Equal(t, 1, runner.calls, "an invalid skill is never sent to the runner")
}

func TestStore_RoundTripAndVersionCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eval-results.json")
	store := NewStore()
	store.Put(SkillRecord{ID: "z", Digest: "sha256:1", Passing: true, Date: "d1"})
	store.Put(SkillRecord{ID: "a", Digest: "sha256:2"})
	require.NoError(t, store.Save(path))
	data, _ := os.ReadFile(path)
	assert.Less(t, strings.Index(string(data), `"a"`), strings.Index(string(data), `"z"`), "sorted by id")
	assert.True(t, strings.HasSuffix(string(data), "\n"))

	loaded, err := LoadStore(path)
	require.NoError(t, err)
	z, _ := loaded.Get("z")
	assert.Equal(t, "d1", z.LastPass.Date)

	missing, err := LoadStore(filepath.Join(dir, "none.json"))
	require.NoError(t, err)
	assert.Empty(t, missing.Skills)

	require.NoError(t, os.WriteFile(path, []byte(`{"schema_version":99,"skills":[]}`), 0o600))
	_, err = LoadStore(path)
	assert.ErrorContains(t, err, "schema_version 99")
}

func TestFormats_JUnitMarkdownJSON(t *testing.T) {
	cfg := projectWithSkills(t)
	failing := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			r := Result{Case: c.ID, Arm: ArmWith, Triggered: bp(c.Expects()), Output: "ok"}
			switch c.ID {
			case "quiet":
				r.Triggered = bp(true) // false positive -> failure
			case "fires.near-miss-1":
				r.Error = "timeout" // error
			}
			resp.Results = append(resp.Results, r)
		}
		return resp, nil
	}}
	opts := baseOptions(cfg, failing)
	opts.Skills = []string{"alpha"}
	opts.Ablation = false
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)

	var junit strings.Builder
	require.NoError(t, report.Write(&junit, FormatJUnit))
	var doc junitSuites
	require.NoError(t, xml.Unmarshal([]byte(junit.String()), &doc))
	assert.Equal(t, 3, doc.Tests)
	assert.Equal(t, 1, doc.Failures)
	assert.Equal(t, 1, doc.Errors)
	require.Len(t, doc.Suites, 1)
	assert.Equal(t, "alpha", doc.Suites[0].Name)
	assert.Contains(t, junit.String(), `classname="skill.alpha"`)

	var again strings.Builder
	require.NoError(t, report.Write(&again, FormatJUnit))
	assert.Equal(t, junit.String(), again.String())

	var md strings.Builder
	require.NoError(t, report.Write(&md, FormatMarkdown))
	assert.Contains(t, md.String(), "| alpha | ran, failing | 3 |")
	assert.Contains(t, md.String(), "`quiet`: skill triggered but should not have")

	var js strings.Builder
	require.NoError(t, report.Write(&js, FormatJSON))
	assert.True(t, json.Valid([]byte(js.String())))
	assert.Error(t, report.Write(&js, "yaml"))
}

func TestChangedSkills_UsesGitDiffAndUntracked(t *testing.T) {
	cfg := projectWithSkills(t)
	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	top := filepath.Dir(cfg)
	rel := func(p string) string { r, _ := filepath.Rel(top, p); return filepath.ToSlash(r) }
	git := func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "rev-parse":
			return top + "\n", nil
		case "diff":
			assert.Equal(t, []string{"diff", "--name-only", "origin/main", "--"}, args)
			return rel(filepath.Join(cfg, "skills", "alpha", "SKILL.md")) + "\nREADME.md\n", nil
		default:
			return rel(filepath.Join(cfg, "skills", "beta", "evals", "new.eval.yaml")) + "\n", nil
		}
	}
	changed, err := ChangedSkills(git, cfg, "origin/main", skills)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"alpha": true, "beta": true}, changed)

	_, err = ChangedSkills(func(string, ...string) (string, error) { return "", errors.New("no repo") }, cfg, "", skills)
	assert.ErrorContains(t, err, "git repository")
}

func TestEstimateRun_ScalesWithArmsRunsAndRubric(t *testing.T) {
	cases := []Case{{ID: "a", Prompt: "hello", ExpectTrigger: bp(true)}, {ID: "b", Prompt: "hello", ExpectTrigger: bp(true), Rubric: "good"}}
	base := &Request{Cases: cases}
	one := EstimateRun(base, 500, 1, Price{InPerMTok: 3, OutPerMTok: 15}, mustCounter(t))
	assert.Equal(t, 2, one.AgentRuns)
	three := EstimateRun(base, 500, 3, Price{InPerMTok: 3, OutPerMTok: 15}, mustCounter(t))
	assert.Equal(t, 6, three.AgentRuns)
	assert.InDelta(t, one.CostUSD*3, three.CostUSD, 1e-3)
	abl := EstimateRun(&Request{Cases: cases, Ablation: true}, 500, 1, Price{InPerMTok: 3, OutPerMTok: 15}, mustCounter(t))
	assert.Equal(t, 4, abl.AgentRuns)
	assert.Greater(t, abl.CostUSD, one.CostUSD)
}

func erroringRunner() *fakeRunner {
	return &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			resp.Results = append(resp.Results, Result{Case: req.Cases[i].ID, Arm: ArmWith, Error: "rate limited"})
		}
		return resp, nil
	}}
}

func TestRun_ErroredRunsAreNeverCached(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "x", twoCases)

	for name, runner := range map[string]*fakeRunner{
		"per-case errors": erroringRunner(),
		"no results":      {fn: func(*Request) (*Response, error) { return &Response{Version: ProtocolVersion}, nil }},
	} {
		t.Run(name, func(t *testing.T) {
			opts := baseOptions(cfg, runner)
			_, err := Run(context.Background(), opts)
			require.NoError(t, err)
			require.Equal(t, 1, runner.calls)
			report, err := Run(context.Background(), opts)
			require.NoError(t, err)
			assert.Equal(t, 2, runner.calls, "an errored run is retried, not replayed")
			assert.Equal(t, RunRan, report.Skills[0].Status)
			assert.True(t, report.Failed)
		})
	}
}

func TestRun_StoredErroredRecordIsNotReplayed(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "x", twoCases)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	rec, _ := opts.Store.Get("alpha")
	rec.Score.Errors = 1 // an older ai-rulez stored an errored run under a valid key
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, runner.calls)
}

// printRunner is goodRunner with a fingerprint, like a runner with settings.
type printRunner struct {
	*fakeRunner
	print string
}

func (p *printRunner) Fingerprint() string { return p.print }

func TestRun_CacheKeyCoversRunnerSettingsAndExecFlag(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "x", twoCases)
	inner := goodRunner()
	runner := &printRunner{fakeRunner: inner, print: "cmd=a"}
	opts := baseOptions(cfg, runner)
	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls, "same settings hit the cache")

	runner.print = "cmd=b"
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, inner.calls, "a different runner command re-runs")

	opts.Grade.AllowExec = true
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 3, inner.calls, "--allow-exec changes command_exit grading")

	opts.ToolVersion = "9.9.9"
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 4, inner.calls, "a new ai-rulez version re-runs")
}

func TestRun_ThresholdZeroDoesNotGate(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "x", twoCases)
	runner := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			resp.Results = append(resp.Results, Result{Case: req.Cases[i].ID, Arm: ArmWith, Triggered: bp(!req.Cases[i].Expects()), Output: "bad"})
		}
		return resp, nil
	}}
	opts := baseOptions(cfg, runner)
	zero := 0.0
	opts.PassThreshold = &zero
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, report.Skills[0].Passing)
	assert.False(t, report.Failed)
}

func TestRun_SpendIsCountedConservatively(t *testing.T) {
	cfg := projectWithSkills(t)
	// the runner's total exceeds the sum of per-case costs (a case went missing)
	underCounted := &fakeRunner{fn: func(req *Request) (*Response, error) {
		return &Response{Version: ProtocolVersion, CostUSD: 11, Results: []Result{
			{Case: "fires", Arm: ArmWith, Triggered: bp(true), Output: "ok", CostUSD: 1},
		}}, nil
	}}
	opts := baseOptions(cfg, underCounted)
	opts.MaxCostUSD = 10
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, underCounted.calls)
	assert.Equal(t, RunOverBudget, report.Skills[1].Status)

	// a runner that fails may still have spent money, so with a cap set the run stops
	cfg = projectWithSkills(t)
	failing := &fakeRunner{fn: func(*Request) (*Response, error) { return nil, errors.New("exit status 1") }}
	opts = baseOptions(cfg, failing)
	opts.MaxCostUSD = 10
	report, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, RunError, report.Skills[0].Status)
	assert.Equal(t, 1, failing.calls, "a failed call may have spent the whole budget")
	assert.Equal(t, RunOverBudget, report.Skills[1].Status)

	// without a cap a failing runner is still tried for every skill
	failing.calls = 0
	opts.MaxCostUSD = 0
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, failing.calls)
}

func TestRun_OnSkillSeesEachFinishedSkillAndInterruptKeepsThem(t *testing.T) {
	cfg := projectWithSkills(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	var seen []string
	opts.OnSkill = func(run *SkillRun) error {
		if run.Status == RunRan {
			_, ok := opts.Store.Get(run.ID)
			assert.True(t, ok, "the record is in the store when the hook fires")
			cancel() // simulate Ctrl-C after the first expensive skill
		}
		seen = append(seen, run.ID)
		return nil
	}
	report, err := Run(ctx, opts)
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, report)
	assert.Equal(t, []string{"alpha"}, seen)
	assert.Equal(t, 1, runner.calls)
	assert.True(t, report.Failed)
	assert.Len(t, report.Skills, 1)
}

func TestRun_RejectsBadBudgetAndThreshold(t *testing.T) {
	cfg := projectWithSkills(t)
	for name, mutate := range map[string]func(*RunOptions){
		"nan max cost":      func(o *RunOptions) { o.MaxCostUSD = math.NaN() },
		"negative max cost": func(o *RunOptions) { o.MaxCostUSD = -5 },
		"inf max cost":      func(o *RunOptions) { o.MaxCostUSD = math.Inf(1) },
		"threshold over 1":  func(o *RunOptions) { v := 1.5; o.PassThreshold = &v },
		"negative price":    func(o *RunOptions) { o.Price = Price{InPerMTok: -1} },
		"nan price":         func(o *RunOptions) { o.Price = Price{OutPerMTok: math.NaN()} },
	} {
		t.Run(name, func(t *testing.T) {
			runner := goodRunner()
			opts := baseOptions(cfg, runner)
			mutate(opts)
			_, err := Run(context.Background(), opts)
			require.Error(t, err)
			assert.Zero(t, runner.calls)
		})
	}
}

func realGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := gitutil.CommandNoContext(dir, append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func TestChangedSkills_ProjectInASubdirectoryOfTheRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	project := filepath.Join(repo, "services", "foo")
	cfg := filepath.Join(project, ".ai-rulez")
	writeSkill(t, cfg, "old", "x", twoCases)
	realGit(t, repo, "init", "-q")
	realGit(t, repo, "add", "-A")
	realGit(t, repo, "commit", "-q", "-m", "init")
	// a brand-new untracked skill, and a tracked one that was edited
	writeSkill(t, cfg, "fresh", "x", twoCases)
	writeSkill(t, cfg, "old", "x edited", twoCases)

	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	changed, err := ChangedSkills(ExecGit, project, "HEAD", skills)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"fresh": true, "old": true}, changed)
}

func TestChangedSkills_RejectsOptionLikeBase(t *testing.T) {
	git := func(string, ...string) (string, error) {
		t.Fatal("git must not be called with an option-like base")
		return "", nil
	}
	_, err := ChangedSkills(git, t.TempDir(), "--output=/tmp/evil", nil)
	assert.ErrorContains(t, err, "base")
}

func TestRun_InvalidCaseProblemsAreProjectRelative(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".ai-rulez")
	writeSkill(t, cfg, "broken", "x", "cases:\n  - id: a\n    prompt: p\n")
	report, err := Run(context.Background(), baseOptions(cfg, goodRunner()))
	require.NoError(t, err)
	require.Len(t, report.Skills[0].Problems, 1)
	assert.Equal(t, ".ai-rulez/skills/broken/evals/main.eval.yaml", report.Skills[0].Problems[0].File)
}
