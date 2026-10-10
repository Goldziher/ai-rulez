package llm

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// liter-llm keeps a provider's own wire transform when a base_url is set (upstream #256,
// fixed in v2.2.3), so the Gemini and Vertex request shape and usage mapping can be
// asserted against a local server without a key or the network (#296).
func TestProviderTransformIsKeptWithABaseURL(t *testing.T) {
	const geminiReply = `{"candidates":[{"content":{"parts":[{"text":"pong"}],"role":"model"}}],` +
		`"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"thoughtsTokenCount":5,` +
		`"cachedContentTokenCount":3,"totalTokenCount":23}}`

	tests := []struct {
		provider string
		wantPath string
	}{
		{"gemini", "/v1/models/gemini-2.5-flash:generateContent"},
		{"vertex_ai", "/v1/publishers/google/models/gemini-2.5-flash:generateContent"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			// Arrange
			var gotPath string
			var gotBody map[string]any
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotBody = r.URL.Path, readBody(r)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, geminiReply)
			})
			cfg := localConfig(srv, Config{Provider: tt.provider, Model: "gemini-2.5-flash", APIKeyEnv: "MY_KEY"})
			l := newLocal(t, cfg, func(string) string { return "k" })

			// Act
			resp, err := l.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "ping"}}, MaxTokens: 8})

			// Assert: the provider transform, not the generic OpenAI-compatible one.
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if _, ok := gotBody["contents"]; !ok {
				t.Errorf("request body is not Gemini-shaped: %v", gotBody)
			}
			if _, ok := gotBody["messages"]; ok {
				t.Errorf("request went out as a generic OpenAI-compatible body: %v", gotBody)
			}
			if resp.Text != "pong" {
				t.Errorf("text = %q, want pong", resp.Text)
			}
			// usage: prompt 11, cached 3 (a subset), completion charges the thinking tokens.
			if resp.Usage.PromptTokens != 11 || resp.Usage.CachedTokens != 3 || resp.Usage.CompletionTokens < 12 {
				t.Errorf("usage = %+v, want prompt 11, cached 3, completion >= 12", resp.Usage)
			}
			if !resp.CostKnown || resp.CostUSD <= 0 {
				t.Errorf("cost = %v (known %v), want a priced gemini call", resp.CostUSD, resp.CostKnown)
			}
		})
	}
}
