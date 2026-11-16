package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func servingConfig(mcp *MCPConfig, presets ...string) *Config {
	cfg := &Config{
		MCP:           mcp,
		ConfigDirName: ".ai-rulez",
		SkillSources:  []SkillSourceConfig{{Name: "vendor", URL: "vendor-skills"}},
	}
	for _, p := range presets {
		cfg.Presets = append(cfg.Presets, Preset{BuiltIn: p})
	}
	return cfg
}

func TestSkillsInstructions(t *testing.T) {
	off := false

	t.Run("default text names the tools, the server and the static path", func(t *testing.T) {
		text, ok := servingConfig(nil, "claude").SkillsInstructions()
		assert.True(t, ok)
		assert.Contains(t, text, "## Unlisted skills")
		assert.Contains(t, text, "Before answering a repository-specific question")
		assert.Contains(t, text, "repository-relative `path` when known")
		assert.Contains(t, text, "before concluding that the repository has no guidance")
		assert.Contains(t, text, "Build the query from the requested operation")
		assert.Contains(t, text, "Read a referenced file if needed to resolve ambiguity.")
		assert.Contains(t, text, "Base the operation on the request and file contents.")
		assert.Contains(t, text, "covers the specific operation requested at that path")
		assert.Contains(t, text, "If all matches cover other workflows, continue without a skill.")
		assert.Contains(t, text, "Skip this lookup for unrelated questions.")
		assert.Contains(t, text, "`ToolSearch`")
		assert.Contains(t, text, "`find_skill`")
		assert.Contains(t, text, "`load_skill`")
		assert.Contains(t, text, "`ai-rulez` MCP server")
		assert.Contains(t, text, "`.ai-rulez/skills/<name>/SKILL.md`")
	})

	t.Run("the server is the one that runs --serve-skills", func(t *testing.T) {
		cfg := servingConfig(nil, "claude")
		cfg.MCPServersRaw = []MCPServer{{Name: "skills", Command: "ai-rulez", Args: []string{"mcp", "--serve-skills"}}}
		text, ok := cfg.SkillsInstructions()
		assert.True(t, ok)
		assert.Contains(t, text, "`skills` MCP server")
	})

	t.Run("custom text substitutes the documented variables", func(t *testing.T) {
		cfg := servingConfig(&MCPConfig{SkillsInstructionsText: "Use {{server}}; files in {{skills_dir}}."}, "claude")
		text, ok := cfg.SkillsInstructions()
		assert.True(t, ok)
		assert.Equal(t, "Use ai-rulez; files in .ai-rulez/skills.", text)
	})

	t.Run("switched off", func(t *testing.T) {
		_, ok := servingConfig(&MCPConfig{SkillsInstructions: &off}, "claude").SkillsInstructions()
		assert.False(t, ok)
	})

	t.Run("nothing served", func(t *testing.T) {
		cfg := &Config{Presets: []Preset{{BuiltIn: "claude"}}}
		_, ok := cfg.SkillsInstructions()
		assert.False(t, ok)
	})

	t.Run("no preset that can call MCP", func(t *testing.T) {
		_, ok := servingConfig(nil, "cline").SkillsInstructions()
		assert.False(t, ok)
	})
}

func TestSkillsInstructionsRespectsToolAllowlist(t *testing.T) {
	for _, tt := range []struct {
		name  string
		tools []string
		args  []string
		want  bool
	}{
		{"default tools", nil, nil, true},
		{"config omits load", []string{"find_skill"}, nil, false},
		{"config includes both", []string{"find_skill", "load_skill"}, nil, true},
		{"launcher omits find", nil, []string{"--tools=load_skill"}, false},
		{"launcher overrides config", []string{"find_skill"}, []string{"--tools", "find_skill", "--tools=load_skill"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := servingConfig(&MCPConfig{Tools: tt.tools}, "claude")
			cfg.MCPServers = map[string]*MCPServer{
				"skills": {Name: "skills", Command: "ai-rulez", Args: append([]string{"mcp", "--serve-skills"}, tt.args...)},
			}
			_, shown := cfg.SkillsInstructions()
			assert.Equal(t, tt.want, shown)
		})
	}
}

func TestValidateSkillsInstructions(t *testing.T) {
	off := false
	tests := []struct {
		name    string
		mcp     *MCPConfig
		wantErr string
	}{
		{"unset", &MCPConfig{}, ""},
		{"custom text", &MCPConfig{SkillsInstructionsText: "Call {{server}} ({{skills_dir}})"}, ""},
		{"text with the section off is inert", &MCPConfig{SkillsInstructions: &off, SkillsInstructionsText: "x"}, "skills_instructions_text requires"},
		{"blank text", &MCPConfig{SkillsInstructionsText: "  \n"}, "must not be blank"},
		{"unknown variable", &MCPConfig{SkillsInstructionsText: "Call {{srv}}"}, `unknown variable {{srv}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{MCP: tt.mcp}).validateMCP()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
