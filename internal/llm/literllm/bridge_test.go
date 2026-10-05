//go:build cgo && literllm

package literllm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These tests link the real liter-llm static library; run them with the
// CGO_LDFLAGS described in docs/llm.md. They need no network beyond a local server.
func TestBridgeRoundTrip(t *testing.T) {
	var sawAuth, sawModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sawModel, _ = body["model"].(string)
		if strings.HasSuffix(r.URL.Path, "/embeddings") {
			fmt.Fprint(w, `{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[0.5,0.25]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`)
			return
		}
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
	}))
	defer srv.Close()

	n, err := New("sk-test", srv.URL+"/v1", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := n.ChatJSON([]byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "pong" || resp.Usage.PromptTokens != 5 {
		t.Fatalf("chat response %s (%v)", out, err)
	}
	if sawAuth != "Bearer sk-test" || sawModel != "gpt-4o-mini" {
		t.Fatalf("auth=%q model=%q", sawAuth, sawModel)
	}

	emb, err := n.EmbedJSON([]byte(`{"model":"m","input":["a"]}`))
	if err != nil || !strings.Contains(string(emb), `"embedding":[0.5,0.25]`) {
		t.Fatalf("embed %s (%v)", emb, err)
	}

	n.Free()
	n.Free() // idempotent
	if _, err := n.ChatJSON([]byte(`{"model":"m","messages":[]}`)); err == nil {
		t.Fatal("a freed client must return an error, not crash")
	}
}
