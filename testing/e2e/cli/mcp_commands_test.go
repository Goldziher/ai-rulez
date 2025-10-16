package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type MCPCommandsCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestMCPCommandsCLISuite(t *testing.T) {
	suite.Run(t, new(MCPCommandsCLITestSuite))
}

func (s *MCPCommandsCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *MCPCommandsCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *MCPCommandsCLITestSuite) TestGenerateWithInvalidTemplate() {
	invalidTemplateContent := `Hello, {{ .ProjectName `
	testutil.WriteFile(s.T(), s.workingDir, "broken.tmpl", invalidTemplateContent)

	configContent := `
metadata:
  name: "Invalid Template Test"
outputs:
  - path: "output.txt"
    template:
      type: builtin
      value: broken.tmpl
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", configContent)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), "execute template")
	result.AssertStderrContains(s.T(), "broken.tmpl")
}

func (s *MCPCommandsCLITestSuite) TestGenerateMCPServers() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithMCPServers)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated 6 file(s)")

	s.verifyClaudeCodeMCPFormat()

	s.verifyCursorMCPFormat()

	s.verifyWindsurfMCPFormat()

	s.verifyVSCodeMCPFormat()

	s.verifyContinueDevMCPFormat()

	s.verifyClineMCPFormat()
}

func (s *MCPCommandsCLITestSuite) verifyClaudeCodeMCPFormat() {
	path := filepath.Join(s.workingDir, ".mcp.json")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)

	var config map[string]interface{}
	err := json.Unmarshal([]byte(content), &config)
	s.NoError(err, "Should be valid JSON")

	github, exists := config["github"].(map[string]interface{})
	s.True(exists, "Should have github server")
	s.Equal("stdio", github["type"], "Should have type field")
	s.Equal("npx", github["command"], "Should have correct command")

	args, ok := github["args"].([]interface{})
	s.True(ok && len(args) == 2, "Should have correct args")
	s.Equal("-y", args[0])
	s.Equal("@modelcontextprotocol/server-github", args[1])

	env, ok := github["env"].(map[string]interface{})
	s.True(ok, "Should have env section")
	s.Equal("${GITHUB_TOKEN}", env["GITHUB_PERSONAL_ACCESS_TOKEN"])

	remoteAPI, exists := config["remote-api"].(map[string]interface{})
	s.True(exists, "Should have remote-api server")
	s.Equal("http", remoteAPI["type"], "Should have http transport")
	s.Equal("https://api.example.com/mcp", remoteAPI["url"], "Should have URL for HTTP transport")
}

func (s *MCPCommandsCLITestSuite) verifyCursorMCPFormat() {
	path := filepath.Join(s.workingDir, ".cursor", "mcp.json")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)

	var config map[string]interface{}
	err := json.Unmarshal([]byte(content), &config)
	s.NoError(err, "Should be valid JSON")

	mcpServers, exists := config["McpServers"].(map[string]interface{})
	s.True(exists, "Should have McpServers wrapper (capitalized)")

	github, exists := mcpServers["github"].(map[string]interface{})
	s.True(exists, "Should have github server")
	s.Equal("npx", github["command"], "Should have command")

	remoteAPI, exists := mcpServers["remote-api"].(map[string]interface{})
	s.True(exists, "Should have remote-api server")
	s.Equal("https://api.example.com/mcp", remoteAPI["url"], "Should have URL for remote server")
	_, hasCommand := remoteAPI["command"]
	s.False(hasCommand, "Remote servers should not have command field in Cursor format")
}

func (s *MCPCommandsCLITestSuite) verifyWindsurfMCPFormat() {
	path := filepath.Join(s.workingDir, "mcp_config.json")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)

	var config map[string]interface{}
	err := json.Unmarshal([]byte(content), &config)
	s.NoError(err, "Should be valid JSON")

	mcpServers, exists := config["mcpServers"].(map[string]interface{})
	s.True(exists, "Should have mcpServers wrapper (camelCase)")

	github, exists := mcpServers["github"].(map[string]interface{})
	s.True(exists, "Should have github server")
	s.Equal("npx", github["command"], "Should have command")
	s.Equal("stdio", github["transport"], "Should declare transport for stdio servers")

	remoteAPI, exists := mcpServers["remote-api"].(map[string]interface{})
	s.True(exists, "Should include remote servers")
	s.Equal("http", remoteAPI["transport"])
	s.Equal("https://api.example.com/mcp", remoteAPI["url"])
	_, hasRemoteCommand := remoteAPI["command"]
	s.False(hasRemoteCommand, "HTTP windsuf servers should not emit command field")

	disabledServer, exists := mcpServers["disabled-server"].(map[string]interface{})
	s.True(exists, "Should include disabled server")
	s.Equal(true, disabledServer["disabled"])

	s.NotNil(mcpServers, "Should have mcpServers structure")
}

func (s *MCPCommandsCLITestSuite) verifyVSCodeMCPFormat() {
	path := filepath.Join(s.workingDir, ".vscode", "mcp.json")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)

	var config map[string]interface{}
	err := json.Unmarshal([]byte(content), &config)
	s.NoError(err, "Should be valid JSON")

	servers, exists := config["servers"].(map[string]interface{})
	s.True(exists, "Should have servers wrapper")

	github, exists := servers["github"].(map[string]interface{})
	s.True(exists, "Should have github server")
	s.Equal("stdio", github["type"], "Should have type field")
	s.Equal("npx", github["command"], "Should have command")
}

func (s *MCPCommandsCLITestSuite) verifyContinueDevMCPFormat() {
	path := filepath.Join(s.workingDir, ".continue", "mcpServers", "servers.yaml")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)

	var config map[string]interface{}
	err := yaml.Unmarshal([]byte(content), &config)
	s.NoError(err, "Should be valid YAML")

	s.Equal("MCP Server Test Project MCP Configuration", config["name"])
	s.Equal("1.0.0", config["version"])
	s.Equal("v1", config["schema"])

	mcpServers, exists := config["mcpServers"].([]interface{})
	s.True(exists, "Should have mcpServers array")
	s.True(len(mcpServers) > 0, "Should have at least one server")

	var githubServer map[string]interface{}
	for _, server := range mcpServers {
		if serverMap, ok := server.(map[string]interface{}); ok {
			if serverMap["name"] == "github" {
				githubServer = serverMap
				break
			}
		}
	}
	s.NotNil(githubServer, "Should find GitHub server")
	s.Equal("GitHub integration", githubServer["description"])
	s.Equal("npx", githubServer["command"])

	args, ok := githubServer["args"].([]interface{})
	s.True(ok && len(args) == 2, "Should have args array")

	var remoteServer map[string]interface{}
	for _, server := range mcpServers {
		if serverMap, ok := server.(map[string]interface{}); ok {
			if serverMap["name"] == "remote-api" {
				remoteServer = serverMap
				break
			}
		}
	}
	s.NotNil(remoteServer, "Should find remote server")
	s.Equal("streamable-http", remoteServer["type"], "Should map http to streamable-http for Continue.dev")
	s.Equal("https://api.example.com/mcp", remoteServer["url"])
}

func (s *MCPCommandsCLITestSuite) verifyClineMCPFormat() {
	path := filepath.Join(s.workingDir, "cline_mcp_settings.json")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)

	var config map[string]interface{}
	err := json.Unmarshal([]byte(content), &config)
	s.NoError(err, "Should be valid JSON")

	mcpServers, exists := config["mcpServers"].(map[string]interface{})
	s.True(exists, "Should have mcpServers wrapper")

	github, exists := mcpServers["github"].(map[string]interface{})
	s.True(exists, "Should have github server")
	s.Equal("npx", github["command"], "Should have command")
	s.Equal(false, github["disabled"], "Should have disabled field set to false")

	remoteServer, exists := mcpServers["remote-api"].(map[string]interface{})
	s.True(exists, "Should include remote server for Cline")
	s.Equal("https://api.example.com/mcp", remoteServer["url"])
	s.Equal("http", remoteServer["transport"])

	disabledServer, exists := mcpServers["disabled-server"].(map[string]interface{})
	s.True(exists, "Should have disabled server")
	s.Equal(true, disabledServer["disabled"], "Should have disabled field set to true")
}

func (s *MCPCommandsCLITestSuite) TestGenerateCommands() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithCommands)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated")

	path := filepath.Join(s.workingDir, "commands-output.md")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)
	s.Contains(content, "Commands Test Project Commands")
	s.Contains(content, "## Available Commands")

	s.Contains(content, "### /newtask")
	s.Contains(content, "Start a new task with fresh context")
	s.Contains(content, "**Usage:** /newtask <description>")
	s.Contains(content, "**System Prompt:** You are starting a new focused task")
	s.Contains(content, "- Enabled: true")

	s.Contains(content, "### /smol (aliases: /compact /summarize )")
	s.Contains(content, "Condense chat history")
	s.Contains(content, "**Shortcut:** Ctrl+Shift+S")

	s.Contains(content, "### /review")
	s.Contains(content, "- Enabled: false")
}

func (s *MCPCommandsCLITestSuite) TestValidateMCPAndCommands() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithMCPAndCommands)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "✅ Configuration file is valid")

	s.True(result.Stdout != "" || result.Stderr != "", "Should have some output")
}

func (s *MCPCommandsCLITestSuite) TestInvalidMCPConfiguration() {
	invalidConfig := `metadata:
  name: "Invalid MCP Config"

outputs:
  - path: "test.json"
    template:
      type: builtin
      value: claude-code-mcp

mcp_servers:
  - name: "invalid-server"
    # Missing required command for stdio transport
    args: ["test"]
    transport: "stdio"
    
rules:
  - name: "Test Rule"
    content: "Test content"
`

	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", invalidConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "✅ Configuration file is valid")
}

func (s *MCPCommandsCLITestSuite) TestInvalidCommandConfiguration() {
	invalidConfig := `metadata:
  name: "Invalid Command Config"

outputs:
  - path: "test.md"

commands:
  - name: "123invalid"  # Invalid name pattern
    description: "Invalid command name"
    
rules:
  - name: "Test Rule"
    content: "Test content"
`

	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", invalidConfig)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "validation failed")
}

func (s *MCPCommandsCLITestSuite) TestMCPAndCommandsWithTargets() {
	configWithTargets := `metadata:
  name: "Targeted MCP Test"

outputs:
  - path: "claude-mcp.json"
    template:
      type: builtin
      value: claude-code-mcp
  - path: "cursor-mcp.json" 
    template:
      type: builtin
      value: cursor-mcp

mcp_servers:
  - name: "claude-only-server"
    description: "Server for Claude only"
    command: "claude-server"
    targets: ["claude-mcp.json"]
    
  - name: "cursor-only-server"
    description: "Server for Cursor only"  
    command: "cursor-server"
    targets: ["cursor-mcp.json"]
    
  - name: "universal-server"
    description: "Server for all targets"
    command: "universal-server"

commands:
  - name: "claude-cmd"
    description: "Claude specific command"
    targets: ["claude-mcp.json"]
    
  - name: "universal-cmd"
    description: "Universal command"

rules:
  - name: "Test Rule"
    content: "Test content"
`

	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", configWithTargets)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated 2 file(s)")

	claudePath := filepath.Join(s.workingDir, "claude-mcp.json")
	s.True(testutil.FileExists(s.T(), claudePath))
	claudeContent := testutil.ReadFile(s.T(), claudePath)

	var claudeConfig map[string]interface{}
	err := json.Unmarshal([]byte(claudeContent), &claudeConfig)
	s.NoError(err)

	s.Contains(claudeContent, "claude-only-server")
	s.Contains(claudeContent, "universal-server")
	s.NotContains(claudeContent, "cursor-only-server")

	cursorPath := filepath.Join(s.workingDir, "cursor-mcp.json")
	s.True(testutil.FileExists(s.T(), cursorPath))
	cursorContent := testutil.ReadFile(s.T(), cursorPath)

	var cursorConfig map[string]interface{}
	err = json.Unmarshal([]byte(cursorContent), &cursorConfig)
	s.NoError(err)

	mcpServers := cursorConfig["McpServers"].(map[string]interface{})

	_, hasCursorServer := mcpServers["cursor-only-server"]
	_, hasUniversalServer := mcpServers["universal-server"]
	_, hasClaudeServer := mcpServers["claude-only-server"]

	s.True(hasCursorServer, "Should have cursor-only-server")
	s.True(hasUniversalServer, "Should have universal-server")
	s.False(hasClaudeServer, "Should not have claude-only-server")
}

func (s *MCPCommandsCLITestSuite) TestTemplateCounts() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithMCPAndCommands)

	customConfig := `metadata:
  name: "Count Test"

outputs:
  - path: "counts.md"
    template:
      type: "inline"
      value: |
        # Counts Test
        - MCP Servers: {{.MCPServerCount}}
        - Commands: {{.CommandCount}}
        - Rules: {{.RuleCount}}

mcp_servers:
  - name: "server1"
    command: "test1"
  - name: "server2"
    command: "test2"

commands:
  - name: "cmd1"
    description: "Command 1"
  - name: "cmd2"  
    description: "Command 2"
  - name: "cmd3"
    description: "Command 3"

rules:
  - name: "Rule1"
    content: "Content1"
`

	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", customConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	path := filepath.Join(s.workingDir, "counts.md")
	s.True(testutil.FileExists(s.T(), path))

	content := testutil.ReadFile(s.T(), path)
	s.Contains(content, "- MCP Servers: 2")
	s.Contains(content, "- Commands: 3")
	s.Contains(content, "- Rules: 1")
}

func TestMCPCommandsCLITestSuite(t *testing.T) {
	suite.Run(t, new(MCPCommandsCLITestSuite))
}
