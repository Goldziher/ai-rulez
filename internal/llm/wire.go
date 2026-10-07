package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The OpenAI-compatible JSON shapes shared by the openaicompat backend (over
// HTTP) and the literllm backend (over the FFI, which speaks the same shapes).

type wireMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type wireJSONSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type wireResponseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *wireJSONSchema `json:"json_schema,omitempty"`
}

type wireChatRequest struct {
	Model          string              `json:"model"`
	Messages       []wireMessage       `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	ResponseFormat *wireResponseFormat `json:"response_format,omitempty"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type wireChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage wireUsage `json:"usage"`
}

type wireEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type wireEmbedResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage wireUsage `json:"usage"`
}

// maxRetryAfterSeconds caps a provider-requested delay (one hour).
const maxRetryAfterSeconds = 3600

// encodeChat renders req for model as an OpenAI-compatible chat body.
func encodeChat(model string, req ChatRequest) ([]byte, error) {
	w := wireChatRequest{Model: model, Temperature: req.Temperature, MaxTokens: req.MaxTokens}
	for _, m := range req.Messages {
		w.Messages = append(w.Messages, wireMessage(m))
	}
	if rf := req.ResponseFormat; rf != nil {
		w.ResponseFormat = &wireResponseFormat{Type: "json_schema", JSONSchema: &wireJSONSchema{Name: rf.Name, Schema: rf.Schema, Strict: true}}
	}
	return json.Marshal(w)
}

func decodeChat(body []byte, pricing Pricing, fallbackModel string) (ChatResponse, error) {
	var w wireChatResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return ChatResponse{}, &Error{Kind: KindProvider, Message: "response is not valid chat JSON", Cause: err, permanent: true}
	}
	if len(w.Choices) == 0 {
		return ChatResponse{}, permanentError("response has no choices")
	}
	text := contentText(w.Choices[0].Message.Content)
	model := firstNonEmpty(w.Model, fallbackModel)
	usage := Usage{PromptTokens: max(w.Usage.PromptTokens, 0), CompletionTokens: max(w.Usage.CompletionTokens, 0)}
	cost, known := pricing.Cost(model, usage)
	return ChatResponse{Text: text, Model: model, Usage: usage, CostUSD: cost, CostKnown: known}, nil
}

func encodeEmbed(model string, req EmbedRequest) ([]byte, error) {
	return json.Marshal(wireEmbedRequest{Model: model, Input: req.Input})
}

// errEmbedCount marks a reply with the wrong number of vectors.
var errEmbedCount = errors.New("embedding count mismatch")

func decodeEmbed(body []byte, pricing Pricing, fallbackModel string, want int) (EmbedResponse, error) {
	var w wireEmbedResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return EmbedResponse{}, &Error{Kind: KindProvider, Message: "response is not valid embeddings JSON", Cause: err, permanent: true}
	}
	if len(w.Data) != want {
		e := permanentError("got %d embeddings for %d inputs", len(w.Data), want)
		e.Cause = errEmbedCount
		return EmbedResponse{}, e
	}
	vecs := make([][]float32, want)
	for _, d := range w.Data {
		if d.Index < 0 || d.Index >= want || vecs[d.Index] != nil {
			return EmbedResponse{}, permanentError("embedding index %d is out of range or repeated", d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	model := firstNonEmpty(w.Model, fallbackModel)
	usage := Usage{PromptTokens: max(w.Usage.PromptTokens, 0)}
	cost, known := pricing.Cost(model, usage)
	return EmbedResponse{Vectors: vecs, Model: model, Usage: usage, CostUSD: cost, CostKnown: known}, nil
}

// classifyHTTPError maps an HTTP failure to a typed error. body is scrubbed of
// anything key-like and truncated before it enters the message.
func classifyHTTPError(status int, retryAfter string, body []byte) *Error {
	msg := providerMessage(body)
	e := &Error{Status: status, Message: msg}
	lower := strings.ToLower(msg + " " + string(body[:min(len(body), 512)]))
	switch {
	case status == 401 || status == 403:
		e.Kind = KindAuth
	case status == 429:
		e.Kind = KindRateLimit
		// A non-finite or huge value is ignored or capped; the retry policy caps the wait again.
		if secs, err := strconv.ParseFloat(strings.TrimSpace(retryAfter), 64); err == nil && secs >= 0 && !math.IsInf(secs, 0) && !math.IsNaN(secs) {
			e.RetryAfter = time.Duration(min(secs, maxRetryAfterSeconds) * float64(time.Second))
		}
	case status == 413 || strings.Contains(lower, "context_length") || strings.Contains(lower, "context length") ||
		strings.Contains(lower, "maximum context") || strings.Contains(lower, "context window"):
		e.Kind = KindContextLength
	default:
		e.Kind = KindProvider
	}
	return e
}

func providerMessage(body []byte) string {
	var w struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	msg := ""
	if json.Unmarshal(body, &w) == nil && w.Error.Message != "" {
		msg = w.Error.Message
		if w.Error.Type != "" {
			msg = w.Error.Type + ": " + msg
		}
		if w.Error.Code != nil {
			msg = fmt.Sprintf("%s (code %v)", msg, w.Error.Code)
		}
	} else {
		msg = strings.TrimSpace(string(body))
	}
	msg = RedactSecrets(msg)
	if len(msg) > 300 {
		msg = msg[:300] + "..."
	}
	return msg
}

// contentText flattens message content, which is a string or a list of text parts.
func contentText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var sb strings.Builder
		for _, p := range c {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}
