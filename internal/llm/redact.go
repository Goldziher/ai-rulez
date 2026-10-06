package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// printf writes to w and ignores write errors: the writers are terminals and buffers.
func printf(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, format, args...) //nolint:errcheck // terminals and buffers
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T", v)
	}
	return string(b)
}

var secretRe = regexp.MustCompile(`(?i)(bearer\s+[A-Za-z0-9._~+/=-]{8,}|sk-[A-Za-z0-9_-]{8,}|AKIA[0-9A-Z]{12,}|(?:api[_-]?key|token|secret|authorization)["']?\s*[:=]\s*["']?[^\s"',}]{8,})`)

// RedactSecrets masks key-looking substrings. Provider error bodies can echo
// credentials, so every error message built from a response passes through it.
//
// An assignment whose value is an environment lookup (os.environ[...],
// os.Getenv(...), process.env.X, $VAR) names a secret without containing it and
// is left alone, so a transcript about reading a key from the environment is not
// refused as if it held one.
func RedactSecrets(s string) string {
	return secretRe.ReplaceAllStringFunc(s, func(m string) string {
		if envRefRe.MatchString(m) {
			return m
		}
		return "[REDACTED]"
	})
}

// envRefRe matches a secretRe assignment match whose value is an environment lookup.
var envRefRe = regexp.MustCompile(`(?i)^(?:api[_-]?key|token|secret|authorization)["']?\s*[:=]\s*["']?(?:os\.environ|os\.getenv|process\.env|\$\{?[A-Za-z_]|getenv\(|env\()`)

// Summary describes a chat request without its content: counts, sizes and a
// short content hash that lets two log lines be correlated.
func (r ChatRequest) Summary() string {
	chars := 0
	for _, m := range r.Messages {
		chars += len(m.Content)
	}
	sum := sha256.Sum256([]byte(mustJSON(r)))
	format := "text"
	if r.ResponseFormat != nil {
		format = "json_schema:" + r.ResponseFormat.Name
	}
	return fmt.Sprintf("model=%s messages=%d chars=%d est_prompt_tokens=%d max_tokens=%d format=%s hash=%s",
		r.Model, len(r.Messages), chars, EstimatePromptTokens(r), r.MaxTokens, format, hex.EncodeToString(sum[:4]))
}

// Summary describes an embed request without its content.
func (r EmbedRequest) Summary() string {
	chars := 0
	for _, s := range r.Input {
		chars += len(s)
	}
	sum := sha256.Sum256([]byte(mustJSON(r)))
	return fmt.Sprintf("model=%s inputs=%d chars=%d hash=%s", r.Model, len(r.Input), chars, hex.EncodeToString(sum[:4]))
}

// WithLogging wraps c so each call logs a content-free summary, the outcome and
// the usage. Prompts, completions and keys are never logged.
func WithLogging(c Client, log *slog.Logger) Client {
	if log == nil {
		return c
	}
	return &loggingClient{next: c, log: log}
}

type loggingClient struct {
	next Client
	log  *slog.Logger
}

func (l *loggingClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	start := ambient.Clock(nil).Now()
	resp, err := l.next.Chat(ctx, req)
	l.log.DebugContext(ctx, "llm chat", "request", req.Summary(), "elapsed", time.Since(start).Round(time.Millisecond),
		"prompt_tokens", resp.Usage.PromptTokens, "completion_tokens", resp.Usage.CompletionTokens,
		"cost_usd", resp.CostUSD, "cached", resp.Cached, "error", errSummary(err))
	return resp, err
}

func (l *loggingClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	start := ambient.Clock(nil).Now()
	resp, err := l.next.Embed(ctx, req)
	l.log.DebugContext(ctx, "llm embed", "request", req.Summary(), "elapsed", time.Since(start).Round(time.Millisecond),
		"prompt_tokens", resp.Usage.PromptTokens, "cached", resp.Cached, "error", errSummary(err))
	return resp, err
}

func (l *loggingClient) Close() error { return l.next.Close() }

func errSummary(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(RedactSecrets(err.Error()))
}
