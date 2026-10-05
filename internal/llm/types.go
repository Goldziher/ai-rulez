// Package llm is ai-rulez's single abstraction for calling a language model.
//
// Features that need a model (rubric graders, semantic review, embeddings,
// verifiers) depend on Client and never on a provider SDK. New builds a Client
// from the [llm] config: a backend (pure-Go OpenAI-compatible HTTP, or the
// optional liter-llm binding behind the literllm build tag) wrapped in
// middleware for the network gate, cache, retries, budget and redaction.
//
// Nothing here calls out unless the config sets allow_network = true. Prompts
// and API keys are never logged.
package llm

import (
	"context"
	"time"
)

// Role is the author of a message.
type Role string

// Message roles.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one chat turn.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// JSONSchemaFormat asks the model for output that conforms to a JSON schema.
type JSONSchemaFormat struct {
	// Name identifies the schema (letters, digits, underscores).
	Name string `json:"name"`
	// Schema is the JSON schema document.
	Schema map[string]any `json:"schema"`
}

// ChatRequest is a provider-neutral chat completion request.
type ChatRequest struct {
	// Model overrides the configured model when non-empty.
	Model    string    `json:"model,omitempty"`
	Messages []Message `json:"messages"`
	// Temperature is sent as given; 0 means deterministic as far as the provider allows.
	Temperature float64 `json:"temperature"`
	// MaxTokens caps the completion. 0 lets the budget guard or the provider choose.
	MaxTokens int `json:"max_tokens,omitempty"`
	// ResponseFormat requests structured JSON output when set.
	ResponseFormat *JSONSchemaFormat `json:"response_format,omitempty"`
	// Timeout bounds the whole call including retries. 0 uses the configured default.
	Timeout time.Duration `json:"-"`
	// PromptVersion is mixed into the cache key: bump it when the prompt template
	// changes so stale answers are not served.
	PromptVersion string `json:"prompt_version,omitempty"`
	// NoCache skips the response cache for this call.
	NoCache bool `json:"-"`
}

// Usage is the token accounting of one call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Total returns prompt plus completion tokens.
func (u Usage) Total() int { return u.PromptTokens + u.CompletionTokens }

// ChatResponse is the result of a chat call.
type ChatResponse struct {
	Text  string `json:"text"`
	Model string `json:"model"`
	Usage Usage  `json:"usage"`
	// CostUSD is an estimate from the price table; see CostKnown.
	CostUSD float64 `json:"cost_usd"`
	// CostKnown is false when no price is known for the model (CostUSD is then 0).
	CostKnown bool `json:"cost_known"`
	// Cached is true when the answer came from the on-disk cache (no provider call, no cost).
	Cached bool `json:"cached,omitempty"`
}

// EmbedRequest asks for one embedding per input.
type EmbedRequest struct {
	// Model overrides the configured embedding model when non-empty.
	Model         string        `json:"model,omitempty"`
	Input         []string      `json:"input"`
	Timeout       time.Duration `json:"-"`
	PromptVersion string        `json:"prompt_version,omitempty"`
	NoCache       bool          `json:"-"`
}

// EmbedResponse holds one vector per input, in input order.
type EmbedResponse struct {
	Vectors   [][]float32 `json:"vectors"`
	Model     string      `json:"model"`
	Usage     Usage       `json:"usage"`
	CostUSD   float64     `json:"cost_usd"`
	CostKnown bool        `json:"cost_known"`
	Cached    bool        `json:"cached,omitempty"`
}

// Client is the one interface features use to reach a model.
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)
	Close() error
}
