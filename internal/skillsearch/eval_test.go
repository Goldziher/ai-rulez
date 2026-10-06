package skillsearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rel(ids ...string) []Relevant {
	out := make([]Relevant, len(ids))
	for i, id := range ids {
		out[i] = Relevant{ID: id, Grade: 1}
	}
	return out
}

func evalEnv() *EvalEnv {
	docs := []Doc{
		{Name: "deploy-staging", Description: "Deploy a service to the staging cluster", Triggers: []string{"deploy to staging"}},
		{Name: "deploy-prod", Description: "Deploy a service to production with approvals"},
		{Name: "refund-policy", Description: "Process customer refund requests", Triggers: []string{"customer wants money back"}},
		{Name: "git-workflow", Description: "Branching and pull request conventions"},
	}
	items := make([]Item, len(docs))
	for i := range docs {
		items[i] = Item{ID: docs[i].Name, Doc: docs[i]}
	}
	return &EvalEnv{Items: items}
}

func mustEval(t *testing.T, env *EvalEnv, f *CaseFile, k int, modes ...string) *Result {
	t.Helper()
	res, err := Eval(context.Background(), env, f, k, modes)
	require.NoError(t, err)
	return res
}

func TestEval_Metrics(t *testing.T) {
	t.Parallel()
	// Arrange
	env := evalEnv()
	f := &CaseFile{Version: 1, Cases: []Case{
		{ID: "refund", Query: "customer wants money back", Expect: rel("refund-policy"), Tags: []string{"paraphrase"}},
		{ID: "staging", Query: "deploy to staging", Expect: rel("deploy-staging")},
		{ID: "second", Query: "deploy", Expect: rel("deploy-prod")},
		{ID: "miss", Query: "zebra", Expect: rel("git-workflow")},
		{ID: "neg", Query: "weather tomorrow", Expect: nil, Tags: []string{"negative"}},
	}}

	// Act
	res := mustEval(t, env, f, 2)

	// Assert
	assert.Equal(t, 4, res.N)
	assert.Equal(t, 1, res.NNegative)
	byID := map[string]CaseResult{}
	for _, c := range res.Cases {
		byID[c.ID] = c
	}
	require.NotNil(t, byID["refund"].Rank)
	assert.Equal(t, 1, *byID["refund"].Rank)
	assert.Nil(t, byID["miss"].Rank)
	assert.False(t, byID["miss"].Hit)
	m := res.Modes[ModeLexical]
	assert.Equal(t, 4, m.N)
	assert.InDelta(t, 0.5, m.Top1, 1e-9, "refund and staging are first")
	assert.Less(t, m.MRR, 1.0)
	assert.Greater(t, m.MRR, m.Top1/2)
	assert.InDelta(t, 0.75, m.HitAt, 1e-9, "everything but the zebra query is hit within k=2")
	assert.Equal(t, 1, len(res.Misses))
	assert.Equal(t, "miss", res.Misses[0].ID)
	assert.Equal(t, 1, res.Tags["paraphrase"].N)
	assert.Equal(t, "", res.Negatives[0].Top)
	assert.Contains(t, res.CI95, "mrr")
	assert.LessOrEqual(t, res.CI95["top1"].Low, m.Top1)
	assert.GreaterOrEqual(t, res.CI95["top1"].High, m.Top1)
}

func TestEval_MultiRelevantRecall(t *testing.T) {
	t.Parallel()
	env := evalEnv()
	f := &CaseFile{Version: 1, Cases: []Case{
		{ID: "both", Query: "deploy service", Expect: rel("deploy-staging", "deploy-prod")},
	}}
	res := mustEval(t, env, f, 1)
	assert.InDelta(t, 0.5, res.Modes[ModeLexical].RecallAt, 1e-9, "one of two relevant skills is in the top 1")
	assert.InDelta(t, 1.0, res.Modes[ModeLexical].HitAt, 1e-9)
}

func TestEval_Deterministic(t *testing.T) {
	t.Parallel()
	env := evalEnv()
	f := &CaseFile{Version: 1, Cases: []Case{{ID: "a", Query: "deploy", Expect: rel("deploy-prod")}, {ID: "b", Query: "refund", Expect: rel("refund-policy")}}}
	assert.Equal(t, mustEval(t, env, f, 3), mustEval(t, env, f, 3))
}

func TestCompareBaselineAndGate(t *testing.T) {
	t.Parallel()
	mk := func(hits map[string]bool) *Result {
		r := &Result{SchemaVersion: ResultSchemaVersion, Modes: map[string]Metrics{ModeLexical: {Top1: 0.5, MRR: 0.6}}}
		for id, hit := range hits {
			r.Cases = append(r.Cases, CaseResult{ID: id, Hit: hit})
		}
		return r
	}
	base := mk(map[string]bool{"a": true, "b": true, "c": false, "gone": true})
	cur := mk(map[string]bool{"a": true, "b": false, "c": true, "new": false})

	require.NoError(t, cur.CompareBaseline(base))

	assert.Equal(t, []string{"b"}, cur.Flips.Regressed)
	assert.Equal(t, []string{"c"}, cur.Flips.Fixed)
	tests := []struct {
		name     string
		mins     Minimums
		maxFlips int
		failed   int
	}{
		{"flip gate passes at the limit", nil, 1, 0},
		{"flip gate fails", nil, 0, 1},
		{"flip gate off", nil, -1, 0},
		{"minimum met", Minimums{"top1": 0.5}, -1, 0},
		{"minimum missed", Minimums{"top1": 0.6, "mrr": 0.7}, -1, 2},
		{"both fail", Minimums{"top1": 0.9}, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cur.Gate(tt.mins, tt.maxFlips)
			assert.Len(t, got, tt.failed, "%v", got)
			for _, msg := range got {
				assert.Contains(t, msg, CodeEvalRegression)
			}
		})
	}
}

func TestParseMinimums(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    Minimums
		wantErr bool
	}{
		{"", Minimums{}, false},
		{"top1=0.6,mrr=0.7", Minimums{"top1": 0.6, "mrr": 0.7}, false},
		{"recall@k=0.8, hit=1", Minimums{"recall": 0.8, "hit": 1}, false},
		{"top1", nil, true},
		{"nope=0.5", nil, true},
		{"top1=1.5", nil, true},
		{"top1=abc", nil, true},
	}
	for _, tt := range tests {
		got, err := ParseMinimums(tt.in)
		if tt.wantErr {
			assert.Error(t, err, tt.in)
			continue
		}
		require.NoError(t, err, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}
}

func TestParseCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{"valid", "version: 1\nk: 3\ncases:\n  - {id: a, query: deploy, expect: [x], tags: [t]}\n  - {id: n, query: weather, expect: []}\n", ""},
		{"missing version", "cases:\n  - {id: a, query: q, expect: [x]}\n", "version must be 1"},
		{"unknown field", "version: 1\ncases:\n  - {id: a, query: q, expect: [x], nope: r}\n", "field nope not found"},
		{"bad grade", "version: 1\ncases:\n  - {id: a, query: q, expect: [{id: x, grade: 0}]}\n", "grade of"},
		{"unknown expect key", "version: 1\ncases:\n  - {id: a, query: q, expect: [{id: x, weight: 2}]}\n", "weight"},
		{"expected and avoided", "version: 1\ncases:\n  - {id: a, query: q, expect: [x], avoid: [x]}\n", "both expected and avoided"},
		{"duplicate id", "version: 1\ncases:\n  - {id: a, query: q, expect: [x]}\n  - {id: a, query: r, expect: [x]}\n", "duplicate id"},
		{"missing query", "version: 1\ncases:\n  - {id: a, expect: [x]}\n", "query is required"},
		{"missing id", "version: 1\ncases:\n  - {query: q, expect: [x]}\n", "id is required"},
		{"empty", "version: 1\ncases: []\n", "cases is empty"},
		{"k out of range", "version: 1\nk: 500\ncases:\n  - {id: a, query: q, expect: [x]}\n", "k must be"},
		{"duplicate expect", "version: 1\ncases:\n  - {id: a, query: q, expect: [x, x]}\n", "listed twice"},
		{"not yaml", "version: [", "AR9D2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseCases([]byte(tt.doc))
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Len(t, f.Cases, 2)
				assert.Equal(t, 3, f.EffectiveK(0))
				assert.Equal(t, 7, f.EffectiveK(7))
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), CodeCasesInvalid)
		})
	}
}

func TestCaseFile_CheckSkillsAndK(t *testing.T) {
	t.Parallel()
	f := &CaseFile{Version: 1, Cases: []Case{{ID: "a", Query: "q", Expect: rel("ok", "ghost")}}}
	err := f.CheckSkills([]string{"ok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown skill "ghost"`)
	assert.NoError(t, f.CheckSkills([]string{"ok", "ghost"}))
	assert.Equal(t, 5, f.EffectiveK(0))
}

func TestLoadCases_RejectsNonRegularAndMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadCases(dir)
	assert.Error(t, err)
	_, err = LoadCases(filepath.Join(dir, "missing.yaml"))
	assert.Error(t, err)
	p := filepath.Join(dir, "c.yaml")
	require.NoError(t, os.WriteFile(p, []byte("version: 1\ncases:\n  - {id: a, query: q, expect: [x]}\n"), 0o600))
	f, err := LoadCases(p)
	require.NoError(t, err)
	assert.Len(t, f.Cases, 1)
}

func TestLoadBaseline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := filepath.Join(dir, "b.json")
	require.NoError(t, os.WriteFile(good, []byte(`{"schema_version":1,"cases":[{"id":"a","hit":true}]}`), 0o600))
	b, err := LoadBaseline(good)
	require.NoError(t, err)
	assert.Len(t, b.Cases, 1)
	bad := filepath.Join(dir, "v.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"schema_version":9}`), 0o600))
	_, err = LoadBaseline(bad)
	assert.Error(t, err)
	_, err = LoadBaseline(filepath.Join(dir, "none.json"))
	assert.Error(t, err)
}

func TestCompareBaselineRefusesADifferentK(t *testing.T) {
	t.Parallel()
	base := &Result{SchemaVersion: ResultSchemaVersion, K: 10, Cases: []CaseResult{{ID: "a", Hit: true}}}
	cur := &Result{SchemaVersion: ResultSchemaVersion, K: 3, Cases: []CaseResult{{ID: "a", Hit: false}}}

	err := cur.CompareBaseline(base)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "k=10")
	assert.Contains(t, err.Error(), "k=3")
	assert.Nil(t, cur.Flips, "no flips are reported from incomparable runs")
}

func TestBootstrap_PinnedIntervals(t *testing.T) {
	t.Parallel()
	cases := []CaseResult{
		{ID: "a", top1: 1, recall: 1, Rank: ptr(1)},
		{ID: "b", top1: 0, recall: 1, Rank: ptr(3)},
		{ID: "c", top1: 0, recall: 0},
		{ID: "d", top1: 1, recall: 1, Rank: ptr(1)},
		{ID: "e", top1: 0, recall: 0.5, Rank: ptr(2)},
	}

	got := bootstrap(cases)

	assert.Equal(t, map[string]Interval{
		"mrr":         {Low: 0.2, High: 0.9},
		"recall_at_k": {Low: 0.3, High: 1},
		"top1":        {Low: 0, High: 0.8},
	}, got, "the resampling seed and count are part of the report format: a change here moves every published interval")
	assert.Equal(t, got, bootstrap(cases), "the same cases give the same interval")
}

func ptr(n int) *int { return &n }
