package llm

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// A reasoning model rejects max_tokens and any temperature but its default, so the request
// must carry max_completion_tokens and no temperature; every other model keeps both.
func TestReasoningModelOmitsTemperatureAndUsesMaxCompletionTokens(t *testing.T) {
	tests := []struct {
		model      string
		wantTemp   bool
		wantMaxKey string
	}{
		{"gpt-5", false, "max_completion_tokens"},
		{"openai/gpt-5", false, "max_completion_tokens"},
		{"o4-mini", false, "max_completion_tokens"},
		{"gpt-4o-mini", true, "max_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			// Arrange
			var body map[string]any
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				body = readBody(r)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, chatReply)
			})
			cfg := localConfig(srv, Config{Provider: "openai", Model: tt.model, APIKeyEnv: "MY_KEY"})
			m := newLocal(t, cfg, func(string) string { return "k" })

			// Act
			if _, err := m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}, MaxTokens: 8}); err != nil {
				t.Fatalf("chat: %v", err)
			}

			// Assert
			if _, has := body["temperature"]; has != tt.wantTemp {
				t.Errorf("temperature present = %v, want %v; body = %v", has, tt.wantTemp, body)
			}
			if _, ok := body[tt.wantMaxKey]; !ok {
				t.Errorf("body has no %q: %v", tt.wantMaxKey, body)
			}
		})
	}
}
