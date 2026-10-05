//go:build cgo

// Package literllm is the cgo bridge to the liter-llm Go binding. It lives in its
// own Go module so the default ai-rulez module graph, go.sum and CGO_ENABLED=0
// builds never see the binding. Only OpenAI-shaped JSON crosses this boundary;
// the request and response mapping lives in the parent llm package, where it is
// tested without cgo.
//
// Release builds enable it with a go.work that uses both modules and
// -tags literllm; see docs/llm.md. Status: experimental.
package literllm

import (
	"encoding/json"
	"errors"
	"sync"

	lit "github.com/xberg-io/liter-llm/packages/go/v2"
)

// Native wraps one liter-llm DefaultClient.
type Native struct {
	mu     sync.RWMutex // calls hold RLock; Free waits for in-flight calls (the binding cannot cancel them)
	client *lit.DefaultClient
}

// New creates a client. baseURL may be empty (provider routing decides). A
// timeoutSecs or maxRetries of 0 is passed through as 0 retries / default timeout.
func New(apiKey, baseURL string, timeoutSecs, maxRetries int) (*Native, error) {
	var base *string
	if baseURL != "" {
		base = &baseURL
	}
	var timeout *uint64
	if timeoutSecs > 0 {
		t := uint64(timeoutSecs)
		timeout = &t
	}
	retries := uint32(max(maxRetries, 0))
	c, err := lit.CreateClient(apiKey, base, timeout, &retries, nil)
	if err != nil {
		return nil, err
	}
	return &Native{client: c}, nil
}

var errFreed = errors.New("liter-llm client is closed")

// ChatJSON sends an OpenAI-shaped chat request and returns the response JSON.
func (n *Native) ChatJSON(body []byte) ([]byte, error) {
	var req lit.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.client == nil {
		return nil, errFreed
	}
	resp, err := n.client.Chat(req)
	if err != nil {
		return nil, err
	}
	if resp == nil { // the binding returns nil, nil when it cannot decode the native response
		return nil, errors.New("liter-llm returned an undecodable chat response")
	}
	return json.Marshal(resp)
}

// EmbedJSON sends an OpenAI-shaped embeddings request and returns the response JSON.
func (n *Native) EmbedJSON(body []byte) ([]byte, error) {
	var req lit.EmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.client == nil {
		return nil, errFreed
	}
	resp, err := n.client.Embed(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("liter-llm returned an undecodable embeddings response")
	}
	return json.Marshal(resp)
}

// Free releases the native client once in-flight calls finish.
func (n *Native) Free() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.client != nil {
		n.client.Free()
		n.client = nil
	}
}
