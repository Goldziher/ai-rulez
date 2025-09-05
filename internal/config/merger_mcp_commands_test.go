package config_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestMergeMCPServers(t *testing.T) {
	base := &config.Config{
		MCPServers: []config.MCPServer{
			{Name: "server1", Command: "cmd1"},
			{Name: "server2", URL: "http://localhost:8080"},
		},
	}

	extension := &config.Config{
		MCPServers: []config.MCPServer{
			{Name: "server2", URL: "http://localhost:9090"},
			{Name: "server3", Command: "cmd3"},
		},
	}

	merged := config.MergeMCPServers(base.MCPServers, extension.MCPServers)

	assert.Len(t, merged, 3, "Should have 3 servers after merge")

	server1 := findMCPServer(merged, "server1")
	assert.NotNil(t, server1)
	assert.Equal(t, "cmd1", server1.Command)

	server2 := findMCPServer(merged, "server2")
	assert.NotNil(t, server2)
	assert.Equal(t, "http://localhost:9090", server2.URL)

	server3 := findMCPServer(merged, "server3")
	assert.NotNil(t, server3)
	assert.Equal(t, "cmd3", server3.Command)
}

func TestMergeMCPServers_EmptyBase(t *testing.T) {
	base := &config.Config{}
	extension := &config.Config{
		MCPServers: []config.MCPServer{
			{Name: "server1", Command: "cmd1"},
		},
	}

	merged := config.MergeMCPServers(base.MCPServers, extension.MCPServers)
	assert.Len(t, merged, 1)
	assert.Equal(t, "server1", merged[0].Name)
}

func TestMergeMCPServers_EmptyExtension(t *testing.T) {
	base := &config.Config{
		MCPServers: []config.MCPServer{
			{Name: "server1", Command: "cmd1"},
		},
	}
	extension := &config.Config{}

	merged := config.MergeMCPServers(base.MCPServers, extension.MCPServers)
	assert.Len(t, merged, 1)
	assert.Equal(t, "server1", merged[0].Name)
}

func TestMergeCommands(t *testing.T) {
	base := &config.Config{
		Commands: []config.Command{
			{Name: "cmd1", Description: "Command 1"},
			{Name: "cmd2", SystemPrompt: "System prompt 2"},
		},
	}

	extension := &config.Config{
		Commands: []config.Command{
			{Name: "cmd2", SystemPrompt: "New system prompt 2"},
			{Name: "cmd3", Description: "Command 3"},
		},
	}

	merged := config.MergeCommands(base.Commands, extension.Commands)

	assert.Len(t, merged, 3, "Should have 3 commands after merge")

	cmd1 := findCommand(merged, "cmd1")
	assert.NotNil(t, cmd1)
	assert.Equal(t, "Command 1", cmd1.Description)

	cmd2 := findCommand(merged, "cmd2")
	assert.NotNil(t, cmd2)
	assert.Equal(t, "New system prompt 2", cmd2.SystemPrompt)

	cmd3 := findCommand(merged, "cmd3")
	assert.NotNil(t, cmd3)
	assert.Equal(t, "Command 3", cmd3.Description)
}

func TestMergeCommands_EmptyBase(t *testing.T) {
	base := &config.Config{}
	extension := &config.Config{
		Commands: []config.Command{
			{Name: "cmd1", Description: "Command 1"},
		},
	}

	merged := config.MergeCommands(base.Commands, extension.Commands)
	assert.Len(t, merged, 1)
	assert.Equal(t, "cmd1", merged[0].Name)
}

func TestMergeCommands_EmptyExtension(t *testing.T) {
	base := &config.Config{
		Commands: []config.Command{
			{Name: "cmd1", Description: "Command 1"},
		},
	}
	extension := &config.Config{}

	merged := config.MergeCommands(base.Commands, extension.Commands)
	assert.Len(t, merged, 1)
	assert.Equal(t, "cmd1", merged[0].Name)
}

func findMCPServer(servers []config.MCPServer, name string) *config.MCPServer {
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i]
		}
	}
	return nil
}

func findCommand(commands []config.Command, name string) *config.Command {
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	return nil
}
