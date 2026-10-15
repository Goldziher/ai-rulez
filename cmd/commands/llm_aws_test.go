package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

func TestBedrockEgressHost(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"region from AWS_REGION", map[string]string{"AWS_REGION": "eu-north-1"}, "bedrock-runtime.eu-north-1.amazonaws.com", false},
		{"AWS_REGION wins", map[string]string{"AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "eu-north-1"}, "bedrock-runtime.us-east-1.amazonaws.com", false},
		{"default region", map[string]string{"AWS_DEFAULT_REGION": "ap-southeast-2"}, "bedrock-runtime.ap-southeast-2.amazonaws.com", false},
		{"gov region", map[string]string{"AWS_REGION": "us-gov-west-1"}, "bedrock-runtime.us-gov-west-1.amazonaws.com", false},
		{"china region", map[string]string{"AWS_REGION": "cn-north-1"}, "bedrock-runtime.cn-north-1.amazonaws.com.cn", false},
		{"no region", nil, "", false},
		{"not a region", map[string]string{"AWS_REGION": "evil.example.com/x"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got, err := bedrockEgressHost(func(k string) string { return tc.env[k] })

			// Assert
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExportBedrockCredentials(t *testing.T) {
	bedrock := llm.Config{Model: "bedrock/global.anthropic.claude-sonnet-5-5"}
	resolved := awsCredentials{AccessKeyID: "ASIAEXAMPLE", SecretAccessKey: "secretvalue", SessionToken: "tok"}
	tests := []struct {
		name    string
		lc      llm.Config
		env     map[string]string
		resolve error
		want    map[string]string
		wantErr bool
	}{
		{"profile credentials are exported", bedrock, map[string]string{"AWS_PROFILE": "p"},
			nil, map[string]string{"AWS_ACCESS_KEY_ID": "ASIAEXAMPLE", "AWS_SECRET_ACCESS_KEY": "secretvalue", "AWS_SESSION_TOKEN": "tok"}, false},
		{"another provider is left alone", llm.Config{Model: "openai/gpt-4o"}, nil, nil, nil, false},
		{"a bearer token is enough", bedrock, map[string]string{"AWS_BEARER_TOKEN_BEDROCK": "t"}, nil, nil, false},
		{"an exported key pair is kept", bedrock, map[string]string{"AWS_ACCESS_KEY_ID": "a", "AWS_SECRET_ACCESS_KEY": "b"}, nil, nil, false},
		{"a named key variable is left alone", llm.Config{Model: bedrock.Model, APIKeyEnv: "MY_KEY"}, nil, nil, nil, false},
		{"a chain failure is returned", bedrock, nil, errors.New("no credentials"), nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			got := map[string]string{}
			resolve := func(context.Context) (awsCredentials, error) { return resolved, tc.resolve }
			set := func(k, v string) error { got[k] = v; return nil }

			// Act
			err := exportBedrockCredentials(context.Background(), tc.lc, func(k string) string { return tc.env[k] }, resolve, set)

			// Assert
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestJudgeRunsBedrockWithoutABaseURLAndPreparesCredentials(t *testing.T) {
	// Arrange
	judgeProject(t, map[string]string{"AGENTS.md": "Run tests.\n"})
	rig := newJudgeRig(t)
	rig.env["AWS_REGION"] = "eu-north-1"
	var prepared []string
	rig.prepare = func(_ context.Context, lc llm.Config, _ func(string) string) error {
		prepared = append(prepared, lc.FullModel())
		return nil
	}

	// Act
	out, code := rig.run(t, "--allow-llm", "--model", "bedrock/global.anthropic.claude-sonnet-5-5", "--no-cache", "--max-cost", "0", "--format", "json")

	// Assert
	require.Equal(t, 0, code, rig.stderr.String())
	assert.Contains(t, out, `"egress_host": "bedrock-runtime.eu-north-1.amazonaws.com"`)
	assert.Equal(t, []string{"bedrock/global.anthropic.claude-sonnet-5-5"}, prepared)
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
