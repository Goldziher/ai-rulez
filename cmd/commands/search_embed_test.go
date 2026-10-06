package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// embedServer is a fake OpenAI-compatible /embeddings endpoint whose vectors
// have one dimension per concept, so synonyms land close together.
type embedServer struct {
	*httptest.Server
	mu    sync.Mutex
	texts []string
	fail  bool
}

var embedConcepts = [][]string{
	{"refund", "refunds", "reimburse", "chargebacks", "customer", "money", "back"},
	{"deploy", "staging", "rollout", "cluster", "service"},
	{"git", "branching", "pull", "request", "conventions"},
}

func newEmbedServer(t *testing.T) *embedServer {
	t.Helper()
	s := &embedServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.fail {
			http.Error(w, `{"error":{"message":"boom"}}`, http.StatusBadRequest)
			return
		}
		s.texts = append(s.texts, req.Input...)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		tokens := 0
		for i, text := range req.Input {
			v := make([]float32, len(embedConcepts)+1)
			for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
				hit := false
				for c, words := range embedConcepts {
					for _, x := range words {
						if x == word {
							v[c]++
							hit = true
						}
					}
				}
				if !hit {
					v[len(embedConcepts)] += 0.05
				}
			}
			data = append(data, item{i, v})
			tokens += len(strings.Fields(text))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "model": "concepts", "usage": map[string]int{"prompt_tokens": tokens, "total_tokens": tokens}})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *embedServer) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

func (s *embedServer) setFail(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = v
}

// useEmbeddings writes the user config that points [llm] at the fake server.
func useEmbeddings(t *testing.T, srv *embedServer, allowNetwork bool) {
	t.Helper()
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	dir := filepath.Join(cfgHome, "ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	net := "false"
	if allowNetwork {
		net = "true"
	}
	body := "[llm]\nallow_network = " + net + "\nbase_url = \"" + srv.URL + "\"\napi_key_env = \"SEARCH_TEST_KEY\"\nembedding_model = \"concepts\"\nmax_retries = -1\n\n[search]\nmode = \"hybrid\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600))
	t.Setenv("SEARCH_TEST_KEY", "k")
}

func execSub(t *testing.T, fn func() int) (code int) {
	t.Helper()
	return fn()
}

func runIndex(t *testing.T, flags ...string) (code int, out, errOut string) {
	t.Helper()
	for i := 0; i+1 < len(flags); i += 2 {
		require.NoError(t, SearchCmd.Flags().Set(flags[i], flags[i+1]))
	}
	var o, e bytes.Buffer
	code = runSearchIndex(t.Context(), &o, &e)
	return code, o.String(), e.String()
}

func runStatus(t *testing.T) (code int, out string) {
	t.Helper()
	var o bytes.Buffer
	code = runSearchStatus(t.Context(), &o)
	return code, o.String()
}

func TestSearchIndex_DryRunSendsNothing(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)

	code, out, _ := runIndex(t, "dry-run", "true")

	assert.Equal(t, 0, code)
	assert.Contains(t, out, "items      3   cached 0   to embed 3")
	assert.Contains(t, out, "egress     3 texts")
	assert.Contains(t, out, "127.0.0.1")
	assert.Empty(t, srv.sent(), "a dry run makes no call")
	_, statErr := os.Stat(filepath.Join(".ai-rulez", "local", "search", "manifest.json"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSearchIndex_IncrementalLifecycle(t *testing.T) {
	root := searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)

	// First build embeds every skill
	code, out, errOut := runIndex(t)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "embedded 3, reused 0")
	assert.Len(t, srv.sent(), 3)
	code, status := runStatus(t)
	require.Equal(t, 0, code)
	assert.Contains(t, status, "state      fresh")

	// A second run sends nothing and writes nothing
	code, out, _ = runIndex(t)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "index is current, nothing written")
	assert.Len(t, srv.sent(), 3)

	// One edit costs one text, and status reports it before the rebuild
	skillPath := filepath.Join(root, ".ai-rulez", "skills", "deploy-staging", "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("---\nname: deploy-staging\ndescription: Deploy a service to the production cluster\n---\nbody\n"), 0o600))
	_, status = runStatus(t)
	assert.Contains(t, status, "state      stale")
	assert.Contains(t, status, "changed: deploy-staging")
	code, out, _ = runIndex(t)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "embedded 1, reused 2")
	assert.Len(t, srv.sent(), 4)
}

func TestSearchIndex_NetworkDisabledRefusesBeforeSending(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, false)

	code, _, _ := runIndex(t)

	assert.Equal(t, 1, code)
	assert.Empty(t, srv.sent())
}

func TestSearchIndex_WithholdsASecretAndNeverSendsIt(t *testing.T) {
	root := searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	// With AR001 downgraded the catalog still serves a skill that holds a key; the index must not send it.
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	cfg, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(cfg, []byte("\n[lint.severity]\nAR001 = \"warning\"\n")...), 0o600))
	skillPath := filepath.Join(root, ".ai-rulez", "skills", "git-workflow", "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("---\nname: git-workflow\ndescription: Push with token AKIAABCDEFGHIJKLMNOP to the remote\n---\nbody\n"), 0o600))

	code, out, errOut := runIndex(t)

	require.Equal(t, 0, code, errOut)
	assert.Contains(t, errOut, "AR9D3")
	assert.Contains(t, errOut, "git-workflow")
	assert.Contains(t, out, "withheld 1")
	for _, text := range srv.sent() {
		assert.NotContains(t, text, "AKIA")
	}
	assert.Len(t, srv.sent(), 2)
}

func TestSearchIndex_ProviderStopWritesNothingUsableAndExitsOne(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	srv.setFail(true)

	code, _, errOut := runIndex(t)

	assert.Equal(t, 1, code, errOut)
}

func TestSearchQuery_HybridFindsAParaphraseAndDegradesWhenTheProviderIsDown(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	code, _, errOut := runIndex(t)
	require.Equal(t, 0, code, errOut)
	resetSearch(t)
	setSearchFlag(t, "format", "json")

	// Act: "reimburse me" shares no stem with any skill's text
	code, out, _ := execSearch(t, "reimburse me")

	// Assert
	require.Equal(t, 0, code)
	var doc searchDoc
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.Equal(t, "hybrid", doc.Ranking)
	assert.Nil(t, doc.Degraded)
	require.NotEmpty(t, doc.Results)
	assert.Equal(t, "refund-policy", doc.Results[0].Name)
	require.NotNil(t, doc.Results[0].VectorRank)
	assert.Equal(t, 1, *doc.Results[0].VectorRank)
	validateAgainst(t, "../../schema/search.v1.schema.json", []byte(out))

	// The provider goes down: still answered, lexically, with the reason
	srv.setFail(true)
	code, out, errOut = execSearch(t, "deploy to staging")
	require.Equal(t, 0, code)
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.Equal(t, "lexical", doc.Ranking)
	require.NotNil(t, doc.Degraded)
	assert.Equal(t, "provider_unavailable", *doc.Degraded)
	assert.Equal(t, "deploy-staging", doc.Results[0].Name)
	assert.Contains(t, errOut, "ranking lexically")
	validateAgainst(t, "../../schema/search.v1.schema.json", []byte(out))
}

func TestSearchQuery_NoIndexAndNetworkDisabledAreDegraded(t *testing.T) {
	searchProject(t)
	srv := newEmbedServer(t)
	tests := []struct {
		name    string
		network bool
		want    string
	}{
		{"no index", true, "no_index"},
		{"network disabled", false, "no_index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSearch(t)
			useEmbeddings(t, srv, tt.network)
			setSearchFlag(t, "format", "json")

			code, out, _ := execSearch(t, "deploy to staging")

			require.Equal(t, 0, code)
			var doc searchDoc
			require.NoError(t, json.Unmarshal([]byte(out), &doc))
			require.NotNil(t, doc.Degraded)
			assert.Equal(t, tt.want, *doc.Degraded)
			assert.Equal(t, "deploy-staging", doc.Results[0].Name)
		})
	}
}

func TestSearchQuery_ExplainShowsListRanks(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	code, _, errOut := runIndex(t)
	require.Equal(t, 0, code, errOut)
	resetSearch(t)
	setSearchFlag(t, "explain", "true")

	code, out, _ := execSearch(t, "rollout to staging")

	require.Equal(t, 0, code)
	assert.Contains(t, out, "LEX#")
	assert.Contains(t, out, "VEC#")
	assert.Contains(t, out, "ranking: hybrid")
}

func TestSearchEval_LexicalVersusHybridWithPairedIntervals(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	code, _, errOut := runIndex(t)
	require.Equal(t, 0, code, errOut)
	resetSearch(t)
	cases := writeCases(t, `version: 1
k: 3
cases:
  - {id: p1, query: "reimburse me", expect: [refund-policy]}
  - {id: p2, query: "chargebacks", expect: [refund-policy]}
  - {id: p3, query: "cluster rollout", expect: [deploy-staging]}
  - {id: lex, query: "pull request conventions", expect: [git-workflow]}
  - {id: neg, query: "weather tomorrow", expect: []}
`)
	setSearchFlag(t, "eval", cases)
	setSearchFlag(t, "mode", "lexical,hybrid")
	setSearchFlag(t, "format", "json")

	code, out, errOut := execSearch(t)

	require.Equal(t, 0, code, errOut)
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, "lexical", res["mode"])
	modes := res["modes"].(map[string]any)
	assert.Contains(t, modes, "hybrid")
	assert.Greater(t, modes["hybrid"].(map[string]any)["mrr"].(float64), modes["lexical"].(map[string]any)["mrr"].(float64)-1e-9)
	assert.Contains(t, res, "paired_vs_lexical")
	assert.Contains(t, res, "index")
	validateAgainst(t, "../../schema/search-eval.v1.schema.json", []byte(out))
}

func TestSearchEval_HybridWithoutAnIndexCannotRun(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	setSearchFlag(t, "eval", writeCases(t, searchGoodCases))
	setSearchFlag(t, "mode", "hybrid")

	code, _, _ := execSearch(t)

	assert.Equal(t, 1, code)
}

func TestSearchEval_DegradedEmbeddingsAreNeverAPass(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	useEmbeddings(t, srv, true)
	code, _, errOut := runIndex(t)
	require.Equal(t, 0, code, errOut)
	resetSearch(t)
	srv.setFail(true)
	setSearchFlag(t, "eval", writeCases(t, searchGoodCases))
	setSearchFlag(t, "mode", "hybrid")

	code, _, errOut = execSearch(t)

	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "fell back to lexical")
}

func TestSearchEval_FromEvalsDerivesCases(t *testing.T) {
	root := searchProject(t)
	resetSearch(t)
	evalFile := filepath.Join(root, ".ai-rulez", "skills", "refund-policy", "evals", "refund.eval.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(evalFile), 0o750))
	require.NoError(t, os.WriteFile(evalFile, []byte(`cases:
  - id: money-back
    prompt: the customer wants money back
    expect_trigger: true
    near_miss:
      - how do I deploy to staging
`), 0o600))
	setSearchFlag(t, "from-evals", "true")
	setSearchFlag(t, "format", "json")

	code, out, errOut := execSearch(t)

	require.Equal(t, 0, code, errOut)
	assert.Contains(t, errOut, "from-evals: 2 cases derived")
	var res struct {
		N         int `json:"n"`
		NNegative int `json:"n_negative"`
		Cases     []struct {
			ID string `json:"id"`
		} `json:"cases"`
		Negatives []struct {
			ID       string `json:"id"`
			Violated bool   `json:"avoid_violated"`
		} `json:"negatives"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, 1, res.N)
	assert.Equal(t, "refund-policy/money-back", res.Cases[0].ID)
	require.Len(t, res.Negatives, 1)
	assert.Equal(t, "refund-policy/money-back.near-miss-1", res.Negatives[0].ID)
	assert.False(t, res.Negatives[0].Violated, "the deploy near miss does not rank refund-policy first")
}

func TestSearchMine_LabelsQueriesFromTheLog(t *testing.T) {
	root := searchProject(t)
	resetSearch(t)
	logPath := filepath.Join(root, ".ai-rulez", "local", "search-queries.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o750))
	lines := `{"v":1,"ts":"2026-10-06T10:00:00Z","event":"query","session":"s1","query":"reimburse the customer","ranking":"lexical"}
{"v":1,"ts":"2026-10-06T10:00:01Z","event":"loaded","session":"s1","skill":"refund-policy"}
{"v":1,"ts":"2026-10-06T10:00:02Z","event":"query","session":"s2","query":"nothing loaded"}
`
	require.NoError(t, os.WriteFile(logPath, []byte(lines), 0o600))
	out := filepath.Join(t.TempDir(), "mined.yaml")
	require.NoError(t, SearchCmd.Flags().Set("out", out))
	require.NoError(t, SearchCmd.Flags().Set("purge", "true"))
	var o, e bytes.Buffer

	code := runSearchMine(t.Context(), &o, &e)

	require.Equal(t, 0, code, e.String())
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "reimburse the customer")
	assert.Contains(t, string(raw), "refund-policy")
	assert.Contains(t, string(raw), "mined")
	assert.Contains(t, e.String(), "mined 1 cases from 2 distinct queries")
	_, statErr := os.Stat(logPath)
	assert.True(t, os.IsNotExist(statErr), "--purge deletes the log")
	_, evalErr := os.Stat(out)
	require.NoError(t, evalErr)
	// the mined file is a valid cases file
	resetSearch(t)
	setSearchFlag(t, "eval", out)
	code, _, errOut := execSearch(t)
	assert.Equal(t, 0, code, errOut)
}

func TestSearchIndex_DryRunWithoutAModelStillShowsThePlan(t *testing.T) {
	searchProject(t)
	resetSearch(t)

	code, out, _ := runIndex(t, "dry-run", "true")

	assert.Equal(t, 0, code)
	assert.Contains(t, out, "model      (none configured)")
	assert.Contains(t, out, "to embed 3")
}

func TestSearch_SubcommandWordsDispatchOnlyAlone(t *testing.T) {
	searchProject(t)
	resetSearch(t)

	// "status" alone is the subcommand
	code, out, _ := execSearch(t, "status")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "state      none")

	// with another word it is a query
	code, out, _ = execSearch(t, "status", "report")
	assert.Equal(t, 0, code)
	assert.NotContains(t, out, "state      none")

	// flags of another mode are refused rather than ignored
	setSearchFlag(t, "dry-run", "true")
	code, _, _ = execSearch(t, "status")
	assert.Equal(t, 1, code)
	code, _, _ = execSearch(t, "a", "query")
	assert.Equal(t, 1, code, "--dry-run requires 'search index'")
}
