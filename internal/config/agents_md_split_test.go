package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAgentsMDSplit(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"off by default", Config{}, ""},
		{"with the agents source", Config{Source: SourceAgents, AgentsMDSplit: true}, ""},
		{"with the agents layout", Config{AgentsLayout: true, AgentsMDSplit: true}, ""},
		{"needs the agents source", Config{AgentsMDSplit: true}, "agents_md_split needs source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.cfg.validateSource()
			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestAgentsMDSplitRoundTripsThroughTOML(t *testing.T) {
	// Arrange
	cfg, err := decodeConfigTOML([]byte("version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\nagents_md_split = true\n"), "config.toml")
	require.NoError(t, err)

	// Act
	out, err := MarshalTOML(cfg)

	// Assert
	require.NoError(t, err)
	assert.True(t, cfg.AgentsMDSplit)
	assert.Contains(t, string(out), "agents_md_split = true")

	plain, err := decodeConfigTOML([]byte("version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\n"), "config.toml")
	require.NoError(t, err)
	written, err := MarshalTOML(plain)
	require.NoError(t, err)
	assert.False(t, plain.AgentsMDSplit)
	assert.NotContains(t, string(written), "agents_md_split", "the default is not written")
}

func TestLayoutOfReportsAgentsMDSplit(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		want bool
	}{
		{"config in .agents", ".agents/ai-rulez.toml", agentsTestConfig + "agents_md_split = true\n", true},
		{"source key in .ai-rulez", ".ai-rulez/config.toml", agentsTestConfig + "source = \"agents\"\nagents_md_split = true\n", true},
		{"off", ".agents/ai-rulez.toml", agentsTestConfig, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsFile(t, root, tt.path, tt.body)
			dir := root + "/.agents"
			if tt.path == ".ai-rulez/config.toml" {
				dir = root + "/.ai-rulez"
			}

			// Act
			layout := LayoutOf(dir)

			// Assert
			assert.True(t, layout.Agents)
			assert.Equal(t, tt.want, layout.AgentsMDSplit)
		})
	}
}
