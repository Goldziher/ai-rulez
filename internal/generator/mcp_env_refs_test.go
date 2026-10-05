package generator

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestResolvePlaceholderMap_RecordsRefsOnlyForProcessEnv(t *testing.T) {
	t.Setenv("REF_PROC_TOKEN", "from-process")
	tests := []struct {
		name      string
		value     string
		overrides map[string]string
		dotenv    map[string]string
		baseDir   string
		wantValue string
		wantRef   bool
	}{
		{"process env records ref", "${REF_PROC_TOKEN}", nil, nil, "", "from-process", true},
		{"embedded process env records ref", "Bearer ${REF_PROC_TOKEN}", nil, nil, "", "Bearer from-process", true},
		{"--env override is not a ref", "${REF_PROC_TOKEN}", map[string]string{"REF_PROC_TOKEN": "o"}, nil, "", "o", false},
		{"dotenv is not a ref", "${REF_DOTENV_ONLY}", nil, map[string]string{"REF_DOTENV_ONLY": "d"}, "", "d", false},
		{"PROJECT_ROOT is not a ref", "${PROJECT_ROOT}", nil, nil, "/proj", "/proj", false},
		{"mixed sources are not a ref", "${REF_PROC_TOKEN}-${REF_DOTENV_ONLY}", nil,
			map[string]string{"REF_DOTENV_ONLY": "d"}, "", "from-process-d", false},
		{"unresolved lenient placeholder is not a ref", "${REF_NOWHERE}", nil, nil, "", "${REF_NOWHERE}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g := &Generator{config: &config.Config{MCPEnvOverrides: tt.overrides, BaseDir: tt.baseDir}}

			// Act
			resolved, _, _, refs := g.resolvePlaceholderMap("s.env", map[string]string{"K": tt.value}, tt.dotenv, isSensitiveEnvName)

			// Assert
			assert.Equal(t, tt.wantValue, resolved["K"])
			if tt.wantRef {
				assert.Equal(t, tt.value, refs["K"])
			} else {
				assert.NotContains(t, refs, "K")
			}
		})
	}
}

func TestIsMCPConfigOutput_CoversEveryMergedMCPDocument(t *testing.T) {
	for _, p := range []string{
		".codex/config.toml", ".cursor/mcp.json", ".vscode/mcp.json", ".agents/mcp_config.json",
		".devin/mcp_config.json", ".junie/mcp/mcp.json", ".mcp.json", "opencode.json",
	} {
		t.Run(p, func(t *testing.T) { assert.True(t, isMCPConfigOutput(p)) })
	}
	assert.False(t, isMCPConfigOutput(".codex/hooks.json"))
	assert.False(t, isMCPConfigOutput(".cursor/hooks.json"))
}

func TestMarkSensitiveOutputs_ReferenceOnlyMCPConfigIsNotSensitive(t *testing.T) {
	// Arrange: TOKEN resolved from the process env, so cursor writes ${env:TOKEN}.
	cfg := &config.Config{BaseDir: t.TempDir(), MCPServers: map[string]*config.MCPServer{
		"s": {
			Env: map[string]string{"TOKEN": "x"}, SecretEnvKeys: []string{"TOKEN"},
			EnvRefs: map[string]string{"TOKEN": "${TOKEN}"},
		},
	}}
	outputs := []config.OutputFile{
		{Path: filepath.Join(cfg.BaseDir, ".cursor/mcp.json"), Content: `{"env":{"TOKEN":"${env:TOKEN}"}}`},
		{Path: filepath.Join(cfg.BaseDir, ".mcp.json"), Content: `{"env":{"TOKEN":"x"}}`},
	}

	// Act
	NewGenerator(cfg).markSensitiveOutputs(outputs)

	// Assert
	assert.False(t, outputs[0].Sensitive, "only a reference, no secret value")
	assert.True(t, outputs[1].Sensitive, "the resolved value is written")
}
