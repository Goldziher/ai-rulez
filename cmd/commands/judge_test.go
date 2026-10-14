package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const (
	judgeTestModel  = "openai/gpt-4o-mini"
	judgeGoodReply  = `{"score":0.9,"rationale":"clear and actionable"}`
	judgeFileSecret = "AKIAIOSFODNN7EXAMPLE"
)

// judgeRig is a fake model behind the real middleware, recording what the
// command built and sent.
type judgeRig struct {
	mu     sync.Mutex
	fake   *llm.Fake
	built  int
	cfg    llm.Config
	opts   llm.Options
	stderr *bytes.Buffer
	// beforeChat runs inside every chat call, with the stderr text written so far.
	beforeChat func(stderrSoFar string)
	env        map[string]string
	locked     bool
	reply      func(req llm.ChatRequest) (string, error)
}

func newJudgeRig(t *testing.T) *judgeRig {
	t.Helper()
	r := &judgeRig{fake: llm.NewFake(), env: map[string]string{}, stderr: &bytes.Buffer{}}
	r.reply = func(llm.ChatRequest) (string, error) { return judgeGoodReply, nil }
	r.fake.ChatFunc = func(req llm.ChatRequest) (string, error) {
		if r.beforeChat != nil {
			r.beforeChat(r.stderr.String())
		}
		return r.reply(req)
	}
	return r
}

func (r *judgeRig) deps() judgeDeps {
	return judgeDeps{
		newClient: func(lc llm.Config, opts llm.Options) (judgeClient, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.built++
			r.cfg, r.opts = lc, opts
			return llm.Wrap(r.fake, lc, opts), nil
		},
		getenv:       func(name string) string { return r.env[name] },
		policyLocked: func(string) bool { return r.locked },
	}
}

// requests returns the user messages sent to the model.
func (r *judgeRig) requests() []string {
	var out []string
	for _, c := range r.fake.ChatCalls() {
		out = append(out, c.Messages[len(c.Messages)-1].Content)
	}
	return out
}

// run executes the judge command in the current directory and returns the
// exit code the command line would end with.
func (r *judgeRig) run(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	cmd := newJudgeCmd(r.deps())
	cmd.SilenceUsage, cmd.SilenceErrors = true, true // the root command sets both
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(r.stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || !exitErr.Silent {
			r.stderr.WriteString(err.Error())
		}
		return out.String(), exitCodeFor(err)
	}
	return out.String(), 0
}

func judgeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return dir
}

const skillBody = "---\nname: review\ndescription: Use when asked to review a pull request.\n---\nRead the diff and report problems.\n"

func defaultProject(t *testing.T) string {
	return judgeProject(t, map[string]string{
		"AGENTS.md":                         "# Agents\nRun `go test ./...` before you commit.\n",
		"sub/AGENTS.md":                     "# Sub\nUse poly fmt.\n",
		".agents/rules/style.md":            "---\npriority: high\n---\nKeep functions short.\n",
		".agents/agents/helper.md":          "---\nname: helper\ndescription: Helps.\n---\nHelp the user.\n",
		".agents/skills/review/SKILL.md":    skillBody,
		".agents/skills/review/notes.md":    "a resource of the skill\n",
		"README.md":                         "# Project\n",
		"docs/guide.md":                     "# Guide\nRead me.\n",
		".claude/rules/other.md":            "Another harness.\n",
		"src/main.go":                       "package main\n",
		".agents/rules/not-markdown.txt":    "plain\n",
		".agents/rules/.hidden-config.json": "{}\n",
	})
}

type judgeDoc struct {
	Mode       string  `json:"mode"`
	Rubric     string  `json:"rubric"`
	Model      string  `json:"model"`
	EgressHost string  `json:"egress_host"`
	MinScore   float64 `json:"min_score"`
	Files      []struct {
		Path      string   `json:"path"`
		Status    string   `json:"status"`
		Score     *float64 `json:"score"`
		Passed    *bool    `json:"passed"`
		Rationale string   `json:"rationale"`
		Reason    string   `json:"reason"`
		Bytes     int      `json:"bytes"`
		Truncated bool     `json:"truncated"`
		Redacted  bool     `json:"redacted"`
	} `json:"files"`
	Estimate struct {
		Files               int     `json:"files"`
		PromptTokens        int     `json:"prompt_tokens"`
		MaxCompletionTokens int     `json:"max_completion_tokens"`
		CostUSD             float64 `json:"cost_usd"`
		CostKnown           bool    `json:"cost_known"`
	} `json:"estimate"`
	Summary struct {
		Judged  int     `json:"judged"`
		Passed  int     `json:"passed"`
		Failed  int     `json:"failed"`
		Skipped int     `json:"skipped"`
		Errors  int     `json:"errors"`
		Calls   int     `json:"calls"`
		CostUSD float64 `json:"cost_usd"`
	} `json:"summary"`
}

func parseJudgeDoc(t *testing.T, stdout string) judgeDoc {
	t.Helper()
	var d judgeDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &d), stdout)
	return d
}

func (d judgeDoc) paths() []string {
	out := make([]string, len(d.Files))
	for i, f := range d.Files {
		out[i] = f.Path
	}
	return out
}

func (d judgeDoc) status(path string) string {
	for _, f := range d.Files {
		if f.Path == path {
			return f.Status
		}
	}
	return ""
}

func TestJudgeDefaultTargetsNeedNoConfig(t *testing.T) {
	// Arrange
	defaultProject(t)
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	doc := parseJudgeDoc(t, stdout)
	assert.Equal(t, "judge", doc.Mode)
	assert.Equal(t, []string{
		".agents/agents/helper.md", ".agents/rules/style.md", ".agents/skills/review/SKILL.md", "AGENTS.md", "sub/AGENTS.md",
	}, doc.paths(), "AGENTS.md at any depth and the .agents tree, in order")
	assert.Len(t, rig.fake.ChatCalls(), 5)
	assert.Equal(t, 5, doc.Summary.Judged)
	assert.Equal(t, 5, doc.Summary.Passed)
}

func TestJudgeDefaultTargetsIncludeAIRulezContentWhenPresent(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{
		".ai-rulez/config.toml":             "version = \"5.0\"\nname = \"t\"\n",
		".ai-rulez/rules/style.md":          "Keep it short.\n",
		".ai-rulez/domains/web/rules/ui.md": "Use components.\n",
		".ai-rulez/skills/s/SKILL.md":       skillBody,
	})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Equal(t, []string{".ai-rulez/domains/web/rules/ui.md", ".ai-rulez/rules/style.md", ".ai-rulez/skills/s/SKILL.md"}, parseJudgeDoc(t, stdout).paths())
}

func TestJudgeExplicitPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"one file", []string{"docs/guide.md"}, []string{"docs/guide.md"}},
		{"a directory", []string{"docs"}, []string{"docs/guide.md"}},
		{"two paths in order", []string{"README.md", "docs/guide.md"}, []string{"README.md", "docs/guide.md"}},
		{"a file of another harness", []string{".claude/rules/other.md"}, []string{".claude/rules/other.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			defaultProject(t)
			rig := newJudgeRig(t)

			// Act
			stdout, code := rig.run(t, append([]string{"--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache"}, tc.args...)...)

			// Assert
			require.Equal(t, 0, code, rig.stderr.String())
			assert.Equal(t, tc.want, parseJudgeDoc(t, stdout).paths())
		})
	}
}

func TestJudgeRefusesWhatCannotBeJudged(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing path", []string{"nope.md"}, "does not exist"},
		{"outside the project", []string{"../outside.md"}, "outside the project"},
		{"not a judgeable file", []string{"src/main.go"}, "nothing to judge"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			defaultProject(t)
			rig := newJudgeRig(t)

			// Act
			_, code := rig.run(t, append([]string{"--allow-llm", "--model", judgeTestModel, "--no-cache"}, tc.args...)...)

			// Assert
			assert.Equal(t, 1, code)
			assert.Contains(t, rig.stderr.String(), tc.want)
			assert.Empty(t, rig.fake.ChatCalls())
		})
	}
}

func TestJudgeEmptyProjectHasNothingToJudge(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"README.md": "# x\n", ".agents/notes.txt": "not markdown\n"})
	rig := newJudgeRig(t)

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", judgeTestModel)

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "nothing to judge")
	assert.Zero(t, rig.built)
}

func TestJudgeNeedsExplicitConsentAndAModel(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    map[string]string
		locked bool
		want   string
	}{
		{"no --allow-llm", []string{"--model", judgeTestModel}, nil, false, "--allow-llm"},
		{"env allow_network is not consent", []string{"--model", judgeTestModel}, map[string]string{"AI_RULEZ_LLM_ALLOW_NETWORK": "1"}, false, "--allow-llm"},
		{"no model", []string{"--allow-llm"}, nil, false, "--model"},
		{"organization policy", []string{"--allow-llm", "--model", judgeTestModel}, nil, true, "organization policy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			defaultProject(t)
			rig := newJudgeRig(t)
			rig.locked = tc.locked
			for k, v := range tc.env {
				rig.env[k] = v
			}

			// Act
			_, code := rig.run(t, append(tc.args, "--no-cache")...)

			// Assert
			assert.Equal(t, 1, code)
			assert.Contains(t, rig.stderr.String(), tc.want)
			assert.Zero(t, rig.built, "no client is built and nothing is sent")
			assert.Empty(t, rig.fake.ChatCalls())
		})
	}
}

func TestJudgeEstimateSendsNothing(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{"estimate", "--estimate"},
		{"dry run", "--dry-run"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			defaultProject(t)
			require.NoError(t, os.WriteFile("AGENTS.md", []byte("key "+judgeFileSecret+"\n"), 0o600))
			rig := newJudgeRig(t)
			rig.locked = true // an estimate is not a call, so the policy does not stop it

			// Act
			stdout, code := rig.run(t, tc.flag, "--model", judgeTestModel)

			// Assert
			require.Equal(t, 0, code, rig.stderr.String())
			assert.Zero(t, rig.built, "an estimate builds no client, so it needs no key and no consent")
			assert.Empty(t, rig.fake.ChatCalls())
			assert.Contains(t, stdout, "nothing was sent")
			assert.Contains(t, stdout, "api.openai.com")
			assert.Contains(t, stdout, judgeTestModel)
			assert.Contains(t, stdout, ".agents/rules/style.md")
			assert.Contains(t, stdout, "AGENTS.md")
			assert.Contains(t, stdout, "secret-looking text")
			assert.NotContains(t, stdout, judgeFileSecret)
			assert.NotContains(t, stdout, "Keep functions short")
		})
	}
}

func TestJudgeEstimateJSON(t *testing.T) {
	// Arrange
	defaultProject(t)
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--estimate", "--model", judgeTestModel, "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	doc := parseJudgeDoc(t, stdout)
	assert.Equal(t, "estimate", doc.Mode)
	assert.Equal(t, "api.openai.com", doc.EgressHost)
	assert.Equal(t, judgeTestModel, doc.Model)
	assert.Equal(t, 5, doc.Estimate.Files)
	assert.Positive(t, doc.Estimate.PromptTokens)
	assert.True(t, doc.Estimate.CostKnown)
	assert.Positive(t, doc.Estimate.CostUSD)
	for _, f := range doc.Files {
		assert.Equal(t, "would_send", f.Status)
	}
}

func TestJudgeScoresDecideTheExitCode(t *testing.T) {
	tests := []struct {
		name     string
		reply    string
		minScore string
		want     int
		failed   int
	}{
		{"above the default floor", `{"score":0.9,"rationale":"fine"}`, "", 0, 0},
		{"below the default floor", `{"score":0.5,"rationale":"vague"}`, "", 2, 1},
		{"exactly at the floor passes", `{"score":0.5,"rationale":"ok"}`, "0.5", 0, 0},
		{"a high floor fails a good file", `{"score":0.9,"rationale":"fine"}`, "0.95", 2, 1},
		{"a zero floor passes everything", `{"score":0,"rationale":"bad"}`, "0", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
			rig := newJudgeRig(t)
			rig.reply = func(llm.ChatRequest) (string, error) { return tc.reply, nil }
			args := []string{"--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache"}
			if tc.minScore != "" {
				args = append(args, "--min-score", tc.minScore)
			}

			// Act
			stdout, code := rig.run(t, args...)

			// Assert
			assert.Equal(t, tc.want, code, rig.stderr.String())
			doc := parseJudgeDoc(t, stdout)
			assert.Equal(t, tc.failed, doc.Summary.Failed)
			require.Len(t, doc.Files, 1)
			require.NotNil(t, doc.Files[0].Passed)
			assert.Equal(t, tc.failed == 0, *doc.Files[0].Passed)
		})
	}
}

func TestJudgeTextReport(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n", "b/AGENTS.md": "Format.\n"})
	rig := newJudgeRig(t)
	rig.reply = func(req llm.ChatRequest) (string, error) {
		if strings.Contains(req.Messages[1].Content, "Format.") {
			return `{"score":0.2,"rationale":"too vague"}`, nil
		}
		return judgeGoodReply, nil
	}

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache")

	// Assert
	assert.Equal(t, 2, code)
	assert.Contains(t, stdout, "PASS")
	assert.Contains(t, stdout, "FAIL")
	assert.Contains(t, stdout, "0.90")
	assert.Contains(t, stdout, "AGENTS.md")
	assert.Contains(t, stdout, "too vague")
	assert.NotContains(t, stdout, "Run tests.", "file text is never printed")
}

func TestJudgeSecretsAreSkippedOrRedacted(t *testing.T) {
	files := map[string]string{
		"AGENTS.md":          "# Agents\nUse the key " + "token=" + judgeFileSecret + "xyz for deploys.\n",
		".agents/rules/a.md": "Keep functions short.\n",
	}
	t.Run("refused by default", func(t *testing.T) {
		// Arrange
		judgeProject(t, files)
		rig := newJudgeRig(t)

		// Act
		stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache")

		// Assert
		assert.Equal(t, 2, code, "a file skipped for secrets is a finding")
		doc := parseJudgeDoc(t, stdout)
		assert.Equal(t, "skipped", doc.status("AGENTS.md"))
		assert.Equal(t, "judged", doc.status(".agents/rules/a.md"))
		assert.Equal(t, 1, doc.Summary.Skipped)
		assert.Len(t, rig.fake.ChatCalls(), 1, "only the clean file is sent")
		for _, req := range rig.requests() {
			assert.NotContains(t, req, judgeFileSecret)
		}
		assert.NotContains(t, stdout, judgeFileSecret)
	})
	t.Run("masked with --redact", func(t *testing.T) {
		// Arrange
		judgeProject(t, files)
		rig := newJudgeRig(t)

		// Act
		stdout, code := rig.run(t, "--allow-llm", "--redact", "--model", judgeTestModel, "--format", "json", "--no-cache")

		// Assert
		assert.Equal(t, 0, code, rig.stderr.String())
		doc := parseJudgeDoc(t, stdout)
		assert.Equal(t, "judged", doc.status("AGENTS.md"))
		assert.Len(t, rig.fake.ChatCalls(), 2)
		for _, req := range rig.requests() {
			assert.NotContains(t, req, judgeFileSecret)
		}
		for _, f := range doc.Files {
			assert.Equal(t, f.Path == "AGENTS.md", f.Redacted, f.Path)
		}
	})
}

func TestJudgeNeverFollowsSymlinks(t *testing.T) {
	// Arrange
	dir := judgeProject(t, map[string]string{
		"AGENTS.md":        "Run tests.\n",
		"real/inside.md":   "Inside the project.\n",
		".agents/rules/ok": "x",
	})
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside the project\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, ".agents", "rules", "escape.md"))
	testutil.SymlinkOrSkip(t, filepath.Join(dir, "real", "inside.md"), filepath.Join(dir, ".agents", "rules", "link.md"))
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache")

	// Assert
	assert.Equal(t, 0, code, rig.stderr.String())
	doc := parseJudgeDoc(t, stdout)
	assert.Equal(t, "judged", doc.status("AGENTS.md"))
	assert.NotEqual(t, "judged", doc.status(".agents/rules/link.md"), "a symlink is never read")
	assert.NotEqual(t, "judged", doc.status(".agents/rules/escape.md"))
	for _, req := range rig.requests() {
		assert.NotContains(t, req, "outside the project")
		assert.NotContains(t, req, "Inside the project")
	}
}

func TestJudgeEndpointMustBeHTTPSOrLoopback(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		extra   map[string]string
		ok      bool
		host    string
	}{
		{"https", "https://gateway.example.com/v1", nil, true, "gateway.example.com"},
		{"plain http to a remote host", "http://gateway.example.com/v1", nil, false, ""},
		{"the plain http opt-in does not apply", "http://gateway.example.com/v1", map[string]string{
			"AI_RULEZ_LLM_ALLOW_PLAIN_HTTP": "1", "AI_RULEZ_LLM_PLAIN_HTTP_HOSTS": "gateway.example.com",
		}, false, ""},
		{"loopback ip", "http://127.0.0.1:8080/v1", nil, true, "127.0.0.1:8080"},
		{"localhost", "http://localhost:11434/v1", nil, true, "localhost:11434"},
		{"ftp", "ftp://gateway.example.com", nil, false, ""},
		{"credentials in the url", "https://user:pass@gateway.example.com/v1", nil, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
			rig := newJudgeRig(t)
			rig.env["AI_RULEZ_LLM_BASE_URL"] = tc.baseURL
			for k, v := range tc.extra {
				rig.env[k] = v
			}

			// Act
			_, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache")

			// Assert
			if tc.ok {
				assert.Equal(t, 0, code, rig.stderr.String())
				assert.Contains(t, rig.stderr.String(), tc.host)
				return
			}
			assert.Equal(t, 1, code)
			assert.Regexp(t, "(?i)base.?url", rig.stderr.String())
			assert.Zero(t, rig.built)
			assert.NotContains(t, rig.stderr.String(), "pass@")
		})
	}
}

func TestJudgeConfigFromFlagsAndEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		env   map[string]string
		check func(t *testing.T, lc llm.Config, opts llm.Options)
	}{
		{
			name: "defaults",
			args: []string{"--model", judgeTestModel},
			check: func(t *testing.T, lc llm.Config, _ llm.Options) {
				assert.True(t, lc.AllowNetwork)
				assert.InDelta(t, 0.25, lc.MaxCostUSD, 1e-9)
				assert.Zero(t, lc.MaxTokens)
				assert.Equal(t, judgeTestModel, lc.FullModel())
			},
		},
		{
			name: "environment fills what the flags leave out",
			args: nil,
			env:  map[string]string{"AI_RULEZ_LLM_MODEL": "openai/gpt-4o-mini", "AI_RULEZ_LLM_MAX_COST_USD": "0.5", "AI_RULEZ_LLM_API_KEY_ENV": "MY_KEY"},
			check: func(t *testing.T, lc llm.Config, _ llm.Options) {
				assert.Equal(t, judgeTestModel, lc.FullModel())
				assert.InDelta(t, 0.5, lc.MaxCostUSD, 1e-9)
				assert.Equal(t, "MY_KEY", lc.APIKeyEnv)
			},
		},
		{
			name: "flags beat the environment",
			args: []string{"--model", judgeTestModel, "--max-cost", "1.5", "--max-tokens", "9000"},
			env:  map[string]string{"AI_RULEZ_LLM_MODEL": "anthropic/other", "AI_RULEZ_LLM_MAX_COST_USD": "0.5", "AI_RULEZ_LLM_MAX_TOKENS": "5"},
			check: func(t *testing.T, lc llm.Config, _ llm.Options) {
				assert.Equal(t, judgeTestModel, lc.FullModel())
				assert.InDelta(t, 1.5, lc.MaxCostUSD, 1e-9)
				assert.Equal(t, 9000, lc.MaxTokens)
			},
		},
		{
			name: "provider flag prefixes a bare model",
			args: []string{"--provider", "openai", "--model", "gpt-4o-mini"},
			check: func(t *testing.T, lc llm.Config, _ llm.Options) {
				assert.Equal(t, judgeTestModel, lc.FullModel())
			},
		},
		{
			name: "caching is on unless --no-cache",
			args: []string{"--model", judgeTestModel},
			check: func(t *testing.T, _ llm.Config, opts llm.Options) {
				assert.False(t, opts.NoCache)
				assert.NotEmpty(t, opts.ConfigDir, "the cache is keyed by the project and lives in the user cache dir")
			},
		},
		{
			name:  "--no-cache disables it",
			args:  []string{"--model", judgeTestModel, "--no-cache"},
			check: func(t *testing.T, _ llm.Config, opts llm.Options) { assert.True(t, opts.NoCache) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
			rig := newJudgeRig(t)
			for k, v := range tc.env {
				rig.env[k] = v
			}

			// Act
			_, code := rig.run(t, append([]string{"--allow-llm"}, tc.args...)...)

			// Assert
			require.Equal(t, 0, code, rig.stderr.String())
			require.Equal(t, 1, rig.built)
			tc.check(t, rig.cfg, rig.opts)
		})
	}
}

func TestJudgeRejectsBadFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"min-score above 1", []string{"--min-score", "1.5"}, "--min-score"},
		{"negative min-score", []string{"--min-score", "-0.1"}, "--min-score"},
		{"negative max-cost", []string{"--max-cost", "-1"}, "--max-cost"},
		{"negative max-tokens", []string{"--max-tokens", "-1"}, "--max-tokens"},
		{"unknown format", []string{"--format", "yaml"}, "format"},
		{"unknown builtin rubric", []string{"--rubric", "builtin:nope"}, "unknown built-in rubric"},
		{"missing rubric file", []string{"--rubric", "missing-rubric.md"}, "missing-rubric.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
			rig := newJudgeRig(t)

			// Act
			_, code := rig.run(t, append([]string{"--allow-llm", "--model", judgeTestModel, "--no-cache"}, tc.args...)...)

			// Assert
			assert.Equal(t, 1, code)
			assert.Contains(t, rig.stderr.String(), tc.want)
			assert.Empty(t, rig.fake.ChatCalls())
		})
	}
}

func TestJudgeRubricFile(t *testing.T) {
	// Arrange
	dir := judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n", "rubric.md": "Score 1 only when the file mentions tests.\n"})
	testutil.SymlinkOrSkip(t, filepath.Join(dir, "rubric.md"), filepath.Join(dir, "linked.md"))
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json", "--rubric", "rubric.md", "AGENTS.md")
	_, linkedCode := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--rubric", "linked.md", "AGENTS.md")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Equal(t, "file:rubric.md", parseJudgeDoc(t, stdout).Rubric)
	require.Len(t, rig.requests(), 1)
	assert.Contains(t, rig.requests()[0], "Score 1 only when the file mentions tests.")
	assert.Equal(t, 1, linkedCode, "a rubric symlink is refused")
}

func TestJudgeBuiltinRubricIsTheDefault(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Equal(t, "builtin:instruction-quality", parseJudgeDoc(t, stdout).Rubric)
	require.Len(t, rig.requests(), 1)
	assert.Contains(t, rig.requests()[0], "Non-contradiction", "the rubric text is in the prompt")
}

func TestJudgePrintsTheEgressBeforeSending(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)
	rig.beforeChat = func(stderr string) {
		assert.Contains(t, stderr, "api.openai.com", "the host is printed before the first call")
		assert.Contains(t, stderr, judgeTestModel)
	}

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache")

	// Assert
	assert.Equal(t, 0, code, rig.stderr.String())
	assert.Len(t, rig.fake.ChatCalls(), 1)
}

func TestJudgeNeverPrintsPromptTextOrKeys(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Distinctive instruction sentence about gophers.\n"})
	rig := newJudgeRig(t)
	rig.env["OPENAI_API_KEY"] = "sk-test-0123456789abcdefghijklmnop"
	rig.reply = func(llm.ChatRequest) (string, error) {
		return `{"score":0.9,"rationale":"uses sk-test-0123456789abcdefghijklmnop as a key"}`, nil
	}

	// Act
	stdout, _ := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache")

	// Assert
	for _, out := range []string{stdout, rig.stderr.String()} {
		assert.NotContains(t, out, "Distinctive instruction sentence")
		assert.NotContains(t, out, "sk-test-0123456789")
	}
}

func TestJudgeStopsWhenTheEstimateExceedsTheCap(t *testing.T) {
	// Arrange
	defaultProject(t)
	rig := newJudgeRig(t)

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--max-cost", "0.0000001")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "--max-cost")
	assert.Empty(t, rig.fake.ChatCalls(), "nothing is sent when the worst case already exceeds the cap")
}

func TestJudgeRefusesAnUnpricedModelUnderACap(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)
	rig.env["AI_RULEZ_LLM_BASE_URL"] = "https://gateway.example.com/v1"

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", "acme/never-heard-of-it", "--no-cache")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "no price")
	assert.Empty(t, rig.fake.ChatCalls())
}

func TestJudgeProviderFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCalls  int
		wantErrors int
	}{
		{"an auth failure stops the run", &llm.Error{Kind: llm.KindAuth, Message: "bad key"}, 1, 3},
		{"a provider failure is per file", &llm.Error{Kind: llm.KindProvider, Message: "overloaded"}, 3, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "a\n", "b/AGENTS.md": "b\n", "c/AGENTS.md": "c\n"})
			rig := newJudgeRig(t)
			rig.reply = func(llm.ChatRequest) (string, error) { return "", tc.err }

			// Act
			stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json", "--max-cost", "0")

			// Assert
			assert.Equal(t, 1, code, "a run that could not judge every file is a failure, not a finding")
			doc := parseJudgeDoc(t, stdout)
			assert.Equal(t, tc.wantErrors, doc.Summary.Errors)
			assert.GreaterOrEqual(t, len(rig.fake.ChatCalls()), tc.wantCalls)
			for _, f := range doc.Files {
				assert.Equal(t, "error", f.Status)
				assert.NotEmpty(t, f.Reason)
			}
		})
	}
}

func TestJudgeTruncatesLongFiles(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "HEAD-MARK\n" + strings.Repeat("filler line of text\n", 5000) + "TAIL-MARK\n"})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	doc := parseJudgeDoc(t, stdout)
	require.Len(t, doc.Files, 1)
	assert.True(t, doc.Files[0].Truncated)
	req := rig.requests()[0]
	assert.Contains(t, req, "HEAD-MARK")
	assert.Contains(t, req, "TAIL-MARK")
	assert.Contains(t, req, "bytes omitted")
	assert.Less(t, len(req), 40000)
}

func TestJudgeSkipsGeneratedFiles(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{
		"AGENTS.md":          "<!-- Generated by ai-rulez. DO NOT EDIT. -->\nGenerated text.\n",
		"docs/guide.md":      "Guide.\n",
		".agents/rules/r.md": "Rule.\n",
	})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Equal(t, []string{".agents/rules/r.md"}, parseJudgeDoc(t, stdout).paths(), "the sources are judged, not their generated output")
}

func TestJudgeJSONMatchesItsSchema(t *testing.T) {
	// Arrange
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "judge-report.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	runs := map[string][]string{
		"judged":   {"--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json"},
		"estimate": {"--estimate", "--model", judgeTestModel, "--format", "json"},
	}
	for name, args := range runs {
		t.Run(name, func(t *testing.T) {
			defaultProject(t)
			require.NoError(t, os.WriteFile("sub/AGENTS.md", []byte("key "+judgeFileSecret+"\n"), 0o600))
			rig := newJudgeRig(t)

			// Act
			stdout, _ := rig.run(t, args...)

			// Assert
			result := compiled.Validate([]byte(stdout))
			assert.True(t, result.IsValid(), "%v\n%s", result.Errors, stdout)
		})
	}
}
