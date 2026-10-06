package skillsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/samber/oops"
)

// Command provider limits: a hung or chatty program cannot stall or exhaust the run.
const (
	commandTimeout   = 30 * time.Second
	commandMaxStdout = 64 << 20
	commandMaxStderr = 4 << 10
)

// Embedding is what one Embed call returns: a vector per input text, in order,
// with what the call cost.
type Embedding struct {
	Vectors   [][]float32
	Tokens    int
	CostUSD   float64
	CostKnown bool
	Cached    bool
}

// Embedder turns texts into vectors. Fingerprint identifies the provider
// (endpoint host, not the key) and keys vector reuse together with the model.
type Embedder interface {
	Embed(ctx context.Context, texts []string) (Embedding, error)
	Fingerprint() string
	Model() string
}

// LLMEmbedder embeds through internal/llm, so the network gate, the budget, the
// cache and the redaction apply to every call.
type LLMEmbedder struct {
	Client llm.Client
	// ModelName is the model recorded in the index.
	ModelName string
	// RequestModel overrides [llm] embedding_model for the request; empty leaves the client's own routing
	// (including the provider prefix of the literllm backend) untouched.
	RequestModel string
	Provider     string
}

// Embed implements Embedder.
func (e *LLMEmbedder) Embed(ctx context.Context, texts []string) (Embedding, error) {
	resp, err := e.Client.Embed(ctx, llm.EmbedRequest{Model: e.RequestModel, Input: texts, PromptVersion: "search-v1"})
	if err != nil {
		return Embedding{}, oops.Wrapf(err, "embed %d texts", len(texts))
	}
	return Embedding{Vectors: resp.Vectors, Tokens: resp.Usage.Total(), CostUSD: resp.CostUSD, CostKnown: resp.CostKnown, Cached: resp.Cached}, nil
}

// Fingerprint implements Embedder.
func (e *LLMEmbedder) Fingerprint() string { return e.Provider }

// Model implements Embedder.
func (e *LLMEmbedder) Model() string { return e.ModelName }

// CommandEmbedder embeds by running a program: argv only, no shell, a scrubbed
// environment, a 30 s timeout and a capped stdout. The program reads
// {"input": ["text", ...]} on stdin and writes {"vectors": [[...], ...]} to
// stdout, one vector per input in order.
type CommandEmbedder struct {
	Argv    []string
	Dir     string
	PassEnv []string
	// ModelName is recorded in the index; the command's own model is its business.
	ModelName string
	// Getenv resolves the variables passed through; nil means os.Getenv.
	Getenv func(string) string
}

// Embed implements Embedder.
func (c *CommandEmbedder) Embed(ctx context.Context, texts []string) (Embedding, error) {
	if len(c.Argv) == 0 {
		return Embedding{}, oops.Errorf("search.embeddings.command is empty")
	}
	in, err := json.Marshal(map[string]any{"input": texts})
	if err != nil {
		return Embedding{}, oops.Wrapf(err, "encode the embedding request")
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...) //nolint:gosec // an explicit, trust-gated argv with no shell
	cmd.Dir = c.Dir
	cmd.Env = c.environment()
	cmd.Stdin = bytes.NewReader(in)
	stdout := &limitedBuffer{limit: commandMaxStdout}
	stderr := &limitedBuffer{limit: commandMaxStderr}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Embedding{}, oops.Wrapf(ctx.Err(), "the embedding command %s did not finish in %s", c.Argv[0], commandTimeout)
		}
		return Embedding{}, oops.Errorf("the embedding command %s failed: %v: %s", c.Argv[0], err, llm.RedactSecrets(strings.TrimSpace(stderr.String())))
	}
	if stdout.over {
		return Embedding{}, oops.Errorf("the embedding command %s wrote more than %d bytes", c.Argv[0], commandMaxStdout)
	}
	var out struct {
		Vectors [][]float32 `json:"vectors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return Embedding{}, oops.Errorf("the embedding command %s did not print {\"vectors\": [[...]]}: %v", c.Argv[0], err)
	}
	if len(out.Vectors) != len(texts) {
		return Embedding{}, oops.Errorf("the embedding command returned %d vectors for %d texts", len(out.Vectors), len(texts))
	}
	return Embedding{Vectors: out.Vectors, CostKnown: true}, nil
}

func (c *CommandEmbedder) environment() []string {
	getenv := c.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	var env []string
	for _, name := range append([]string{"PATH", "HOME"}, c.PassEnv...) {
		if v, ok := lookup(getenv, name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

func lookup(getenv func(string) string, name string) (string, bool) {
	v := getenv(name)
	return v, v != ""
}

// Fingerprint implements Embedder: the program, not its arguments.
func (c *CommandEmbedder) Fingerprint() string {
	if len(c.Argv) == 0 {
		return "command:"
	}
	return "command:" + c.Argv[0]
}

// Model implements Embedder.
func (c *CommandEmbedder) Model() string { return c.ModelName }

// limitedBuffer keeps at most limit bytes and remembers it dropped some.
type limitedBuffer struct {
	bytes.Buffer
	limit int
	over  bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); len(p) > room {
		b.over = true
		if room > 0 {
			b.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

var _ io.Writer = (*limitedBuffer)(nil)

// DegradedReason maps an embedding failure to the `degraded` value a result carries.
func DegradedReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, llm.ErrNetworkDisabled):
		return DegradedNetworkDisabled
	case errors.Is(err, llm.ErrBudget):
		return DegradedBudget
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, llm.ErrTimeout):
		return DegradedTimeout
	}
	return DegradedProvider
}

// Values of `degraded`: why a hybrid or vector search was answered lexically.
const (
	DegradedNoIndex         = "no_index"
	DegradedProvider        = "provider_unavailable"
	DegradedTimeout         = "timeout"
	DegradedBudget          = "budget"
	DegradedNetworkDisabled = "network_disabled"
)

// DegradedMessage is the one line printed to stderr for a degraded search.
func DegradedMessage(reason string) string {
	switch reason {
	case DegradedNoIndex:
		return "search: no usable index, ranking lexically (run 'ai-rulez search index')"
	case DegradedNetworkDisabled:
		return "search: the network is disabled ([llm] allow_network), ranking lexically"
	case DegradedBudget:
		return "search: the [llm] budget is spent, ranking lexically"
	case DegradedTimeout:
		return "search: the query embedding timed out, ranking lexically"
	case DegradedProvider:
		return "search: the embedding provider is unavailable, ranking lexically"
	}
	return fmt.Sprintf("search: ranking lexically (%s)", reason)
}

var errEmbedCount = errors.New("the embedder returned a different number of vectors than texts")
