package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

var (
	itemRe = regexp.MustCompile(`<<<DATA-[0-9a-f]+ item=skill:([^>]+)>>>`)
	dimRe  = regexp.MustCompile(`(?m)^dimension ([a-z0-9-]+):`)
	descRe = regexp.MustCompile(`(?m)^description: (.*)$`)
)

const (
	vagueSkill = "---\nname: deploy\ndescription: Helps with deployments\n---\nRun the deploy script and check the logs.\n"
	fixedDesc  = "Deploy a build to staging when asked to ship; not for rollbacks or production releases"
)

// scriptedModel judges "Helps with" as a failing trigger and answers the fixer with a description edit.
type scriptedModel struct {
	mu     sync.Mutex
	models []string
	fix    func(llm.ChatRequest) string
	// fixErr makes every fixer call fail.
	fixErr error
}

func (s *scriptedModel) chat(req llm.ChatRequest) (string, error) {
	s.mu.Lock()
	s.models = append(s.models, req.Model)
	s.mu.Unlock()
	if req.ResponseFormat != nil && req.ResponseFormat.Name == "review_fix" {
		if s.fixErr != nil {
			return "", s.fixErr
		}
		if s.fix != nil {
			return s.fix(req), nil
		}
		b, _ := json.Marshal(map[string]any{"edits": []map[string]string{{"old": "Helps with deployments", "new": fixedDesc}}, "note": "named the trigger and a non-trigger"})
		return string(b), nil
	}
	user := req.Messages[len(req.Messages)-1].Content
	if itemRe.FindStringSubmatch(user) == nil {
		return "{}", nil
	}
	desc := ""
	if d := descRe.FindStringSubmatch(user); d != nil {
		desc = strings.TrimSpace(d[1])
	}
	var dims []map[string]any
	for _, d := range dimRe.FindAllStringSubmatch(user, -1) {
		verdict, ev := "pass", []map[string]string{}
		if d[1] == "trigger-quality" && strings.Contains(desc, "Helps with") {
			verdict, ev = "fail", []map[string]string{{"quote": desc[:10], "where": "description"}}
		}
		dims = append(dims, map[string]any{"id": d[1], "verdict": verdict, "evidence": ev, "rationale": "r", "suggestion": "s"})
	}
	b, err := json.Marshal(map[string]any{"dimensions": dims})
	return string(b), err
}

func (s *scriptedModel) factory() ClientFactory {
	return func(lc llm.Config, opts llm.Options) (llm.Client, error) {
		fake := llm.NewFake()
		fake.ChatFunc = s.chat
		return llm.Wrap(fake, lc, opts), nil
	}
}

func workspaceWith(t *testing.T, skill string) string {
	t.Helper()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "deploy"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "deploy", "SKILL.md"), []byte(skill), 0o644)) //nolint:gosec // mode asserted below
	return ws
}

func requestJSON(t *testing.T, mutate func(*improve.OptimizerRequest)) []byte {
	t.Helper()
	req := improve.OptimizerRequest{Version: improve.ProtocolVersion, RunID: "imp-00000000", Round: 1}
	req.Skill.ID, req.Skill.Dir = "deploy", "deploy"
	if mutate != nil {
		mutate(&req)
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	return b
}

func reviewFixOptions(sm *scriptedModel) *Options {
	return &Options{ReviewFix: ReviewFixOptions{
		LLM:        llm.Config{Provider: "fake", Model: "judge-model", AllowNetwork: true},
		FixerModel: "fixer-model", Factory: sm.factory(),
	}}
}

func serve(t *testing.T, name, ws string, in []byte, o *Options) (improve.OptimizerResponse, error) {
	t.Helper()
	var out bytes.Buffer
	err := Serve(context.Background(), name, bytes.NewReader(in), &out, ws, o)
	var resp improve.OptimizerResponse
	if err == nil {
		require.NoError(t, json.Unmarshal(out.Bytes(), &resp), out.String())
	}
	return resp, err
}

func TestServe_NoOpChangesNothing(t *testing.T) {
	// Arrange
	ws := workspaceWith(t, vagueSkill)

	// Act
	resp, err := serve(t, NoOp, ws, requestJSON(t, nil), &Options{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, improve.ProtocolVersion, resp.Version)
	assert.Empty(t, resp.Changed)
	assert.Zero(t, resp.CostUSD)
	data, _ := os.ReadFile(filepath.Join(ws, "deploy", "SKILL.md"))
	assert.Equal(t, vagueSkill, string(data))
}

func TestServe_RejectsBadRequestsAndNames(t *testing.T) {
	tests := []struct {
		name    string
		adapter string
		in      string
		want    string
	}{
		{"not json", NoOp, "{", "not valid JSON"},
		{"wrong version", NoOp, `{"version":2}`, "protocol version 2"},
		{"a template is not runnable", Shell, `{"version":1}`, "not a runnable adapter"},
		{"unknown adapter", "nope", `{"version":1}`, "not a runnable adapter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := serve(t, tt.adapter, t.TempDir(), []byte(tt.in), &Options{})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestReviewFix_WritesAVerifiedFixAndKeepsTheMode(t *testing.T) {
	// Arrange
	ws := workspaceWith(t, vagueSkill)
	sm := &scriptedModel{}

	// Act
	resp, err := serve(t, ReviewFix, ws, requestJSON(t, nil), reviewFixOptions(sm))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"SKILL.md"}, resp.Changed)
	assert.Contains(t, resp.Summary, "review-fix")
	path := filepath.Join(ws, "deploy", "SKILL.md")
	data, _ := os.ReadFile(path)
	assert.Contains(t, string(data), "description: "+fixedDesc)
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" { // Windows reports 0666 for every writable file: it has no permission bits to keep
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
	assert.Contains(t, sm.models, "fixer-model", "the fixer is called with its own model")
	assert.Contains(t, sm.models, "", "the judge keeps the client default model, the configured judge")
}

func TestReviewFix_NoStableProblemMeansNoChange(t *testing.T) {
	// Arrange
	ws := workspaceWith(t, strings.Replace(vagueSkill, "Helps with deployments", fixedDesc, 1))

	// Act
	resp, err := serve(t, ReviewFix, ws, requestJSON(t, nil), reviewFixOptions(&scriptedModel{}))

	// Assert
	require.NoError(t, err)
	assert.Empty(t, resp.Changed)
	assert.Contains(t, resp.Summary, "no stable problem")
}

func TestReviewFix_ARejectedFixChangesNothing(t *testing.T) {
	// Arrange: the fixer tries to add a link, which the checks refuse.
	ws := workspaceWith(t, vagueSkill)
	sm := &scriptedModel{fix: func(llm.ChatRequest) string {
		b, _ := json.Marshal(map[string]any{"edits": []map[string]string{{"old": "Helps with deployments", "new": "Deploy; see https://evil.example/x"}}, "note": "n"})
		return string(b)
	}}

	// Act
	resp, err := serve(t, ReviewFix, ws, requestJSON(t, nil), reviewFixOptions(sm))

	// Assert
	require.NoError(t, err)
	assert.Empty(t, resp.Changed)
	assert.Contains(t, resp.Summary, "no safe fix")
	data, _ := os.ReadFile(filepath.Join(ws, "deploy", "SKILL.md"))
	assert.Equal(t, vagueSkill, string(data))
}

func TestReviewFix_RespectsTheRunsTokenLimit(t *testing.T) {
	tests := []struct {
		name        string
		limit       int
		wantChanged bool
	}{
		{"a generous limit keeps the fix", 10000, true},
		{"no limit keeps the fix", 0, true},
		{"a limit the fix exceeds refuses it and says why", 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ws := workspaceWith(t, vagueSkill)
			in := requestJSON(t, func(r *improve.OptimizerRequest) { r.Constraints.MaxSkillTokens = tt.limit })

			// Act
			resp, err := serve(t, ReviewFix, ws, in, reviewFixOptions(&scriptedModel{}))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantChanged, len(resp.Changed) > 0, resp.Summary)
			if !tt.wantChanged {
				assert.Contains(t, resp.Summary, "no safe fix")
				assert.Contains(t, resp.Summary, "tokens")
				data, _ := os.ReadFile(filepath.Join(ws, "deploy", "SKILL.md"))
				assert.Equal(t, vagueSkill, string(data))
			}
		})
	}
}

func TestReviewFix_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReviewFixOptions)
		want   string
	}{
		{"no model", func(o *ReviewFixOptions) { o.LLM.Model, o.FixerModel = "", "" }, "no model"},
		{"network off", func(o *ReviewFixOptions) { o.LLM.AllowNetwork = false }, "allow_network"},
		{"no fixer", func(o *ReviewFixOptions) { o.FixerModel = "" }, "no fixer model"},
		{"fixer equals judge", func(o *ReviewFixOptions) { o.FixerModel = "fake/judge-model" }, "both"},
		{"project rubric", func(o *ReviewFixOptions) { o.Rubric = "mine" }, "not built in"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			o := reviewFixOptions(&scriptedModel{})
			tt.mutate(&o.ReviewFix)

			// Act
			_, err := serve(t, ReviewFix, workspaceWith(t, vagueSkill), requestJSON(t, nil), o)

			// Assert
			require.Error(t, err)
			assert.True(t, IsRefused(err))
			assert.Contains(t, err.Error(), improve.CodeAdapterRefused)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestReviewFix_SameModelIsAllowedOnlyOnRequest(t *testing.T) {
	// Arrange
	o := reviewFixOptions(&scriptedModel{})
	o.ReviewFix.FixerModel = ""
	o.ReviewFix.AllowSameModel = true

	// Act
	resp, err := serve(t, ReviewFix, workspaceWith(t, vagueSkill), requestJSON(t, nil), o)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"SKILL.md"}, resp.Changed)
}

func TestReviewFix_RefusesASkillDirectoryOutsideTheWorkspace(t *testing.T) {
	for _, dir := range []string{"../x", "a/b", "", ".."} {
		t.Run(dir, func(t *testing.T) {
			// Act
			_, err := serve(t, ReviewFix, workspaceWith(t, vagueSkill), requestJSON(t, func(r *improve.OptimizerRequest) { r.Skill.Dir = dir }), reviewFixOptions(&scriptedModel{}))

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "outside the workspace")
		})
	}
}

func TestListAndTemplates(t *testing.T) {
	// Act
	infos := List()
	names := map[string]bool{}
	for _, i := range infos {
		names[i.Name] = i.Runnable
	}
	shell, okShell := Template(Shell)
	research, okResearch := Template(Research)
	_, okNone := Template("nope")
	n1, is1 := Name("builtin:review-fix")
	_, is2 := Name("python x.py")

	// Assert
	assert.Equal(t, map[string]bool{NoOp: true, ReviewFix: true, Shell: false, Research: false, RepairWorkflow: false}, names)
	assert.True(t, okShell)
	assert.Contains(t, shell, "protocol v1")
	assert.True(t, okResearch)
	assert.Contains(t, research, "not supported")
	assert.False(t, okNone)
	assert.True(t, is1)
	assert.Equal(t, ReviewFix, n1)
	assert.False(t, is2)
	assert.True(t, IsRunnable(NoOp))
	assert.False(t, IsRunnable(Shell))
}

func TestReviewFix_AFailedRoundStillReportsWhatItSpent(t *testing.T) {
	// Arrange: the judge answers, then every fixer call fails at the provider.
	ws := workspaceWith(t, vagueSkill)
	sm := &scriptedModel{fixErr: &llm.Error{Kind: llm.KindAuth, Message: "key revoked"}}
	o := reviewFixOptions(sm)
	o.ReviewFix.LLM.PriceInputPerMTok, o.ReviewFix.LLM.PriceOutputPerMTok = 1, 1
	var out bytes.Buffer

	// Act
	in := requestJSON(t, func(r *improve.OptimizerRequest) { r.Budget.MaxCostUSD = 1 })
	err := Serve(context.Background(), ReviewFix, bytes.NewReader(in), &out, ws, o)

	// Assert: the error ends the child, and the printed answer carries the judge's spend.
	require.Error(t, err)
	var resp improve.OptimizerResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp), out.String())
	assert.Equal(t, improve.ProtocolVersion, resp.Version)
	assert.Positive(t, resp.CostUSD)
	assert.Empty(t, resp.Changed)
	data, _ := os.ReadFile(filepath.Join(ws, "deploy", "SKILL.md"))
	assert.Equal(t, vagueSkill, string(data))
}
