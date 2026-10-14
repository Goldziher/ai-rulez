package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const judgeLiveKey = "zq-live-7f3a9c1d5e8b2046"

func TestJudgeRefusesARubricWithASecret(t *testing.T) {
	tests := []struct {
		name   string
		rubric string
	}{
		{"a token shape", "Score 1 when clear. Use " + "token=" + judgeFileSecret + "xyz to verify.\n"},
		{"the provider key value", "Score 1 when clear. The key is " + judgeLiveKey + ".\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n", "rubric.md": tc.rubric})
			rig := newJudgeRig(t)
			rig.env["OPENAI_API_KEY"] = judgeLiveKey

			// Act
			stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--rubric", "rubric.md", "--redact")

			// Assert
			assert.Equal(t, 1, code, "--redact does not apply to a rubric")
			assert.Contains(t, rig.stderr.String(), "secret")
			assert.NotContains(t, rig.stderr.String()+stdout, judgeFileSecret)
			assert.NotContains(t, rig.stderr.String()+stdout, judgeLiveKey)
			assert.Empty(t, rig.fake.ChatCalls())
		})
	}
}

func TestJudgeScreensTheValueOfTheAPIKey(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"the provider's own variable", map[string]string{"OPENAI_API_KEY": judgeLiveKey}},
		{"the variable named by api key env", map[string]string{"AI_RULEZ_LLM_API_KEY_ENV": "MY_JUDGE_KEY", "MY_JUDGE_KEY": judgeLiveKey}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			judgeProject(t, map[string]string{
				"AGENTS.md":          "# Agents\nDeploy with " + judgeLiveKey + " always.\n",
				".agents/rules/a.md": "Keep functions short.\n",
			})
			rig := newJudgeRig(t)
			for k, v := range tc.env {
				rig.env[k] = v
			}

			// Act
			stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--format", "json", "--no-cache")

			// Assert
			assert.Equal(t, 2, code, "a file skipped for a secret is a finding")
			doc := parseJudgeDoc(t, stdout)
			assert.Equal(t, "skipped", doc.status("AGENTS.md"))
			assert.Equal(t, "judged", doc.status(".agents/rules/a.md"))
			assert.NotContains(t, stdout+rig.stderr.String(), judgeLiveKey)
			for _, req := range rig.requests() {
				assert.NotContains(t, req, judgeLiveKey)
			}
		})
	}
}

func TestJudgeMasksTheKeyValueWithRedact(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "# Agents\nDeploy with " + judgeLiveKey + " always.\n"})
	rig := newJudgeRig(t)
	rig.env["OPENAI_API_KEY"] = judgeLiveKey

	// Act
	_, code := rig.run(t, "--allow-llm", "--redact", "--model", judgeTestModel, "--no-cache")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	require.Len(t, rig.requests(), 1)
	assert.NotContains(t, rig.requests()[0], judgeLiveKey)
	assert.Contains(t, rig.requests()[0], "[REDACTED]")
}

func TestJudgeRedactsASecretInAFilePath(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"docs/" + judgeFileSecret + ".md": "Run tests.\n"})
	rig := newJudgeRig(t)

	// Act
	stdout, code := rig.run(t, "--allow-llm", "--model", judgeTestModel, "--no-cache", "--format", "json", "docs")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.NotContains(t, stdout, judgeFileSecret)
	assert.NotContains(t, rig.stderr.String(), judgeFileSecret)
	require.Len(t, rig.requests(), 1)
	assert.NotContains(t, rig.requests()[0], judgeFileSecret)
}
