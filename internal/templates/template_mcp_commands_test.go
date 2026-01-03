package templates_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/stretchr/testify/assert"
)

func TestNewTemplateData_WithMCPServersAndCommands(t *testing.T) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		MCPServers: []config.MCPServer{
			{Name: "server1", Command: "cmd1"},
			{Name: "server2", URL: "http://localhost:8080"},
		},
		Commands: []config.Command{
			{Name: "cmd1", Description: "Command 1"},
			{Name: "cmd2", SystemPrompt: "System prompt 2"},
		},
	}

	data := templates.NewTemplateData(cfg, nil)

	assert.Equal(t, "Test Project", data.ProjectName)
	assert.Len(t, data.MCPServers, 2)
	assert.Equal(t, 2, data.MCPServerCount)
	assert.Len(t, data.Commands, 2)
	assert.Equal(t, 2, data.CommandCount)

	assert.Equal(t, "server1", data.MCPServers[0].Name)
	assert.Equal(t, "cmd1", data.Commands[0].Name)
}

func TestNewTemplateDataForOutput_WithMCPServersAndCommandsFiltering(t *testing.T) {
	cfg := &config.Config{
		MCPServers: []config.MCPServer{
			{Name: "server1", Command: "cmd1", Targets: []string{"output1"}},
			{Name: "server2", URL: "http://localhost:8080"},
		},
		Commands: []config.Command{
			{Name: "cmd1", Description: "Command 1"},
			{Name: "cmd2", SystemPrompt: "System prompt 2", Targets: []string{"output1"}},
		},
	}

	data := templates.NewTemplateDataForOutput(cfg, "output1", nil)
	assert.Len(t, data.MCPServers, 2, "Should include targeted and non-targeted servers")
	assert.Equal(t, 2, data.MCPServerCount)
	assert.Len(t, data.Commands, 2, "Should include targeted and non-targeted commands")
	assert.Equal(t, 2, data.CommandCount)

	data = templates.NewTemplateDataForOutput(cfg, "output2", nil)
	assert.Len(t, data.MCPServers, 1, "Should only include non-targeted servers")
	assert.Equal(t, "server2", data.MCPServers[0].Name)
	assert.Equal(t, 1, data.MCPServerCount)

	assert.Len(t, data.Commands, 1, "Should only include non-targeted commands")
	assert.Equal(t, "cmd1", data.Commands[0].Name)
	assert.Equal(t, 1, data.CommandCount)
}
