package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// The MCP line follows [mcp] instead of a hardcoded npx launch (#360).
func TestHeader_MCPLineFollowsSelfServer(t *testing.T) {
	t.Parallel()

	off := false
	serving := []config.SkillSourceConfig{{Name: "vendor", URL: "vendor-skills"}}

	tests := []struct {
		name string
		cfg  func(style string) *config.Config
		want string // substring that must appear, "" when no MCP line is expected
	}{
		{
			name: "self_server_command with served skills",
			cfg: func(style string) *config.Config {
				return &config.Config{
					Name: "P", Header: &config.HeaderConfig{Style: style}, SkillSources: serving,
					MCP: &config.MCPConfig{SelfServerCommand: []string{"ai-rulez", "mcp"}},
				}
			},
			want: "ai-rulez mcp --serve-skills",
		},
		{
			name: "pinned version",
			cfg: func(style string) *config.Config {
				return &config.Config{Name: "P", Header: &config.HeaderConfig{Style: style}, MCP: &config.MCPConfig{SelfServerVersion: "4.1.0"}}
			},
			want: "npx -y ai-rulez@4.1.0 mcp",
		},
		{
			name: "self_server off",
			cfg: func(style string) *config.Config {
				return &config.Config{Name: "P", Header: &config.HeaderConfig{Style: style}, MCP: &config.MCPConfig{SelfServer: &off}}
			},
		},
		{
			name: "mcp_line off",
			cfg: func(style string) *config.Config {
				return &config.Config{Name: "P", Header: &config.HeaderConfig{Style: style, MCPLine: &off}}
			},
		},
	}
	for _, tt := range tests {
		for _, style := range []string{"detailed", "compact", "minimal"} {
			t.Run(tt.name+"/"+style, func(t *testing.T) {
				t.Parallel()
				header := templates.GenerateHeader(createTemplateData("P", tt.cfg(style)))
				if tt.want != "" {
					assert.Contains(t, header, tt.want)
					assert.NotContains(t, header, "ai-rulez@latest mcp\n")
					return
				}
				assert.NotContains(t, header, "MCP")
				assert.NotContains(t, header, "npx")
			})
		}
	}
}

func TestHeader_DetailedStepsStayNumberedWithoutMCPLine(t *testing.T) {
	t.Parallel()

	off := false
	header := templates.GenerateHeader(createTemplateData("P",
		&config.Config{Name: "P", Header: &config.HeaderConfig{Style: "detailed", MCPLine: &off}}))
	assert.Contains(t, header, "3. After making changes: ai-rulez generate")
	assert.Contains(t, header, "4. Complete workflow:")
	assert.NotContains(t, header, "5. ")
}
