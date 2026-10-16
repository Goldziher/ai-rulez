package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJudgeRefusesARealRunWhenTheEgressHostIsUnknown(t *testing.T) {
	tests := []struct {
		name  string
		model string
		env   map[string]string
		want  int
	}{
		{"an unlisted provider", "someprovider/some-model", nil, 1},
		{"an unlisted provider with a base url", "someprovider/some-model", map[string]string{"AI_RULEZ_LLM_BASE_URL": "https://gateway.example.com/v1"}, 0},
		{"a listed provider", "mistral/mistral-small-latest", nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
			rig := newJudgeRig(t)
			for k, v := range tc.env {
				rig.env[k] = v
			}

			// Act
			_, code := rig.run(t, "--allow-llm", "--model", tc.model, "--no-cache", "--max-cost", "0")

			// Assert
			require.Equal(t, tc.want, code, rig.stderr.String())
			if tc.want == 1 {
				assert.Contains(t, rig.stderr.String(), "AI_RULEZ_LLM_BASE_URL")
				assert.Zero(t, rig.built, "no client is built, so nothing is sent")
				assert.Empty(t, rig.fake.ChatCalls())
			}
		})
	}
}

func TestJudgeRefusesABareModelWithoutAProvider(t *testing.T) {
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)

	_, code := rig.run(t, "--allow-llm", "--model", "gpt-4o-mini", "--no-cache", "--max-cost", "0")

	require.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "no provider")
	assert.Zero(t, rig.built)
}

func TestJudgeEstimateStillWorksWhenTheEgressHostIsUnknown(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--estimate", "--model", "someprovider/some-model")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Contains(t, stdout, "(the provider's default endpoint)")
}

func TestProviderEndpoint(t *testing.T) {
	tests := []struct {
		provider, host, keyEnv string
	}{
		{"openai", "api.openai.com", "OPENAI_API_KEY"},
		{"anthropic", "api.anthropic.com", "ANTHROPIC_API_KEY"},
		{"gemini", "generativelanguage.googleapis.com", "GEMINI_API_KEY"},
		{"azure", "", ""},
		{"vertex_ai", "", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			// Act
			host, keyEnv := providerEndpoint(tc.provider)

			// Assert
			assert.Equal(t, tc.host, host)
			assert.Equal(t, tc.keyEnv, keyEnv)
		})
	}
}
