package review

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// twoSkills is a project of two clearly different skills.
func twoSkills(t *testing.T, extra ...lint.Finding) (*Rubric, *Results) {
	t.Helper()
	rb := builtin(t)
	a := skill("a", "Deploy the service to staging when asked to ship a build; not for rollbacks")
	b := skill("b", "Summarise a pull request for the changelog; not for releases")
	return rb, Run(Input{Rubric: rb, Items: []Item{a, b}, Findings: extra})
}

func semDim(t *testing.T, res *Results, item, dim string) SemDim {
	t.Helper()
	for i := range res.Items {
		if res.Items[i].ID == item && res.Items[i].Semantic != nil {
			for _, d := range res.Items[i].Semantic.Dimensions {
				if d.ID == dim {
					return d
				}
			}
		}
	}
	t.Fatalf("no judged dimension %s of %s", dim, item)
	return SemDim{}
}

func TestAFirstVotePassIsAcceptedWithoutExtraVotes(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{})

	// Act
	out := mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3}})

	// Assert: one intrinsic and one contextual call per item, nothing more
	assert.Equal(t, 4, sj.count())
	assert.Equal(t, 4, out.Usage.Calls)
	d := semDim(t, res, "skill:a", "trigger-quality")
	assert.Equal(t, SemJudged, d.Status)
	assert.Equal(t, VerdictPass, d.Verdict)
	assert.Equal(t, []string{VerdictPass}, d.Votes)
	require.NotNil(t, res.Items[0].Semantic.Score)
	assert.Equal(t, 100, *res.Items[0].Semantic.Score)
}

func TestFlaggedDimensionsAreVotedAndAggregated(t *testing.T) {
	tests := []struct {
		name       string
		verdict    func(item, dim string, vote int) string
		k          int
		wantVotes  []string
		wantFinal  string
		wantStatus string
		wantAgree  float64
		wantCalls  int
	}{
		{
			name:      "unanimous flags stop after two votes",
			verdict:   func(_, dim string, _ int) string { return pick(dim == "trigger-quality", VerdictWarn) },
			k:         3,
			wantVotes: []string{"warn", "warn"}, wantFinal: VerdictWarn, wantStatus: SemJudged, wantAgree: 1, wantCalls: 2,
		},
		{
			name: "disagreement takes all votes, the median wins and the verdict is unstable",
			verdict: func(_, dim string, vote int) string {
				if dim != "trigger-quality" {
					return VerdictPass
				}
				return []string{"", VerdictFail, VerdictWarn, VerdictPass}[vote]
			},
			k:         3,
			wantVotes: []string{"fail", "warn", "pass"}, wantFinal: VerdictWarn, wantStatus: SemUnstable, wantAgree: 0.333, wantCalls: 3,
		},
		{
			name: "two of three agreeing is below the 0.67 instability threshold",
			verdict: func(_, dim string, vote int) string {
				if dim != "trigger-quality" {
					return VerdictPass
				}
				return []string{"", VerdictFail, VerdictWarn, VerdictWarn}[vote]
			},
			k:         3,
			wantVotes: []string{"fail", "warn", "warn"}, wantFinal: VerdictWarn, wantStatus: SemUnstable, wantAgree: 0.667, wantCalls: 3,
		},
		{
			name:      "k=1 is one vote and never unstable",
			verdict:   func(_, dim string, _ int) string { return pick(dim == "trigger-quality", VerdictFail) },
			k:         1,
			wantVotes: []string{"fail"}, wantFinal: VerdictFail, wantStatus: SemJudged, wantAgree: 1, wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rb := builtin(t)
			res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Deploy the service to staging when asked to ship a build; not for rollbacks")}})
			sj := &scriptedJudge{verdict: tt.verdict}
			client, _ := newClient(t, sj, llm.Config{})

			// Act
			mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: tt.k}})

			// Assert
			d := semDim(t, res, "skill:a", "trigger-quality")
			assert.Equal(t, tt.wantVotes, d.Votes)
			assert.Equal(t, tt.wantFinal, d.Verdict)
			assert.Equal(t, tt.wantStatus, d.Status)
			assert.InDelta(t, tt.wantAgree, d.Agreement, 0.001)
			assert.Len(t, sj.callsFor("a"), tt.wantCalls, "no contextual call: the item has no sibling")
		})
	}
}

func pick(cond bool, verdict string) string {
	if cond {
		return verdict
	}
	return VerdictPass
}

func TestVotesAreCachedSeparatelyAndReproduced(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	sj := &scriptedJudge{verdict: func(_, dim string, _ int) string { return pick(dim == "trigger-quality", VerdictWarn) }}
	client, fake := newClient(t, sj, llm.Config{})
	first := mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3}})
	before := len(fake.ChatCalls())

	// Act: the same run over fresh results
	again := Run(Input{Rubric: rb, Items: []Item{skill("a", "Deploy the service to staging when asked to ship a build; not for rollbacks"), skill("b", "Summarise a pull request for the changelog; not for releases")}})
	second := mustRun(t, SemanticInput{Rubric: rb, Results: again, Options: SemanticOptions{Client: client, K: 3}})

	// Assert
	assert.Positive(t, first.Usage.Calls)
	assert.Zero(t, second.Usage.Calls, "unchanged content costs nothing on re-run")
	assert.Equal(t, first.Usage.Calls, second.Usage.Cached)
	assert.Len(t, fake.ChatCalls(), before)
	assert.Equal(t, semDim(t, res, "skill:a", "trigger-quality").Votes, semDim(t, again, "skill:a", "trigger-quality").Votes)
}

func TestAnInventedQuoteDropsTheVerdictAndIsCounted(t *testing.T) {
	// Arrange
	rb := builtin(t)
	res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Deploy the service to staging")}})
	sj := &scriptedJudge{
		verdict: func(_, dim string, _ int) string { return pick(dim == "body-accuracy", VerdictFail) },
		quote:   func(_, _, _ string) string { return "text the item never contained" },
	}
	client, _ := newClient(t, sj, llm.Config{})

	// Act
	out := mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3, Content: config.ReviewContentFull}})

	// Assert
	d := semDim(t, res, "skill:a", "body-accuracy")
	assert.Equal(t, VerdictPass, d.Verdict, "a claim the data cannot back is not a finding")
	assert.Equal(t, 1, d.DroppedVotes)
	assert.Positive(t, out.Usage.Hallucinated)
	assert.Len(t, sj.callsFor("a"), 1, "a dropped verdict is a pass, so no vote follows")
}

func TestAMalformedReplyIsRetriedOnceThenIsAnErrorNeverAPass(t *testing.T) {
	// Arrange
	rb := builtin(t)
	res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Deploy the service to staging")}})
	fake := llm.NewFake()
	fake.ChatFunc = func(llm.ChatRequest) (string, error) { return "I think it is fine.", nil }
	cfg := llm.Config{AllowNetwork: true, Model: "fake/model"}
	client := llm.Wrap(fake, cfg, llm.Options{})

	// Act
	out := mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3}})

	// Assert
	d := semDim(t, res, "skill:a", "trigger-quality")
	assert.Equal(t, SemError, d.Status)
	assert.Empty(t, d.Verdict)
	assert.Len(t, fake.ChatCalls(), 2, "one retry")
	assert.Equal(t, 2, out.Usage.Calls)
	assert.Nil(t, res.Items[0].Semantic.Score, "nothing was judged")
}

func TestAMalformedFirstReplyThenAGoodOneWorks(t *testing.T) {
	// Arrange
	rb := builtin(t)
	res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Deploy the service to staging")}})
	sj := &scriptedJudge{}
	fake := llm.NewFake()
	n := 0
	fake.ChatFunc = func(req llm.ChatRequest) (string, error) {
		n++
		if n == 1 {
			return "```not json```", nil
		}
		return sj.chat(req)
	}
	client := llm.Wrap(fake, llm.Config{AllowNetwork: true, Model: "fake/model"}, llm.Options{})

	// Act
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})

	// Assert
	assert.Equal(t, VerdictPass, semDim(t, res, "skill:a", "trigger-quality").Verdict)
	assert.Equal(t, 2, n)
}

func TestTheSpendCapEndsTheRunIncomplete(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{MaxCalls: 2})

	// Act
	out, err := RunSemantic(t.Context(), SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3, Workers: 1}})

	// Assert
	require.NoError(t, err, "a reached cap is a report, not an error")
	assert.True(t, out.Incomplete)
	assert.Equal(t, "the spend cap was reached", out.StoppedBecause)
	assert.Equal(t, 2, out.Usage.Calls)
	assert.Equal(t, []string{"skill:b"}, out.Unjudged)
	assert.NotNil(t, res.Items[0].Semantic)
	assert.Nil(t, res.Items[1].Semantic)
}

func TestARefusedNetworkStopsTheRunWithAFatalError(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	managed, err := llm.New(llm.Config{Model: "fake/model"}, llm.Options{}) // allow_network defaults to false
	require.NoError(t, err)

	// Act
	out, runErr := RunSemantic(t.Context(), SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: managed, K: 1}})

	// Assert
	require.Error(t, runErr)
	assert.True(t, errors.Is(runErr, ErrFatal))
	assert.True(t, errors.Is(runErr, llm.ErrNetworkDisabled))
	assert.True(t, out.Incomplete)
}

func TestRepeatedFailuresStopTheRun(t *testing.T) {
	// Arrange
	rb := builtin(t)
	var items []Item
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		items = append(items, skill(n, "Deploy the service number "+n+" to staging"))
	}
	res := Run(Input{Rubric: rb, Items: items})
	fake := llm.NewFake()
	fake.ChatFunc = func(llm.ChatRequest) (string, error) {
		return "", &llm.Error{Kind: llm.KindProvider, Status: 400, Message: "bad schema"}
	}
	client := llm.Wrap(fake, llm.Config{AllowNetwork: true, Model: "fake/model", MaxRetries: -1}, llm.Options{})

	// Act
	_, err := RunSemantic(t.Context(), SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1, Workers: 1}})

	// Assert
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFatal))
	assert.LessOrEqual(t, len(fake.ChatCalls()), maxConsecutiveFailures+1, "a systematic failure must not burn through every item")
}

func TestDescriptionsModeJudgesOnlyWhatDescriptionsCanAnswer(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{})

	// Act
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})

	// Assert
	for _, c := range sj.callsFor("a") {
		for _, d := range c.Dims {
			assert.Contains(t, []string{"trigger-quality", "overlap"}, d, "a dimension that needs the body is not guessed from a description")
		}
		assert.NotContains(t, c.User, "--- body ---")
	}
	assert.Equal(t, SemSkipped, semDim(t, res, "skill:a", "body-accuracy").Status)
	assert.Contains(t, semDim(t, res, "skill:a", "body-accuracy").Note, "--content full")
}

func TestFullModeSendsTheBodyAndJudgesTheBodyDimensions(t *testing.T) {
	// Arrange
	rb := builtin(t)
	it := skill("a", "Deploy the service to staging")
	it.Body = "Run the deploy script, then check the logs.\n"
	it.Frontmatter = "name: a\ndescription: Deploy the service to staging\nallowed-tools: Bash"
	res := Run(Input{Rubric: rb, Items: []Item{it}})
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{})

	// Act
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1, Content: config.ReviewContentFull}})

	// Assert
	calls := sj.callsFor("a")
	require.NotEmpty(t, calls)
	assert.Contains(t, calls[0].User, "--- body ---\nRun the deploy script")
	assert.Contains(t, calls[0].User, "allowed-tools: Bash", "the frontmatter is sent in full mode, so scope-creep can see the tools")
	assert.ElementsMatch(t, []string{"trigger-quality", "body-accuracy", "injection-intent", "scope-creep", "body-structure"}, calls[0].Dims)
}

func TestATwinErrorPreemptsTheJudgedDimension(t *testing.T) {
	// Arrange
	rb := builtin(t)
	a := skill("a", "Deploy the service to staging")
	res := Run(Input{Rubric: rb, Items: []Item{a}, Findings: []lint.Finding{finding("AR201", lint.SeverityError, a, 3, "broken link")}})
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{})

	// Act
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1, Content: config.ReviewContentFull}})

	// Assert
	d := semDim(t, res, "skill:a", "body-accuracy")
	assert.Equal(t, SemPreempted, d.Status)
	assert.Equal(t, []string{"AR201"}, d.PreemptedBy)
	for _, c := range sj.callsFor("a") {
		assert.NotContains(t, c.Dims, "body-accuracy", "the judge is not asked what lint already answered")
	}
}

func TestSiblingsAreSentToTheContextualCallOnly(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{})

	// Act
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})

	// Assert
	var contextual, intrinsic int
	for _, c := range sj.callsFor("a") {
		if strings.Contains(c.User, "sibling skill:b:") {
			contextual++
			assert.Contains(t, c.Dims, "overlap")
		} else {
			intrinsic++
		}
	}
	assert.Equal(t, 1, contextual)
	assert.Equal(t, 1, intrinsic)
}

func TestRedactModeSendsTheMaskedItemAndNeverTheCredential(t *testing.T) {
	// Arrange
	rb := builtin(t)
	leak := skill("leak", "Rotate the cloud credentials when asked, key AKIAIOSFODNN7EXAMPLE")
	leak.Raw = "---\nname: leak\ndescription: Rotate the cloud credentials, key AKIAIOSFODNN7EXAMPLE\n---\nbody\n"
	hidden := skill("hidden", "Deploy​ the service")
	hidden.Raw = "---\nname: hidden\ndescription: Deploy​ the service\n---\n"
	other := skill("other", "Summarise a pull request for the changelog")

	t.Run("on_secret = redact masks the credential", func(t *testing.T) {
		res := Run(Input{Rubric: rb, Items: []Item{leak, hidden, other}, Config: &config.ReviewConfig{OnSecret: config.ReviewOnSecretRedact}})
		sj := &scriptedJudge{}
		client, _ := newClient(t, sj, llm.Config{})

		mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})

		byID := map[string]ItemResult{}
		for _, r := range res.Items {
			byID[r.ID] = r
		}
		assert.Equal(t, StatusScored, byID["skill:leak"].Status)
		assert.True(t, byID["skill:leak"].Redacted)
		assert.Equal(t, StatusWithheld, byID["skill:hidden"].Status, "hidden characters are always withheld")
		for _, c := range sj.calls {
			assert.NotContains(t, c.User, "AKIAIOSFODNN7EXAMPLE", "the credential never reaches the model, not even as a sibling")
		}
		assert.NotEmpty(t, sj.callsFor("leak"))
		assert.Contains(t, sj.callsFor("leak")[0].User, "[REDACTED:AR001]")
	})

	t.Run("the default withholds", func(t *testing.T) {
		res := Run(Input{Rubric: rb, Items: []Item{leak, other}})
		for _, r := range res.Items {
			if r.ID == "skill:leak" {
				assert.Equal(t, StatusWithheld, r.Status)
			}
		}
	})
}

func TestMedianVerdict(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{[]string{"pass"}, "pass"},
		{[]string{"warn", "fail"}, "warn"},
		{[]string{"fail", "pass"}, "pass"},
		{[]string{"fail", "warn", "pass"}, "warn"},
		{[]string{"fail", "fail", "pass"}, "fail"},
		{[]string{"pass", "pass", "fail", "fail"}, "pass"},
		{nil, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, MedianVerdict(tt.in), "%v", tt.in)
	}
}

func TestSemanticScoreFormula(t *testing.T) {
	tests := []struct {
		name string
		dims []SemDim
		want *int
	}{
		{"all pass", []SemDim{{Weight: 0.5, Status: SemJudged, Verdict: "pass"}, {Weight: 0.5, Status: SemJudged, Verdict: "pass"}}, ptr(100)},
		{"weighted", []SemDim{{Weight: 0.75, Status: SemJudged, Verdict: "warn"}, {Weight: 0.25, Status: SemJudged, Verdict: "fail"}}, ptr(38)},
		{"unstable does not accuse", []SemDim{{Weight: 0.5, Status: SemUnstable, Verdict: "fail"}, {Weight: 0.5, Status: SemJudged, Verdict: "pass"}}, ptr(100)},
		{"preempted, skipped and errored dimensions are left out", []SemDim{
			{Weight: 0.5, Status: SemPreempted, Verdict: "fail"}, {Weight: 0.25, Status: SemSkipped}, {Weight: 0.1, Status: SemError}, {Weight: 0.15, Status: SemJudged, Verdict: "warn"},
		}, ptr(50)},
		{"nothing judged", []SemDim{{Weight: 1, Status: SemSkipped}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semanticScore(tt.dims)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *tt.want, *got)
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestSemanticFindingsAreAdvisoryCappedAndFingerprinted(t *testing.T) {
	// Arrange
	rb := builtin(t)
	res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Helps with deployments")}})
	sj := &scriptedJudge{verdict: func(_, dim string, _ int) string { return pick(dim == "trigger-quality", VerdictFail) }}
	client, _ := newClient(t, sj, llm.Config{})
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3}})

	// Act
	var judged []Finding
	for _, f := range res.Findings(rb) {
		if f.Origin == OriginLLMJudge {
			judged = append(judged, f)
		}
	}
	again := res.Findings(rb)

	// Assert
	require.Len(t, judged, 1)
	f := judged[0]
	assert.Equal(t, "AR9G1", f.Code)
	assert.Equal(t, "warning", f.Severity, "the ceiling, never error")
	assert.Equal(t, "Helps with deployments", f.Quote)
	assert.Equal(t, "improve trigger-quality", f.Suggestion)
	assert.Equal(t, fingerprint("AR9G1", "skill:a", "trigger-quality", "Helps with deployments"), f.Fingerprint)
	assert.Equal(t, again[0].Fingerprint, res.Findings(rb)[0].Fingerprint)
}

func TestAnUnstableOrTruncatedVerdictIsReportedAtInfo(t *testing.T) {
	// Arrange
	it := &ItemResult{Item: skill("a", "x"), Status: StatusScored, Semantic: &SemanticResult{Dimensions: []SemDim{
		{ID: "trigger-quality", Code: "AR9G1", Status: SemUnstable, Verdict: VerdictFail, severity: "warning", Votes: []string{"fail", "warn", "pass"}, Agreement: 0.333},
		{ID: "overlap", Code: "AR9G2", Status: SemJudged, Verdict: VerdictWarn, severity: "warning", Capped: true},
	}}}

	// Act
	got := semanticFindings(it)

	// Assert
	require.Len(t, got, 2)
	assert.Equal(t, "info", got[0].Severity)
	assert.Contains(t, got[0].Message, "unstable")
	assert.Equal(t, "info", got[1].Severity)
}

func TestContextCancelStopsTheRun(t *testing.T) {
	// Arrange
	rb, res := twoSkills(t)
	sj := &scriptedJudge{}
	client, _ := newClient(t, sj, llm.Config{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Act
	_, err := RunSemantic(ctx, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})

	// Assert: nothing was judged, and nothing was reported as a pass
	if err != nil {
		assert.True(t, errors.Is(err, context.Canceled))
	}
	for _, r := range res.Items {
		assert.Nil(t, r.Semantic)
	}
}

func TestChangedItemsMarksADirectoryItemWhenAReferenceChanged(t *testing.T) {
	// Arrange
	a, b := skill("a", "x"), skill("b", "y")
	cmd := skill("c", "z")
	cmd.Path = ".ai-rulez/commands/c.md"

	// Act
	got := ChangedItems([]Item{a, b, cmd}, []string{"sub/.ai-rulez/skills/a/references/guide.md", "sub/.ai-rulez/commands/c.md", "README.md"}, "sub")

	// Assert
	assert.Equal(t, map[string]bool{"skill:a": true, "skill:c": true}, got)
}
