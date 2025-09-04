package testutil

const BasicConfig = `metadata:
  name: "Test Project"
  version: "1.0.0"
  description: "A test project for ai-rulez"

outputs:
  - path: "CLAUDE.md"

rules:
  - name: "Basic Rule"
    priority: medium
    content: "This is a basic rule for testing"

  - name: "High Priority Rule"  
    priority: critical
    content: "This is a high priority rule"

sections:
  - name: "Development Guidelines"
    priority: high
    content: "Follow these guidelines for development"
`

const MinimalConfig = `metadata:
  name: "Minimal Project"

outputs:
  - path: "output.md"

rules:
  - name: "Only Rule"
    content: "Single rule content"
`

const ConfigWithAgents = `metadata:
  name: "Project with Agents"

outputs:
  - path: "CLAUDE.md"
  - path: ".claude/agents/"
    type: "agent"

rules:
  - name: "Code Quality"
    priority: high
    content: "Maintain high code quality"

agents:
  - name: "code-reviewer"
    description: "Reviews code for quality and best practices"
    priority: critical
    tools: ["Read", "Edit", "Grep"]
    system_prompt: "You are a code reviewer focused on quality"
`

const ConfigWithTargets = `metadata:
  name: "Project with Targets"

outputs:
  - path: "frontend.md"
  - path: "backend.md"

rules:
  - name: "Frontend Rule"
    content: "Frontend specific rule"
    targets: ["frontend.md"]

  - name: "Backend Rule"
    content: "Backend specific rule"
    targets: ["backend.md"]

  - name: "Universal Rule"
    content: "Rule for everyone"
`

const ConfigWithRemoteIncludes = `metadata:
  name: "Project with Remote Includes"

includes:
  - "https://raw.githubusercontent.com/example/standards/main/base.yaml"

outputs:
  - path: "output.md"

rules:
  - name: "Local Rule"
    content: "This is a local rule"
`

const InvalidYAMLConfig = `metadata:
  name: "Invalid Config"
  invalid_yaml: [unclosed_list
outputs:
  - path: "test.md"
`

const InvalidSchemaConfig = `metadata:
  # Missing required 'name' field
  version: "1.0.0"

# Missing required 'outputs' field

rules:
  - name: "Rule without content"
    # Missing required 'content' field
`

const ConfigWithMCPServers = `metadata:
  name: "MCP Server Test Project"
  description: "Testing MCP server generation"

outputs:
  - path: ".mcp.json"
    template: "claude-code-mcp"
  - path: ".cursor/mcp.json" 
    template: "cursor-mcp"
  - path: "mcp_config.json"
    template: "windsurf-mcp"
  - path: ".vscode/mcp.json"
    template: "vscode-mcp"
  - path: ".continue/mcpServers/servers.yaml"
    template: "continuedev-mcp"
  - path: "cline_mcp_settings.json"
    template: "cline-mcp"

mcp_servers:
  - name: "github"
    description: "GitHub integration"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
    transport: "stdio"
    enabled: true
    
  - name: "postgres"  
    description: "PostgreSQL database"
    command: "uvx"
    args: ["mcp-server-postgres", "--connection-string", "postgresql://user:pass@localhost/db"]
    
  - name: "remote-api"
    description: "Remote API server"
    url: "https://api.example.com/mcp"
    transport: "http"
    enabled: true
    
  - name: "disabled-server"
    description: "Disabled server"
    command: "python"
    args: ["server.py"]
    enabled: false

rules:
  - name: "Test Rule"
    content: "Test content for MCP servers"
`

const ConfigWithCommands = `metadata:
  name: "Commands Test Project"
  description: "Testing custom commands"

outputs:
  - path: "commands-output.md"
    template:
      type: "inline"
      value: |
        # {{.ProjectName}} Commands
        
        {{if .Commands}}
        ## Available Commands
        {{range .Commands}}
        ### /{{.Name}}{{if .Aliases}} (aliases: {{range .Aliases}}/{{.}} {{end}}){{end}}
        {{.Description}}
        {{if .Usage}}**Usage:** {{.Usage}}{{end}}
        {{if .SystemPrompt}}**System Prompt:** {{.SystemPrompt}}{{end}}
        {{if .Shortcut}}**Shortcut:** {{.Shortcut}}{{end}}
        - Enabled: {{.IsEnabled}}
        {{end}}
        {{end}}

commands:
  - name: "newtask"
    description: "Start a new task with fresh context"
    usage: "/newtask <description>"
    system_prompt: "You are starting a new focused task"
    enabled: true
    
  - name: "smol"
    aliases: ["compact", "summarize"]
    description: "Condense chat history"
    usage: "/smol"
    system_prompt: "Summarize conversation concisely"
    shortcut: "Ctrl+Shift+S"
    
  - name: "review"
    description: "Request code review"
    usage: "/review [file]"
    system_prompt: "Focus on code quality and best practices"
    enabled: false

rules:
  - name: "Command Rule"
    content: "Use commands appropriately"
`

const ConfigWithMCPAndCommands = `metadata:
  name: "Full Feature Test"
  description: "Testing both MCP servers and commands"

outputs:
  - path: "full-output.md"
    
mcp_servers:
  - name: "test-server"
    description: "Test MCP server"
    command: "test"
    args: ["--mode", "test"]
    
commands:
  - name: "test"
    description: "Test command"
    usage: "/test"

rules:
  - name: "Integration Rule"
    content: "Integration test content"
`

var ClaudeProviderOutputs = []string{"CLAUDE.md", ".claude/agents/"}
var CursorProviderOutputs = []string{".cursor/rules/"}
var WindsurfProviderOutputs = []string{".windsurf/"}
var CopilotProviderOutputs = []string{".github/copilot-instructions.md"}
var AllProviderOutputs = []string{
	"CLAUDE.md", ".claude/agents/",
	".cursor/rules/", ".windsurf/",
	".github/copilot-instructions.md",
	"GEMINI.md", "AGENTS.md", ".clinerules/", ".continue/rules/",
}
