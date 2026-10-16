package judge

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// scriptedClient answers by role, recording every call.
type scriptedClient struct {
	mu       sync.Mutex
	calls    []llm.ChatRequest
	security func(req llm.ChatRequest) (string, string)
	quality  func(req llm.ChatRequest) (string, string)
	govern   func(req llm.ChatRequest) (string, string)
	adjud    func(req llm.ChatRequest) (string, string)
}

const emptyReviewerReply = `{"findings":[],"criteria_results":[],"coverage":{"not_reviewed":[]}}`

func (s *scriptedClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req)
	s.mu.Unlock()
	sys := req.Messages[0].Content
	pick := func(f func(llm.ChatRequest) (string, string)) (llm.ChatResponse, error) {
		text, finish := emptyReviewerReply, "stop"
		if f != nil {
			text, finish = f(req)
		}
		return llm.ChatResponse{Text: text, Model: req.Model, FinishReason: finish, Usage: llm.Usage{PromptTokens: 1000, CompletionTokens: 200}}, nil
	}
	switch {
	case strings.Contains(sys, "ROLE: security reviewer"):
		return pick(s.security)
	case strings.Contains(sys, "ROLE: quality reviewer"):
		return pick(s.quality)
	case strings.Contains(sys, "ROLE: governance reviewer"):
		return pick(s.govern)
	default:
		if s.adjud == nil {
			return llm.ChatResponse{Text: `{"decisions":[],"chains":[],"escalations":[],"coverage_notes":[]}`, FinishReason: "stop", Model: req.Model}, nil
		}
		return pick(s.adjud)
	}
}
func (s *scriptedClient) Embed(context.Context, llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (s *scriptedClient) Close() error { return nil }

func (s *scriptedClient) count(role string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if strings.Contains(c.Messages[0].Content, role) {
			n++
		}
	}
	return n
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func reviewerReply(t *testing.T, findings ...Candidate) string {
	t.Helper()
	for i := range findings {
		if findings[i].Chain == nil {
			findings[i].Chain = []Step{}
		}
		if findings[i].StaticRefs == nil {
			findings[i].StaticRefs = []string{}
		}
		if findings[i].AlsoAt == nil {
			findings[i].AlsoAt = []AlsoAt{}
		}
	}
	return mustJSON(t, ReviewerOutput{Findings: findings, CriteriaResults: []CriterionResult{}, Coverage: Coverage{NotReviewed: []NotReviewed{}}})
}

// memFS builds the Read function of a unit from a map.
func memRead(files map[string]string) func(string) SourceFile {
	return func(rel string) SourceFile {
		d, ok := files[rel]
		if !ok {
			return SourceFile{Path: rel, Skip: SkipUnreadable, SkipReason: "missing"}
		}
		return SourceFile{Path: rel, Data: []byte(d), Bytes: len(d)}
	}
}

func skillUnit(t *testing.T, files map[string]string, dir string) Unit {
	t.Helper()
	var members []string
	for p := range files {
		if strings.HasPrefix(p, dir+"/") {
			members = append(members, p)
		}
	}
	refs := Discover(members, false)
	if len(refs) != 1 {
		t.Fatalf("want one unit, got %d", len(refs))
	}
	return BuildUnit(refs[0], memRead(files), UnitOptions{})
}

func newTestEngine(c llm.Client, opts Options) *Engine {
	if opts.Models == (Models{}) {
		opts.Models = Models{Security: "bedrock/global.anthropic.claude-sonnet-5-5", Quality: "bedrock/global.anthropic.claude-sonnet-5-5", Governance: "bedrock/global.anthropic.claude-sonnet-5-5", Adjudicator: "bedrock/global.anthropic.claude-opus-5-5"}
	}
	if opts.FailOn.Rules == nil {
		opts.FailOn = DefaultFailOn()
	}
	return NewEngine(c, llm.NewPricing(llm.Config{}), opts)
}
