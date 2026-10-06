package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func searchProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	skill := func(name, desc, extra string) string {
		return "---\nname: " + name + "\ndescription: " + desc + "\n" + extra + "---\nbody\n"
	}
	files := map[string]string{
		".ai-rulez/config.toml":                    "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n",
		".ai-rulez/skills/refund-policy/SKILL.md":  skill("refund-policy", "Process customer refund requests and chargebacks", "triggers:\n  - customer wants money back\n"),
		".ai-rulez/skills/deploy-staging/SKILL.md": skill("deploy-staging", "Deploy a service to the staging cluster", "keywords: [rollout, staging]\n"),
		".ai-rulez/skills/git-workflow/SKILL.md":   skill("git-workflow", "Branching and pull request conventions", ""),
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	t.Chdir(root)
	return root
}

// resetSearch clears the command's flag state so tests do not leak into each other.
func resetSearch(t *testing.T) {
	t.Helper()
	reset := func() {
		SearchCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}
	reset()
	t.Cleanup(reset)
}

func setSearchFlag(t *testing.T, name, value string) {
	t.Helper()
	require.NoError(t, SearchCmd.Flags().Set(name, value))
}

func execSearch(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runSearch(SearchCmd, &out, &errOut, args)
	return code, out.String(), errOut.String()
}

func writeCases(t *testing.T, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cases.yaml")
	require.NoError(t, os.WriteFile(p, []byte(doc), 0o600))
	return p
}

const searchGoodCases = `version: 1
k: 3
cases:
  - {id: refund, query: "the customer wants money back", expect: [refund-policy], tags: [paraphrase]}
  - {id: staging, query: "rollout to staging", expect: [deploy-staging]}
  - {id: negative, query: "weather tomorrow", expect: []}
`

func TestSearch_Query(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		args  []string
		want  string
		first string
	}{
		{"text", nil, []string{"customer", "refund"}, "refund-policy", "refund-policy"},
		{"json", map[string]string{"format": "json"}, []string{"deploy to staging"}, `"deploy-staging"`, "deploy-staging"},
		{"no match", nil, []string{"zebra"}, "no served skill matches", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchProject(t)
			resetSearch(t)
			for k, v := range tt.flags {
				setSearchFlag(t, k, v)
			}

			code, out, _ := execSearch(t, tt.args...)

			assert.Equal(t, 0, code)
			assert.Contains(t, out, tt.want)
			if tt.flags["format"] == formatJSON {
				var doc searchDoc
				require.NoError(t, json.Unmarshal([]byte(out), &doc))
				assert.Equal(t, tt.first, doc.Results[0].Name)
				assert.Equal(t, "root", doc.Results[0].Domain)
				assert.NotEmpty(t, doc.Results[0].Digest)
				validateAgainst(t, "../../schema/search.v1.schema.json", []byte(out))
			}
		})
	}
}

func TestSearch_QueryMatchesFindSkillRanking(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	setSearchFlag(t, "format", "json")
	setSearchFlag(t, "limit", "2")

	code, out, _ := execSearch(t, "deploy staging pull request")

	require.Equal(t, 0, code)
	var doc searchDoc
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.Len(t, doc.Results, 2, "--limit caps the results")
	assert.GreaterOrEqual(t, doc.Results[0].Score, doc.Results[1].Score)
}

func TestSearch_FlagErrors(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		args  []string
	}{
		{"no query", nil, nil},
		{"blank query", nil, []string{"  "}},
		{"bad format", map[string]string{"format": "xml"}, []string{"q"}},
		{"limit too big", map[string]string{"limit": "99"}, []string{"q"}},
		{"eval flag without eval", map[string]string{"min": "top1=0.5"}, []string{"q"}},
		{"query with eval", map[string]string{"eval": "c.yaml"}, []string{"q"}},
		{"max-flips without baseline", map[string]string{"eval": "c.yaml", "max-flips": "1"}, nil},
		{"role and profile", map[string]string{"role": "x", "profile": "y"}, []string{"q"}},
		{"eval k above the maximum", map[string]string{"eval": "c.yaml", "k": "101"}, nil},
		{"eval negative k", map[string]string{"eval": "c.yaml", "k": "-1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchProject(t)
			resetSearch(t)
			for k, v := range tt.flags {
				setSearchFlag(t, k, v)
			}

			code, _, _ := execSearch(t, tt.args...)

			assert.Equal(t, 1, code)
		})
	}
}

func TestSearchEval(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	setSearchFlag(t, "eval", writeCases(t, searchGoodCases))
	setSearchFlag(t, "format", "json")
	setSearchFlag(t, "min", "top1=1,mrr=1")

	code, out, errOut := execSearch(t)

	require.Equal(t, 0, code, errOut)
	var res struct {
		K     int                                    `json:"k"`
		N     int                                    `json:"n"`
		NNeg  int                                    `json:"n_negative"`
		Modes map[string]struct{ Top1, MRR float64 } `json:"modes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, 3, res.K)
	assert.Equal(t, 2, res.N)
	assert.Equal(t, 1, res.NNeg)
	validateAgainst(t, "../../schema/search-eval.v1.schema.json", []byte(out))
}

func TestSearchEval_TextAndGateFailure(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	cases := writeCases(t, "version: 1\ncases:\n  - {id: miss, query: zebra, expect: [git-workflow]}\n  - {id: ok, query: pull request, expect: [git-workflow]}\n")
	setSearchFlag(t, "eval", cases)
	setSearchFlag(t, "min", "top1=0.9")

	code, out, errOut := execSearch(t)

	assert.Equal(t, 2, code)
	assert.Contains(t, out, "misses:")
	assert.Contains(t, out, "gate failed")
	assert.Contains(t, errOut, "AR9D4")
}

func TestSearchEval_BaselineFlips(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	passing := writeCases(t, "version: 1\ncases:\n  - {id: a, query: pull request, expect: [git-workflow]}\n  - {id: b, query: money back, expect: [refund-policy]}\n")
	baseline := filepath.Join(t.TempDir(), "base.json")
	setSearchFlag(t, "eval", passing)
	setSearchFlag(t, "out", baseline)
	code, _, errOut := execSearch(t)
	require.Equal(t, 0, code, errOut)
	_, err := os.Stat(baseline)
	require.NoError(t, err)

	// The same case ids, one of which no longer finds its skill.
	resetSearch(t)
	regressed := writeCases(t, "version: 1\ncases:\n  - {id: a, query: pull request, expect: [git-workflow]}\n  - {id: b, query: zebra, expect: [refund-policy]}\n")
	setSearchFlag(t, "eval", regressed)
	setSearchFlag(t, "baseline", baseline)
	code, out, errOut := execSearch(t)
	assert.Equal(t, 2, code, "default --max-flips is 0")
	assert.Contains(t, out, "regressed: b")
	assert.Contains(t, errOut, "AR9D4")

	setSearchFlag(t, "max-flips", "1")
	code, _, errOut = execSearch(t)
	assert.Equal(t, 0, code, errOut)
}

func TestSearchEval_CannotRun(t *testing.T) {
	tests := []struct {
		name  string
		cases string
		flags map[string]string
		want  string
	}{
		{"invalid cases", "version: 1\ncases:\n  - {id: a}\n", nil, "AR9D2"},
		{"unknown skill", "version: 1\ncases:\n  - {id: a, query: q, expect: [ghost]}\n", nil, "ghost"},
		{"bad min", searchGoodCases, map[string]string{"min": "top1"}, "--min"},
		{"missing baseline", searchGoodCases, map[string]string{"baseline": "/nonexistent/base.json"}, "baseline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchProject(t)
			resetSearch(t)
			setSearchFlag(t, "eval", writeCases(t, tt.cases))
			for k, v := range tt.flags {
				setSearchFlag(t, k, v)
			}

			code, _, _ := execSearch(t)

			assert.Equal(t, 1, code)
		})
	}
}

func TestSearchCmd_Registered(t *testing.T) {
	cmd, _, err := RootCmd.Find([]string{"search"})
	require.NoError(t, err)
	assert.Equal(t, SearchCmd, cmd)
	for _, name := range []string{"role", "profile", "source", "include-static", "eval", "min", "baseline", "max-flips", "out", "k", "format", "limit"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "--%s", name)
	}
}

func TestSearchEval_BaselineAtAnotherKIsRefused(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	cases := writeCases(t, "version: 1\ncases:\n  - {id: a, query: pull request, expect: [git-workflow]}\n")
	baseline := filepath.Join(t.TempDir(), "base.json")
	setSearchFlag(t, "eval", cases)
	setSearchFlag(t, "k", "3")
	setSearchFlag(t, "out", baseline)
	code, _, errOut := execSearch(t)
	require.Equal(t, 0, code, errOut)

	resetSearch(t)
	setSearchFlag(t, "eval", cases)
	setSearchFlag(t, "k", "7")
	setSearchFlag(t, "baseline", baseline)
	code, _, _ = execSearch(t)

	assert.Equal(t, 1, code)
}

func TestSearchEval_NegativeKIsRejected(t *testing.T) {
	searchProject(t)
	resetSearch(t)
	setSearchFlag(t, "eval", writeCases(t, "version: 1\ncases:\n  - {id: a, query: pull request, expect: [git-workflow]}\n"))
	setSearchFlag(t, "k", "-1")

	code, _, _ := execSearch(t)

	assert.Equal(t, 1, code)
}

func TestSearchEval_TextShowsTagsAndNegativesAligned(t *testing.T) {
	// Arrange
	searchProject(t)
	resetSearch(t)
	cases := writeCases(t, "version: 1\nk: 100\ncases:\n  - {id: ok, query: pull request, expect: [git-workflow], tags: [paraphrase]}\n  - {id: none, query: zebra crossing, expect: [], tags: [off-topic]}\n")
	setSearchFlag(t, "eval", cases)

	// Act
	code, out, errOut := execSearch(t)

	// Assert
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "by tag:")
	assert.Contains(t, out, "paraphrase")
	assert.Contains(t, out, "negatives (")
	assert.Contains(t, out, "none")
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "top1") || strings.HasPrefix(line, "recall@") || strings.HasPrefix(line, "hit@") || strings.HasPrefix(line, "mrr") {
			assert.Regexp(t, `^\S+ +\d\.\d{4}`, line)
			assert.Equal(t, 12, strings.Index(line, strings.Fields(line)[1]), line)
		}
	}
}
