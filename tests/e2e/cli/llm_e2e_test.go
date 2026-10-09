package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChatServer is an OpenAI-compatible endpoint that answers every chat call
// with "pong" and records the Authorization header it saw.
func fakeChatServer(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	calls := &atomic.Int32{}
	auth := &atomic.Value{}
	auth.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		auth.Store(r.Header.Get("Authorization"))
		_, _ = io.Copy(io.Discard, r.Body) //nolint:errcheck // test server
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`) //nolint:errcheck // test server
	}))
	t.Cleanup(srv.Close)
	return srv, calls, auth
}

func TestLLMCommandE2E(t *testing.T) {
	srv, calls, auth := fakeChatServer(t)
	network := map[string]string{
		"AI_RULEZ_LLM_ALLOW_NETWORK": "1",
		"AI_RULEZ_LLM_BASE_URL":      srv.URL + "/v1",
		"AI_RULEZ_LLM_MODEL":         "gpt-4o-mini",
		"AI_RULEZ_LLM_PROVIDER":      "openai",
		"AI_RULEZ_LLM_API_KEY_ENV":   "E2E_LLM_KEY",
		"E2E_LLM_KEY":                "sk-e2e-not-a-real-key-0000000000",
	}
	tests := []struct {
		name     string
		env      map[string]string
		args     []string
		wantCode int
		check    func(t *testing.T, stdout, stderr string)
	}{
		{
			name: "doctor json reports a closed network gate and no key", args: []string{"llm", "doctor", "--format", "json"},
			check: func(t *testing.T, stdout, _ string) {
				var doc map[string]any
				require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
				assert.Equal(t, false, doc["allow_network"])
				assert.Equal(t, false, doc["api_key_set"])
				assert.Regexp(t, `^v?\d+\.\d+\.\d+`, doc["literllm"])
			},
		},
		{
			name: "ping without allow_network is refused before any call", args: []string{"llm", "doctor", "--ping"}, wantCode: 1,
			check: func(t *testing.T, stdout, _ string) { assert.Contains(t, stdout, "allow_network") },
		},
		{
			name: "ping through the user-scope gateway sends one call and never prints the key", env: network,
			args: []string{"llm", "doctor", "--ping", "--format", "json"},
			check: func(t *testing.T, stdout, stderr string) {
				var doc struct {
					AllowNetwork bool `json:"allow_network"`
					APIKeySet    bool `json:"api_key_set"`
					Ping         struct {
						Attempted bool `json:"attempted"`
						OK        bool `json:"ok"`
					} `json:"ping"`
				}
				require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
				assert.True(t, doc.AllowNetwork)
				assert.True(t, doc.APIKeySet)
				assert.True(t, doc.Ping.Attempted)
				assert.True(t, doc.Ping.OK, stderr)
				assert.NotContains(t, stdout+stderr, "sk-e2e-not-a-real-key")
			},
		},
		{
			name: "estimate prices the configured model and calls nothing", args: []string{"llm", "estimate", ".ai-rulez/rules/local.md", "--format", "json"},
			check: func(t *testing.T, stdout, _ string) {
				var doc map[string]any
				require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
				assert.Greater(t, doc["prompt_tokens"], float64(0))
				assert.Equal(t, "gpt-4o-mini", doc["model"])
				assert.Equal(t, true, doc["cost_known"])
				assert.Greater(t, doc["cost_usd"], float64(0))
			},
		},
		{
			name: "estimate of a missing file cannot run", args: []string{"llm", "estimate", "missing.md"}, wantCode: 1,
		},
		{
			name: "a repository config cannot open the network gate",
			args: []string{"llm", "doctor", "--format", "json", "--config", filepath.Join(".ai-rulez", "config.toml")},
			check: func(t *testing.T, stdout, _ string) {
				var doc map[string]any
				require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
				assert.Equal(t, false, doc["allow_network"], "allow_network from a committed config is ignored")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := minimalProject(t, "\n[llm]\nmodel = \"gpt-4o-mini\"\nallow_network = true\nbase_url = \""+srv.URL+"/v1\"\n")
			env := newIsoEnv(t)
			for k, v := range tt.env {
				env.set(k, v)
			}
			before := calls.Load()

			// Act
			res := env.run(root, tt.args...)

			// Assert
			require.Equal(t, tt.wantCode, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			if tt.env == nil {
				assert.Equal(t, before, calls.Load(), "no call may reach the endpoint without the user-scope gate")
			}
			if tt.check != nil {
				tt.check(t, res.Stdout, res.Stderr)
			}
		})
	}
	assert.Equal(t, "Bearer sk-e2e-not-a-real-key-0000000000", auth.Load(), "the key goes only to the configured gateway, as a bearer token")
}
