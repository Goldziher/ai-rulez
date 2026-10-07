package evals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// Graders of a rubric.
const (
	// GraderRunner leaves rubric grading to the runner (the default).
	GraderRunner = "runner"
	// GraderBuiltin grades each rubric with the model layer's judge, from the
	// runner's transcript.
	GraderBuiltin = "builtin"
)

// maxTranscriptBytes bounds the transcript sent to a judge: a runner's whole
// output is untrusted data and its size is not a reason to run up a bill.
const maxTranscriptBytes = 64 << 10

// DefaultJudgeCompletionTokens is the least completion budget of a built-in judge call.
const DefaultJudgeCompletionTokens = llm.DefaultJudgeCompletionTokens

// completionFloor raises the completion budget of every request to at least min.
type completionFloor struct {
	llm.Client
	min int
}

// Chat implements llm.Client.
func (c completionFloor) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if req.MaxTokens < c.min {
		req.MaxTokens = c.min
	}
	return c.Client.Chat(ctx, req) //nolint:wrapcheck // the model layer's typed error is the useful one
}

// RubricGrade is one graded rubric.
type RubricGrade struct {
	// Score is in [0,1]: the share of the rubric the transcript satisfies.
	Score     float64
	Rationale string
}

// RubricGrader grades a rubric against a transcript. The transcript is data the
// grader must never follow as instructions.
type RubricGrader interface {
	// Name identifies the grader in reports and in the cache key.
	Name() string
	Grade(ctx context.Context, rubric, transcript string) (RubricGrade, error)
	// SpentUSD is the cumulative cost of the grader's calls.
	SpentUSD() float64
}

// JudgeGrader grades through the model layer: structured output at temperature
// 0, the transcript fenced as untrusted data by a per-request token the prompt
// states (grader_prompt.go), secret-looking text refused before anything is
// sent, and the budget, cache and network gate of the model layer.
type JudgeGrader struct {
	// Client is the model client (llm.New's managed client, or a fake in tests).
	Client llm.Client
	// Model names the model for the report and the cache key.
	Model string
	// Spent reports the cumulative cost of Client (llm.Managed.Spent); nil means
	// the cost is not tracked.
	Spent func() float64
	// RedactSecrets sends secret-looking text masked instead of refusing the case.
	RedactSecrets bool
	// MinCompletionTokens is the completion budget a judge call gets at least. The
	// model layer's judge asks for 300 tokens, which a reasoning model (Gemini 2.5
	// flash) spends on its thinking before it writes the verdict, so the reply is cut
	// off. Default DefaultJudgeCompletionTokens.
	MinCompletionTokens int

	mu sync.Mutex
}

// Name implements RubricGrader: the grader, its model and the judge prompt
// version, so a change to any of them re-runs a cached result.
func (g *JudgeGrader) Name() string {
	return fmt.Sprintf("%s:%s:%s", GraderBuiltin, g.Model, GraderPromptVersion)
}

// Grade implements RubricGrader.
func (g *JudgeGrader) Grade(ctx context.Context, rubric, transcript string) (RubricGrade, error) {
	g.mu.Lock() // one judge call at a time: the budget guard and the cost delta stay simple
	defer g.mu.Unlock()
	transcript = boundTranscript(transcript)
	floor := g.MinCompletionTokens
	if floor <= 0 {
		floor = DefaultJudgeCompletionTokens
	}
	v, err := judgeRubric(ctx, completionFloor{Client: g.Client, min: floor}, rubric, transcript, g.RedactSecrets)
	if err != nil {
		return RubricGrade{}, err
	}
	return RubricGrade{Score: round(v.Score), Rationale: oneLine(v.Rationale)}, nil
}

// SpentUSD implements RubricGrader.
func (g *JudgeGrader) SpentUSD() float64 {
	if g.Spent == nil {
		return 0
	}
	return g.Spent()
}

// boundTranscript keeps the head and the tail of an oversized transcript: the
// answer is usually at the end, the framing at the start.
func boundTranscript(t string) string {
	if len(t) <= maxTranscriptBytes {
		return t
	}
	half := maxTranscriptBytes / 2
	return t[:half] + "\n[... transcript truncated ...]\n" + t[len(t)-half:]
}

// oneLine collapses a rationale to one bounded line (it ends up in a report).
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:297] + "..."
	}
	return s
}

// gradeRubrics grades the rubric of every case that has one, from the output the
// runner returned, and sets Result.RubricScore. It overrides a runner's own rubric
// score (the point of the built-in grader is not to depend on it) and never grades
// a result with no output. When the runner also gave its own verdict (Passed, for
// example from its assertion graders) the rubric grade must pass as well: a
// passing assertion never stands in for an ungraded rubric. A call that fails
// leaves that result ungraded, which scores as a failure, with the reason as a
// warning. It returns the warnings and whether the model layer refused to run at
// all (network disabled), which ends the grading.
func gradeRubrics(ctx context.Context, g RubricGrader, cases []Case, resp *Response) (warnings []string, refused error) {
	byID := map[string]*Case{}
	for i := range cases {
		byID[cases[i].ID] = &cases[i]
	}
	for i := range resp.Results {
		r := &resp.Results[i]
		c := byID[r.Case]
		if c == nil || !c.HasRubric() || r.Skipped || r.Error != "" {
			continue
		}
		label := fmt.Sprintf("case %q (%s arm)", r.Case, r.Arm)
		r.builtinRubric = true
		if strings.TrimSpace(r.Output) == "" {
			warnings = append(warnings, label+": the runner returned no output, so there is no transcript for the built-in grader")
			r.RubricScore = nil
			continue
		}
		grade, err := g.Grade(ctx, c.RubricText(), r.Output)
		switch {
		case errors.Is(err, llm.ErrNetworkDisabled):
			return warnings, err
		case err != nil:
			warnings = append(warnings, fmt.Sprintf("%s: the built-in grader failed: %v", label, err))
			r.RubricScore = nil
			continue
		}
		score := grade.Score
		r.RubricScore, r.RubricRationale = &score, grade.Rationale
	}
	return warnings, nil
}
