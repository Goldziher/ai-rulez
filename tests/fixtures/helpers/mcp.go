package fixtures

const ConfigWithMCPServers = `metadata:
  name: "MCP Server Test Project"
  description: "Testing MCP server generation"

presets:
  - claude

outputs:
  - path: ".mcp.json"
    template:
      type: builtin
      value: claude-code-mcp
  - path: ".cursor/mcp.json" 
    template:
      type: builtin
      value: cursor-mcp
  - path: "mcp_config.json"
    template:
      type: builtin
      value: windsurf-mcp
  - path: ".vscode/mcp.json"
    template:
      type: builtin
      value: vscode-mcp
  - path: ".continue/mcpServers/servers.yaml"
    template:
      type: builtin
      value: continuedev-mcp
  - path: "cline_mcp_settings.json"
    template:
      type: builtin
      value: cline-mcp

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

presets:
  - claude

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

presets:
  - claude

outputs:
  - path: "full-output.md"
  - path: ".mcp.json"
    template:
      type: builtin
      value: claude-code-mcp
    
mcp_servers:
  - name: "test-server"
    description: "Test MCP server"
    command: "test"
    args: ["--mode", "test"]
    transport: "stdio"
    
  - name: "http-server"
    description: "HTTP MCP server"
    url: "http://localhost:3000/mcp"
    transport: "http"
    
commands:
  - name: "test"
    description: "Test command"
    usage: "/test"
    
  - name: "debug"
    aliases: ["dbg"]
    description: "Debug mode"
    usage: "/debug [on|off]"
    system_prompt: "Enable detailed debugging information"

rules:
  - name: "Integration Rule"
    content: "Integration test content"
`
