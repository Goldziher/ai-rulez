package llm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sync"
)

// Fake is a deterministic in-memory Client for tests. It never touches the
// network. By default Chat answers "fake:<hash prefix of the request>" and Embed
// returns a hash-derived 8-dimensional vector per input, so equal inputs give
// equal outputs across runs.
type Fake struct {
	// ChatFunc, when set, produces the chat answer. Return a non-nil error to simulate failures.
	ChatFunc func(ChatRequest) (string, error)
	// EmbedDims is the vector length (default 8).
	EmbedDims int

	mu     sync.Mutex
	chats  []ChatRequest
	embeds []EmbedRequest
	closed bool
}

// NewFake returns a Fake with default behavior.
func NewFake() *Fake { return &Fake{} }

// Chat implements Client.
func (f *Fake) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	f.mu.Lock()
	f.chats = append(f.chats, req)
	f.mu.Unlock()
	text := "fake:" + hashPrefix(req)
	if f.ChatFunc != nil {
		var err error
		if text, err = f.ChatFunc(req); err != nil {
			return ChatResponse{}, err
		}
	}
	u := Usage{PromptTokens: EstimatePromptTokens(req), CompletionTokens: EstimateTokens(text)}
	model := req.Model
	if model == "" {
		model = "fake/model"
	}
	return ChatResponse{Text: text, Model: model, Usage: u}, nil
}

// Embed implements Client.
func (f *Fake) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	if err := ctx.Err(); err != nil {
		return EmbedResponse{}, err
	}
	f.mu.Lock()
	f.embeds = append(f.embeds, req)
	f.mu.Unlock()
	dims := f.EmbedDims
	if dims <= 0 {
		dims = 8
	}
	out := EmbedResponse{Model: req.Model}
	if out.Model == "" {
		out.Model = "fake/embedding"
	}
	for _, in := range req.Input {
		sum := sha256.Sum256([]byte(in))
		vec := make([]float32, dims)
		for i := range vec {
			word := binary.BigEndian.Uint32(sum[(i*4)%28 : (i*4)%28+4])
			vec[i] = float32(word%2000)/1000 - 1
		}
		out.Vectors = append(out.Vectors, vec)
		out.Usage.PromptTokens += EstimateTokens(in)
	}
	return out, nil
}

// Close implements Client.
func (f *Fake) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

// ChatCalls returns the chat requests received so far.
func (f *Fake) ChatCalls() []ChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ChatRequest(nil), f.chats...)
}

// EmbedCalls returns the embed requests received so far.
func (f *Fake) EmbedCalls() []EmbedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]EmbedRequest(nil), f.embeds...)
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func hashPrefix(v any) string {
	sum := sha256.Sum256([]byte(mustJSON(v)))
	return hex.EncodeToString(sum[:4])
}
