//go:build literllm && cgo

package llm

import literllm "github.com/Goldziher/ai-rulez/v5/internal/llm/literllm"

// This file is compiled only with -tags literllm (and cgo) inside the literllm.work workspace that
// also uses the nested module internal/llm/literllm; see docs/llm.md.
func init() {
	RegisterNative(func(c NativeConfig) (NativeClient, error) {
		n, err := literllm.New(c.APIKey, c.BaseURL, c.ModelHint, c.TimeoutSeconds, c.MaxRetries)
		if err != nil {
			return nil, err
		}
		return n, nil
	})
}
