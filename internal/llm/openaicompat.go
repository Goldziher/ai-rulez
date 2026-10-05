package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DefaultOpenAIBaseURL is used when provider is openai (or empty) and no base_url is set.
const DefaultOpenAIBaseURL = "https://api.openai.com/v1"

// maxResponseBytes bounds how much of a response body is read.
const maxResponseBytes = 32 << 20

// openAICompat speaks the OpenAI chat and embeddings REST shapes against a
// base URL: OpenAI, Azure OpenAI deployments, LiteLLM and Bedrock gateways,
// Ollama, vLLM and similar. It uses net/http only.
type openAICompat struct {
	baseURL    string
	apiKey     string
	model      string
	embedModel string
	pricing    Pricing
	http       *http.Client
}

// newOpenAICompat builds the backend from cfg. getenv resolves api_key_env.
func newOpenAICompat(cfg Config, getenv func(string) string, hc *http.Client) (*openAICompat, error) {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		if cfg.Provider != "" && cfg.Provider != "openai" {
			return nil, newError(KindConfig, "%s llm-config-invalid: the openaicompat backend needs base_url for provider %q", CodeConfigInvalid, cfg.Provider)
		}
		base = DefaultOpenAIBaseURL
	}
	keyEnv := cfg.APIKeyEnv
	if keyEnv == "" && cfg.BaseURL == "" {
		keyEnv = "OPENAI_API_KEY"
	}
	key := ""
	if keyEnv != "" {
		if key = strings.TrimSpace(getenv(keyEnv)); key == "" {
			return nil, newError(KindAuth, "environment variable %s (api_key_env) is empty or unset", keyEnv)
		}
	}
	if hc == nil {
		hc = &http.Client{}
	}
	return &openAICompat{baseURL: base, apiKey: key, model: cfg.Model, embedModel: cfg.EmbeddingModel, pricing: NewPricing(cfg), http: hc}, nil
}

func (o *openAICompat) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, newError(KindConfig, "cannot build request: %v", scrubURLError(err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Kind: KindProvider, Message: "request failed: " + scrubURLError(err), Cause: errors.New("transport error")}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, &Error{Kind: KindProvider, Status: resp.StatusCode, Message: "reading response failed", Cause: errors.New("read error")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyHTTPError(resp.StatusCode, resp.Header.Get("Retry-After"), data)
	}
	return data, nil
}

// scrubURLError drops the URL (which could carry a query token) from net/http errors.
func scrubURLError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return RedactSecrets(ue.Err.Error())
	}
	return RedactSecrets(err.Error())
}

func (o *openAICompat) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := firstNonEmpty(req.Model, o.model)
	if model == "" {
		return ChatResponse{}, newError(KindConfig, "%s llm-config-invalid: no model configured; set [llm] model", CodeConfigInvalid)
	}
	body, err := encodeChat(model, req)
	if err != nil {
		return ChatResponse{}, newError(KindConfig, "cannot encode request: %v", err)
	}
	data, err := o.post(ctx, "/chat/completions", body)
	if err != nil {
		return ChatResponse{}, err
	}
	return decodeChat(data, o.pricing, model)
}

func (o *openAICompat) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := firstNonEmpty(req.Model, o.embedModel)
	if model == "" {
		return EmbedResponse{}, newError(KindConfig, "%s llm-config-invalid: no embedding model configured; set [llm] embedding_model", CodeConfigInvalid)
	}
	if len(req.Input) == 0 {
		return EmbedResponse{Model: model}, nil
	}
	body, err := encodeEmbed(model, req)
	if err != nil {
		return EmbedResponse{}, newError(KindConfig, "cannot encode request: %v", err)
	}
	data, err := o.post(ctx, "/embeddings", body)
	if err != nil {
		return EmbedResponse{}, err
	}
	return decodeEmbed(data, o.pricing, model, len(req.Input))
}

func (o *openAICompat) Close() error {
	o.http.CloseIdleConnections()
	return nil
}
