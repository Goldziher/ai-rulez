package skillsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
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
	// Batch caps the texts of one call (0: no cap); see Batcher.
	Batch int
}

// MaxBatch implements Batcher.
func (e *LLMEmbedder) MaxBatch() int { return e.Batch }

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
// stdout, one vector per input in order. The program runs in its own process
// group, so the timeout also ends helpers that inherited its stdout.
type CommandEmbedder struct {
	Argv    []string
	Dir     string
	PassEnv []string
	// ModelName is recorded in the index; the command's own model is its business.
	ModelName string
	// Getenv resolves the variables passed through; nil means the process environment.
	Getenv func(string) string
	// Runner starts the process; nil means a real one.
	Runner runner.Runner
	// Timeout bounds one call; 0 means 30 s.
	Timeout time.Duration
}

// baseEnv are the variables the program always gets when set. The Windows ones
// are what a process needs to start there at all.
var baseEnv = []string{"PATH", "HOME", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP", "USERPROFILE"}

// Embed implements Embedder.
func (c *CommandEmbedder) Embed(ctx context.Context, texts []string) (Embedding, error) {
	if len(c.Argv) == 0 {
		return Embedding{}, oops.Errorf("search.embeddings.command is empty")
	}
	in, err := json.Marshal(map[string]any{"input": texts})
	if err != nil {
		return Embedding{}, oops.Wrapf(err, "encode the embedding request")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = commandTimeout
	}
	res := runner.Or(c.Runner).Run(ctx, runner.Spec{
		Argv: c.Argv, Dir: c.Dir, Env: c.environment(), Stdin: in, Timeout: timeout, MaxOutput: commandMaxStdout,
	})
	switch res.Status {
	case runner.StatusOK:
	case runner.StatusTimeout:
		return Embedding{}, oops.Wrapf(context.DeadlineExceeded, "the embedding command %s did not finish in %s", c.Argv[0], timeout)
	case runner.StatusUnavailable:
		return Embedding{}, oops.Errorf("the embedding command %s cannot be run: %v", c.Argv[0], res.Err)
	default:
		if ctx.Err() != nil {
			return Embedding{}, oops.Wrapf(ctx.Err(), "the embedding command %s did not finish", c.Argv[0])
		}
		stderr := truncateBytes(strings.TrimSpace(string(res.Stderr)), commandMaxStderr)
		return Embedding{}, oops.Errorf("the embedding command %s failed: %v: %s", c.Argv[0], res.Err, llm.RedactSecrets(stderr))
	}
	if res.StdoutTruncated {
		return Embedding{}, oops.Errorf("the embedding command %s wrote more than %d bytes", c.Argv[0], commandMaxStdout)
	}
	var out struct {
		Vectors [][]float32 `json:"vectors"`
	}
	if err := json.Unmarshal(res.Stdout, &out); err != nil {
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
		getenv = ambient.GetenvFunc(nil)
	}
	env := []string{}
	for _, name := range append(append([]string{}, baseEnv...), c.PassEnv...) {
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
