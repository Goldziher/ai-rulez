package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveJudgeKeepsUserScopeKeysOutOfTheRepository(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	user := filepath.Join(dir, "ai-rulez")
	require.NoError(t, os.MkdirAll(user, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(user, "config.toml"), []byte("[judge]\nmax_cost_usd = 3\nconcurrency = 2\n[judge.models]\nadjudicator = \"bedrock/x\"\n"), 0o600))
	getenv := func(k string) string { return map[string]string{"XDG_CONFIG_HOME": dir}[k] }
	cfg := &Config{Judge: &JudgeConfig{
		FailOn: []string{"security:medium"}, MaxCostUSD: 500, AllowLLM: true,
		Models: map[string]string{"security": "evil/model"}, AllowPromptOverride: true,
	}}

	// Act
	res, err := cfg.ResolveJudge(getenv)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"security:medium"}, res.Repo.FailOn, "gating is the repository's to set")
	assert.Zero(t, res.Repo.MaxCostUSD)
	assert.False(t, res.Repo.AllowLLM)
	assert.Empty(t, res.Repo.Models)
	assert.ElementsMatch(t, []string{"models", "max_cost_usd", "allow_llm", "allow_prompt_override"}, res.IgnoredRepoKeys)
	require.NotNil(t, res.User)
	assert.Equal(t, 3.0, res.User.MaxCostUSD)
	assert.Equal(t, "bedrock/x", res.User.Models["adjudicator"])
}

func TestResolveJudgeWithoutAnyConfig(t *testing.T) {
	var cfg *Config
	res, err := cfg.ResolveJudge(func(string) string { return "" })
	require.NoError(t, err)
	assert.Nil(t, res.Repo)
	assert.Nil(t, res.User)
}

func TestJudgeConfigValidate(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		cfg  JudgeConfig
		want string
	}{
		{"ok", JudgeConfig{Categories: []string{"security"}, MinConfidence: "medium"}, ""},
		{"bad category", JudgeConfig{Categories: []string{"style"}}, "judge.categories"},
		{"bad confidence", JudgeConfig{MinConfidence: "certain"}, "judge.min_confidence"},
		{"prompt mode", JudgeConfig{Prompts: map[string]JudgePrompt{"security": {Mode: "replace", File: "a.md"}}}, "judge.prompts.security.mode"},
		{"prompt outside the project", JudgeConfig{Prompts: map[string]JudgePrompt{"quality": {Mode: "extend", File: "../x.md"}}}, "inside the project"},
		{"suppression without an expiry", JudgeConfig{Suppress: []JudgeSuppression{{Code: "JR-S05", Path: "a", Quote: "q", Reason: "r"}}}, "expires"},
		{"suppression too far ahead", JudgeConfig{Suppress: []JudgeSuppression{{Code: "JR-S05", Path: "a", Quote: "q", Reason: "r", Expires: "2030-01-01"}}}, "366 days"},
		{"suppression ok", JudgeConfig{Suppress: []JudgeSuppression{{Code: "JR-S05", Path: "a", Quote: "q", Reason: "r", Expires: "2026-12-01"}}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := tc.cfg.Validate(now)
			if tc.want == "" {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0], tc.want)
		})
	}
}
