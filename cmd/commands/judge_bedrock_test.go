package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJudgeRunsBedrockWithoutABaseURL(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)
	rig.env["AWS_REGION"] = "eu-north-1"

	// Act
	out, code := rig.run(t, "--allow-llm", "--model", "bedrock/global.anthropic.claude-sonnet-5-5", "--no-cache", "--max-cost", "0", "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Contains(t, out, `"bedrock-runtime.eu-north-1.amazonaws.com"`)
}

func TestJudgeBedrockWithoutARegionStillNeedsABaseURL(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)

	// Act
	_, code := rig.run(t, "--allow-llm", "--model", "bedrock/global.anthropic.claude-sonnet-5-5", "--no-cache", "--max-cost", "0")

	// Assert
	require.Equal(t, 1, code)
	assert.Contains(t, rig.stderr.String(), "AI_RULEZ_LLM_BASE_URL")
	assert.Zero(t, rig.built)
}

func TestJudgePriceFlagsMakeAnUnknownModelCapped(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)
	rig.env["AI_RULEZ_LLM_BASE_URL"] = "https://gateway.example.com/v1"

	// Act
	_, refused := rig.run(t, "--allow-llm", "--model", "someprovider/unpriced-model", "--no-cache")
	rig2 := newJudgeRig(t)
	rig2.env["AI_RULEZ_LLM_BASE_URL"] = "https://gateway.example.com/v1"
	_, priced := rig2.run(t, "--allow-llm", "--model", "someprovider/unpriced-model", "--no-cache", "--price-input", "3", "--price-output", "15")
	rig3 := newJudgeRig(t)
	_, half := rig3.run(t, "--estimate", "--model", "someprovider/unpriced-model", "--price-input", "3")

	// Assert
	assert.Equal(t, 1, refused, "no price, default cap")
	assert.Equal(t, 0, priced, rig2.stderr.String())
	assert.Equal(t, 1, half, "both prices are required")
}
