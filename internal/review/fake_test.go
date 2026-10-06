package review

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

var (
	fenceItemRe = regexp.MustCompile(`<<<DATA-[0-9a-f]+ item=([^>]+)>>>`)
	dimLineRe   = regexp.MustCompile(`(?m)^dimension ([a-z0-9-]+):`)
	voteLineRe  = regexp.MustCompile(`(?m)^review-vote: (\d+)$`)
	descLineRe  = regexp.MustCompile(`(?m)^description: (.*)$`)
)

// scriptedJudge answers judge requests from a table: verdict(item, dimension, vote) decides the
// verdict, and the quote is the first words of the item's description unless quote overrides it.
type scriptedJudge struct {
	// decide, when set, wins over verdict and sees the whole call (description, user message).
	decide  func(c scriptedCall, dim string) string
	verdict func(item, dim string, vote int) string
	quote   func(item, dim, description string) string

	mu    sync.Mutex
	calls []scriptedCall
}

type scriptedCall struct {
	Item string
	Desc string
	Dims []string
	Vote int
	Temp float64
	User string
}

func (s *scriptedJudge) chat(req llm.ChatRequest) (string, error) {
	user := req.Messages[len(req.Messages)-1].Content
	m := fenceItemRe.FindStringSubmatch(user)
	if m == nil {
		return "", nil
	}
	item := strings.TrimPrefix(m[1], "skill:")
	vote := 1
	if v := voteLineRe.FindStringSubmatch(user); v != nil {
		vote, _ = strconv.Atoi(v[1])
	}
	var dims []string
	for _, d := range dimLineRe.FindAllStringSubmatch(user, -1) {
		dims = append(dims, d[1])
	}
	desc := ""
	if d := descLineRe.FindStringSubmatch(user); d != nil {
		desc = d[1]
	}
	call := scriptedCall{Item: item, Desc: desc, Dims: dims, Vote: vote, Temp: req.Temperature, User: user}
	s.mu.Lock()
	s.calls = append(s.calls, call)
	s.mu.Unlock()
	var reply judgeReply
	for _, d := range dims {
		v := VerdictPass
		switch {
		case s.decide != nil:
			v = s.decide(call, d)
		case s.verdict != nil:
			v = s.verdict(item, d, vote)
		}
		dv := DimVerdict{ID: d, Verdict: v, Rationale: "because " + d, Suggestion: "improve " + d}
		if v != VerdictPass {
			q := strings.Join(strings.Fields(desc), " ")
			if len(q) > 24 {
				q = q[:24]
			}
			if s.quote != nil {
				q = s.quote(item, d, desc)
			}
			dv.Evidence = []Quote{{Quote: q, Where: "description"}}
		} else {
			dv.Evidence = []Quote{}
		}
		reply.Dimensions = append(reply.Dimensions, dv)
	}
	b, err := json.Marshal(reply)
	return string(b), err
}

func (s *scriptedJudge) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *scriptedJudge) callsFor(item string) []scriptedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []scriptedCall
	for _, c := range s.calls {
		if c.Item == item {
			out = append(out, c)
		}
	}
	return out
}

// newClient wraps a fake backend in the real middleware (cache, budget, gate) with the network on.
func newClient(t *testing.T, s *scriptedJudge, cfg llm.Config) (llm.Client, *llm.Fake) {
	t.Helper()
	fake := llm.NewFake()
	fake.ChatFunc = s.chat
	cfg.AllowNetwork = true
	if cfg.Model == "" {
		cfg.Model = "fake/model"
	}
	dir := t.TempDir()
	opts := llm.Options{ConfigDir: dir, CacheDir: filepath.Join(dir, "cache"), SecretPath: filepath.Join(dir, "secret.key")}
	return llm.Wrap(fake, cfg, opts), fake
}

func mustRun(t *testing.T, in SemanticInput) *SemanticOutcome {
	t.Helper()
	out, err := RunSemantic(t.Context(), in)
	require.NoError(t, err)
	return out
}
