package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tempServer struct {
	mu     sync.Mutex
	hasTmp []bool
}

func (s *tempServer) calls() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.hasTmp...)
}

// newTempServer rejects a request that carries temperature with rejectBody (a 400) when
// rejectBody is not empty, and answers the others.
func newTempServer(t *testing.T, rejectBody string) (*tempServer, Config) {
	t.Helper()
	s := &tempServer{}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, has := readBody(r)["temperature"]
		s.mu.Lock()
		s.hasTmp = append(s.hasTmp, has)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if has && rejectBody != "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, rejectBody)
			return
		}
		fmt.Fprint(w, chatReply)
	})
	return s, localConfig(srv, Config{Provider: "openai", Model: "gpt-4o-mini", MaxRetries: -1})
}

func TestTemperatureIsOmittedOrRetriedWithout(t *testing.T) {
	const deprecated = "{\"message\":\"The model returned the following errors: `temperature` is deprecated for this model.\"}"
	tests := []struct {
		name      string
		reject    string
		omit      bool
		wantCalls []bool // per request: did it carry temperature
		wantErr   bool
	}{
		{"sent by default", "", false, []bool{true, true}, false},
		{"omitted when the setting says so", deprecated, true, []bool{false, false}, false},
		{"retried once without it on the temperature 400, then remembered", deprecated, false, []bool{true, false, false}, false},
		{"another 400 is not retried", `{"message":"max_tokens is too large"}`, false, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			reject := tc.reject
			if tc.wantErr {
				reject = "" // the error test uses its own server below
			}
			s, cfg := newTempServer(t, reject)
			if tc.wantErr {
				srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
					_, has := readBody(r)["temperature"]
					s.mu.Lock()
					s.hasTmp = append(s.hasTmp, has)
					s.mu.Unlock()
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, tc.reject)
				})
				cfg = localConfig(srv, Config{Provider: "openai", Model: "gpt-4o-mini", MaxRetries: -1})
			}
			cfg.OmitTemperature = tc.omit
			l := newLocal(t, cfg, nil)
			req := ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}

			// Act
			_, err1 := l.Chat(context.Background(), req)
			_, err2 := l.Chat(context.Background(), req)

			// Assert
			if tc.wantErr {
				require.Error(t, err1)
				require.Error(t, err2)
				assert.Equal(t, []bool{true, true}, s.calls(), "a 400 that is not about temperature is never retried")
				return
			}
			require.NoError(t, err1)
			require.NoError(t, err2)
			assert.Equal(t, tc.wantCalls, s.calls())
		})
	}
}

func TestTemperatureSettingFromTheEnvironment(t *testing.T) {
	tests := []struct {
		value   string
		want    bool
		wantErr string
	}{
		{"", false, ""},
		{"omit", true, ""},
		{" OMIT ", true, ""},
		{"0.5", false, "AI_RULEZ_LLM_TEMPERATURE must be omit"},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			// Act
			cfg, err := Config{}.WithEnv(func(k string) string {
				if k == "AI_RULEZ_LLM_TEMPERATURE" {
					return tc.value
				}
				return ""
			})

			// Assert
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.wantErr), err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.OmitTemperature)
		})
	}
}

func TestPriceOverrideFromTheEnvironmentPricesTheModel(t *testing.T) {
	// Arrange
	env := map[string]string{
		"AI_RULEZ_LLM_MODEL":                 "bedrock/global.anthropic.claude-x",
		"AI_RULEZ_LLM_PRICE_INPUT_PER_MTOK":  "2",
		"AI_RULEZ_LLM_PRICE_OUTPUT_PER_MTOK": "10",
	}

	// Act
	cfg, err := Config{}.WithEnv(func(k string) string { return env[k] })
	require.NoError(t, err)
	cost, known := NewPricing(cfg).Cost("bedrock/global.anthropic.claude-x", Usage{PromptTokens: 1_000_000, CompletionTokens: 500_000})

	// Assert
	assert.True(t, known)
	assert.InDelta(t, 7.0, cost, 1e-9)
}

func TestWithPriceOverrideBindsTheCurrentModel(t *testing.T) {
	// Arrange
	cfg := Config{Model: "bedrock/a"}.WithPriceOverride(1, 3)
	cfg.Model = "bedrock/a"

	// Act
	cost, known := NewPricing(cfg).Cost("bedrock/a", Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000})
	_, otherKnown := NewPricing(cfg).Cost("bedrock/zz.unknown", Usage{PromptTokens: 1})

	// Assert
	assert.True(t, known)
	assert.InDelta(t, 4.0, cost, 1e-9)
	assert.False(t, otherKnown, "an override prices only the model it was written for")
}

func TestOmitsTemperatureForClaudeFiveAndLater(t *testing.T) {
	tests := map[string]bool{
		"bedrock/global.anthropic.claude-sonnet-5-5":        true,
		"bedrock/global.anthropic.claude-opus-5-5":          true,
		"bedrock/eu.anthropic.claude-opus-5-5-v1:0":         true,
		"anthropic/claude-sonnet-5":                         true,
		"anthropic/claude-haiku-6-0":                        true,
		"bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0": false,
		"anthropic/claude-3-5-sonnet-latest":                false,
		"anthropic/claude-haiku-4-5":                        false,
		"openai/gpt-4o-mini":                                false,
		"bedrock/amazon.nova-pro-v1:0":                      false,
	}
	for model, want := range tests {
		t.Run(model, func(t *testing.T) {
			assert.Equal(t, want, omitsTemperature(model))
			req := chatRequest(model, ChatRequest{Temperature: 0})
			assert.Equal(t, want, req.Temperature == nil)
		})
	}
}
