package improve

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// nativeSiblings is a fake harness for the native sibling guard: while the deploy skill's description holds
// the word STEAL, every prompt that expects a trigger loads deploy instead of its own skill.
type nativeSiblings struct {
	mu       sync.Mutex
	requests []*evals.Request
	surfaces []string
	cost     float64
}

func (n *nativeSiblings) Name() string           { return "fake-native" }
func (n *nativeSiblings) Capabilities() []string { return []string{evals.CapabilityActivation} }
func (n *nativeSiblings) Surfaces() []string {
	if n.surfaces != nil {
		return n.surfaces
	}
	return []string{evals.SurfaceNative}
}

func (n *nativeSiblings) Run(_ context.Context, req *evals.Request) (*evals.Response, error) {
	n.mu.Lock()
	n.requests = append(n.requests, req)
	n.mu.Unlock()
	steal := false
	for _, s := range req.Skills {
		steal = steal || s.ID == "deploy" && strings.Contains(s.Description, "STEAL")
	}
	resp := &evals.Response{Version: evals.ProtocolVersion}
	for i := range req.Cases {
		c := &req.Cases[i]
		fired := map[string]int{}
		switch {
		case !c.Expects():
		case steal:
			fired["deploy"] = req.Runs
		default:
			fired[c.Target] = req.Runs
		}
		resp.Results = append(resp.Results, evals.Result{
			Case: c.ID, Arm: evals.ArmWith, Runs: req.Runs, FiredCounts: fired,
			InputTokens: 100, OutputTokens: 10, CostUSD: n.cost / float64(len(req.Cases)),
		})
	}
	return resp, nil
}

func (n *nativeSiblings) calls() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.requests)
}

func nativeGuardOptions(t *testing.T, native *nativeSiblings, optimizerEdit string) (Options, *fakeEval) {
	t.Helper()
	root, configDir := project(t)
	withSibling(t, configDir)
	ev := goodEval()
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) { setDescription(t, dir, optimizerEdit) })
	o := baseOptions(root, configDir, ev, opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	o.SiblingNative = &SiblingNative{Runner: native, Runs: 3}
	return o, ev
}

func TestExecute_NativeSiblingGuardRejectsWhatTheModelRoutesAway(t *testing.T) {
	// Arrange: the offline ranker sees nothing wrong with the description, the harness's model does
	native := &nativeSiblings{cost: 0.30}
	o, ev := nativeGuardOptions(t, native, "Deploy services to staging. STEAL. Use when asked to deploy a service.")
	plan := mustPrepare(t, &o)
	callsBefore := ev.callCount()

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, StatusNoCandidate, report.Status)
	rd := report.Rounds[0]
	assert.Equal(t, "rejected: regression", rd.Decision)
	require.NotNil(t, rd.Siblings)
	assert.Empty(t, rd.Siblings.Regressions(), "the free ranker passed it")
	require.NotNil(t, rd.SiblingsNative)
	assert.Equal(t, evals.SurfaceNative, rd.SiblingsNative.Surface)
	require.Len(t, rd.SiblingsNative.Regressions(), 1)
	assert.Equal(t, "rollback", rd.SiblingsNative.Regressions()[0].Skill)
	assert.Contains(t, strings.Join(rd.Reasons, " "), CodeSiblingRegression)
	assert.Nil(t, rd.Held, "no held-out evaluation was spent on a candidate the guard rejects")
	assert.Equal(t, 2, ev.callCount()-callsBefore, "only the two baseline measurements ran")
	assert.Greater(t, report.Costs.EvalUSD, 0.0, "the native guard's spend counts against --max-cost")
	assert.Greater(t, rd.SiblingsNative.CostUSD, 0.0)
	assertReportSchema(t, report)
}

func TestExecute_NativeSiblingGuardPassesAHarmlessEdit(t *testing.T) {
	// Arrange
	native := &nativeSiblings{cost: 0.10}
	o, _ := nativeGuardOptions(t, native, "Deploy services to staging. Use when asked to deploy, ship or push a service to staging.")
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, StatusAccepted, report.Status)
	sn := report.Rounds[0].SiblingsNative
	require.NotNil(t, sn)
	assert.Empty(t, sn.Regressions())
	require.Len(t, sn.Results, 1)
	assert.Equal(t, 3, sn.Runs)
	assert.Equal(t, 2, native.calls(), "one baseline and one candidate measurement of the one sibling")
	assertReportSchema(t, report)
}

func TestExecute_NativeSiblingGuardMeasuresTheBaselineOnce(t *testing.T) {
	// Arrange: two rounds, each with a description edit
	native := &nativeSiblings{cost: 0.10}
	o, _ := nativeGuardOptions(t, native, "Deploy services to staging. Use when asked to deploy a service to staging.")
	o.MaxRounds = 2
	o.StopAtFirstAccept = false
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	measured := 0
	for _, rd := range report.Rounds {
		if rd.SiblingsNative != nil && rd.SiblingsNative.Skipped == "" {
			measured++
		}
	}
	assert.Equal(t, native.calls(), measured+1, "the original is measured once, then each round's candidate")
}

func TestExecute_NativeSiblingGuardNeedsARunnerThatDeclaresTheSurface(t *testing.T) {
	// Arrange
	native := &nativeSiblings{surfaces: []string{"other"}}
	o, ev := nativeGuardOptions(t, native, "Deploy services to staging. Use when asked to deploy a service.")
	plan := mustPrepare(t, &o)
	callsBefore := ev.callCount()

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	rd := report.Rounds[0]
	assert.Equal(t, "rejected: sibling guard failed", rd.Decision)
	assert.Contains(t, strings.Join(rd.Reasons, " "), "native sibling trigger guard could not run")
	assert.Zero(t, native.calls())
	assert.Equal(t, 2, ev.callCount()-callsBefore)
}

func TestExecute_NativeSiblingGuardStaysOffUnlessAsked(t *testing.T) {
	// Arrange
	root, configDir := project(t)
	withSibling(t, configDir)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
		setDescription(t, dir, "Deploy services to staging. Use when asked to deploy, ship or push a service to staging.")
	})
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Nil(t, report.Rounds[0].SiblingsNative)
}

func TestExecute_NativeSiblingGuardIsSkippedForABodyOnlyEdit(t *testing.T) {
	// Arrange
	native := &nativeSiblings{cost: 0.10}
	root, configDir := project(t)
	withSibling(t, configDir)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) { appendSkill(t, dir, "\nGOOD advice.\n") })
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	o.SiblingNative = &SiblingNative{Runner: native}
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Zero(t, native.calls(), "a body-only edit changes nothing the model chooses by, so it costs nothing")
	require.NotNil(t, report.Rounds[0].SiblingsNative)
	assert.Contains(t, report.Rounds[0].SiblingsNative.Skipped, "ranker reads")
}
