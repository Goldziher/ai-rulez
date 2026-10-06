package skillsearch

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hybridEnv(t *testing.T) *EvalEnv {
	t.Helper()
	items := refundCatalog()
	emb := refundEmbedder()
	res, err := Build(t.Context(), items, &BuildOptions{Embedder: emb})
	require.NoError(t, err)
	return &EvalEnv{Items: items, Cfg: Config{Mode: ModeHybrid}, Index: res.Index, Embedder: emb}
}

func TestEval_HybridBeatsLexicalOnParaphrasesAndReportsPairedDiff(t *testing.T) {
	t.Parallel()
	// Arrange: paraphrases lexical cannot hit, plus one lexical-friendly case
	env := hybridEnv(t)
	f := &CaseFile{Version: 1, Cases: []Case{
		{ID: "p1", Query: "money back", Expect: rel("issue-refund")},
		{ID: "p2", Query: "returned purchase money", Expect: rel("issue-refund")},
		{ID: "p3", Query: "roll out to the cluster", Expect: rel("deploy-staging")},
		{ID: "p4", Query: "chargeback from the bank", Expect: rel("dispute-charge")},
		{ID: "lex", Query: "git branching", Expect: rel("git-workflow")},
	}}

	// Act
	res := mustEval(t, env, f, 3, ModeHybrid, ModeLexical)

	// Assert
	assert.Equal(t, ModeHybrid, res.Mode)
	require.Contains(t, res.Modes, ModeLexical)
	assert.Greater(t, res.Modes[ModeHybrid].MRR, res.Modes[ModeLexical].MRR)
	assert.InDelta(t, 1.0, res.Modes[ModeHybrid].HitAt, 1e-9)
	require.Contains(t, res.Paired, ModeHybrid)
	d := res.Paired[ModeHybrid]["mrr"]
	assert.Greater(t, d.Diff, 0.0)
	assert.LessOrEqual(t, d.Low, d.Diff)
	assert.GreaterOrEqual(t, d.High, d.Diff)
	assert.Len(t, res.ByMode, 2)
	require.NotNil(t, res.Index)
	assert.Equal(t, 4, res.Index.Rows)
	assert.Zero(t, res.ByMode[ModeHybrid].DegradedCases)
}

func TestEval_CountsDegradedCases(t *testing.T) {
	t.Parallel()
	env := hybridEnv(t)
	env.Embedder.(*conceptEmbedder).err = errProvider
	f := &CaseFile{Version: 1, Cases: []Case{{ID: "a", Query: "money back", Expect: rel("issue-refund")}, {ID: "n", Query: "x", Expect: nil}}}

	res := mustEval(t, env, f, 3, ModeHybrid)

	assert.Equal(t, 2, res.ByMode[ModeHybrid].DegradedCases)
	assert.Equal(t, DegradedProvider, res.Cases[0].Degraded)
	assert.Equal(t, DegradedProvider, res.Negatives[0].Degraded)
}

func TestEval_GradedRelevanceNDCG(t *testing.T) {
	t.Parallel()
	// Arrange: the ideal order is staging (grade 3) then prod (grade 1); lexical ranks prod first for "deploy"
	env := evalEnv()
	graded := Case{ID: "g", Query: "deploy", Expect: []Relevant{{ID: "deploy-staging", Grade: 3, Graded: true}, {ID: "deploy-prod", Grade: 1, Graded: true}}}
	f := &CaseFile{Version: 1, Cases: []Case{graded}}

	// Act
	res := mustEval(t, env, f, 5)

	// Assert: dcg = 3/log2(2) + 1/log2(3) when staging is first; the actual order decides
	m := res.Modes[ModeLexical]
	require.NotNil(t, m.NDCG)
	got := res.Cases[0].Got
	var dcg float64
	for i, id := range got {
		switch id {
		case "deploy-staging":
			dcg += 3 / math.Log2(float64(i)+2)
		case "deploy-prod":
			dcg += 1 / math.Log2(float64(i)+2)
		}
	}
	ideal := 3/math.Log2(2) + 1/math.Log2(3)
	assert.InDelta(t, dcg/ideal, *m.NDCG, 1e-4)
	assert.Contains(t, res.CI95, "ndcg_at_k")
	assert.LessOrEqual(t, *m.NDCG, 1.0)
}

func TestEval_NoNDCGWithoutGrades(t *testing.T) {
	t.Parallel()
	res := mustEval(t, evalEnv(), &CaseFile{Version: 1, Cases: []Case{{ID: "a", Query: "deploy", Expect: rel("deploy-prod")}}}, 3)
	assert.Nil(t, res.Modes[ModeLexical].NDCG)
	assert.NotContains(t, res.CI95, "ndcg_at_k")
}

func TestNDCG_PerfectAndReversed(t *testing.T) {
	t.Parallel()
	expect := []Relevant{{ID: "a", Grade: 2}, {ID: "b", Grade: 1}}
	perfect := 2/math.Log2(2) + 1/math.Log2(3)
	reversed := 1/math.Log2(2) + 2/math.Log2(3)
	assert.InDelta(t, 1.0, ndcg(perfect, expect, 5), 1e-9)
	assert.InDelta(t, reversed/perfect, ndcg(reversed, expect, 5), 1e-9)
	assert.Zero(t, ndcg(0, nil, 5))
}

func TestEval_AvoidMeasuresNearMissConfusion(t *testing.T) {
	t.Parallel()
	// Arrange: "deploy to production" is a near miss of deploy-staging; lexical ranks deploy-prod first, which is fine,
	// but "deploy to staging please" would rank deploy-staging first, which violates the avoid list
	env := evalEnv()
	f := &CaseFile{Version: 1, Cases: []Case{
		{ID: "ok", Query: "deploy to production", Expect: nil, Avoid: []string{"deploy-staging"}},
		{ID: "bad", Query: "deploy to staging", Expect: nil, Avoid: []string{"deploy-staging"}},
		{ID: "plain", Query: "weather", Expect: nil},
	}}

	res := mustEval(t, env, f, 3)

	byID := map[string]NegativeResult{}
	for _, n := range res.Negatives {
		byID[n.ID] = n
	}
	assert.False(t, byID["ok"].Violated)
	assert.True(t, byID["bad"].Violated)
	require.NotNil(t, res.Modes[ModeLexical].AvoidTop)
	assert.InDelta(t, 0.5, *res.Modes[ModeLexical].AvoidTop, 1e-9, "one of the two avoid cases ranked the avoided skill first")
}

func TestEval_PerCaseRole(t *testing.T) {
	t.Parallel()
	// Arrange: the "billing" role serves refund and dispute only
	env := evalEnv()
	served := map[string]bool{"refund-policy": true}
	env.Scope = func(role string) (func(int) bool, error) {
		if role != "billing" {
			return nil, fmt.Errorf("unknown role %q", role)
		}
		return func(i int) bool { return served[env.Items[i].ID] }, nil
	}
	f := &CaseFile{Version: 1, Cases: []Case{{ID: "r", Query: "deploy refund", Role: "billing", Expect: rel("refund-policy")}}}

	// Act
	res := mustEval(t, env, f, 3)

	// Assert: deploy-* skills are not in the role's catalog
	assert.Equal(t, []string{"refund-policy"}, res.Cases[0].Got)
	assert.Equal(t, "billing", res.Cases[0].Role)

	t.Run("unknown role", func(t *testing.T) {
		bad := &CaseFile{Version: 1, Cases: []Case{{ID: "r", Query: "q", Role: "nobody", Expect: rel("refund-policy")}}}
		_, err := Eval(t.Context(), env, bad, 3, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR9D2")
		assert.Contains(t, err.Error(), `unknown role "nobody"`)
	})
	t.Run("expected skill outside the role", func(t *testing.T) {
		bad := &CaseFile{Version: 1, Cases: []Case{{ID: "r", Query: "q", Role: "billing", Expect: rel("deploy-prod")}}}
		_, err := Eval(t.Context(), env, bad, 3, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `does not serve`)
	})
	t.Run("no roles configured", func(t *testing.T) {
		env2 := evalEnv()
		_, err := Eval(t.Context(), env2, f, 3, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no roles are configured")
	})
}

func TestParseCases_GradedAvoidRole(t *testing.T) {
	t.Parallel()
	doc := `version: 1
cases:
  - id: g
    query: set up staging deploy
    role: platform
    expect:
      - {id: deploy-staging, grade: 2}
      - ci-pipeline
    avoid: [deploy-prod]
`
	f, err := ParseCases([]byte(doc))
	require.NoError(t, err)
	c := f.Cases[0]
	assert.Equal(t, "platform", c.Role)
	assert.Equal(t, []Relevant{{ID: "deploy-staging", Grade: 2, Graded: true}, {ID: "ci-pipeline", Grade: 1}}, c.Expect)
	assert.Equal(t, []string{"deploy-prod"}, c.Avoid)
	assert.Equal(t, []string{"deploy-staging", "ci-pipeline"}, c.ExpectIDs())

	err = f.CheckSkills([]string{"deploy-staging", "ci-pipeline"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `avoids unknown skill "deploy-prod"`)
}

func TestGate_NDCGMinimum(t *testing.T) {
	t.Parallel()
	v := 0.5
	r := &Result{Mode: ModeHybrid, Modes: map[string]Metrics{ModeHybrid: {Top1: 1, NDCG: &v}}}
	assert.Len(t, r.Gate(Minimums{"ndcg": 0.6}, -1), 1)
	assert.Empty(t, r.Gate(Minimums{"ndcg": 0.4, "top1": 0.9}, -1))
	none := &Result{Mode: ModeLexical, Modes: map[string]Metrics{ModeLexical: {}}}
	failed := none.Gate(Minimums{"ndcg": 0.1}, -1)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0], "no case is graded")
	mins, err := ParseMinimums("ndcg=0.7")
	require.NoError(t, err)
	assert.Equal(t, Minimums{"ndcg": 0.7}, mins)
}
