package config

import "github.com/Goldziher/ai-rulez/internal/llm"

// ResolvedLLM returns the [llm] table (zero value when absent) with the
// AI_RULEZ_LLM_* environment overrides applied.
func (c *Config) ResolvedLLM() (llm.Config, error) {
	var lc llm.Config
	if c != nil && c.LLM != nil {
		lc = *c.LLM
	}
	return lc.WithEnv(nil)
}
