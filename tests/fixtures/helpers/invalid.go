package fixtures

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

const InvalidPriorityConfig = `metadata:
  name: "Invalid Priority"
  
outputs:
  - path: "output.md"
  
rules:
  - name: "Bad Priority Rule"
    content: "Rule with invalid priority"
    priority: "extreme"  # Invalid priority value
`

const InvalidTemplateTypeConfig = `metadata:
  name: "Invalid Template"
  
outputs:
  - path: "output.md"
    template:
      type: "unknown"  # Invalid template type
      value: "something"
`

const InvalidTargetsConfig = `metadata:
  name: "Invalid Targets"
  
outputs:
  - path: "output.md"
  
named_targets:
    123invalid: ["*.go"]  # Invalid named target (starts with number)
    
rules:
  - name: "Bad Target Rule"
    content: "Rule with bad target"
    targets: ["@nonexistent"]  # References non-existent named target
`

const CircularExtendsConfig = `metadata:
  name: "Circular Reference"
  
extends: "./circular.yaml"  # Points to itself

outputs:
  - path: "output.md"
`

const MissingIncludesConfig = `metadata:
  name: "Missing Includes"
  
includes:
  - "./does-not-exist.yaml"
  - "https://example.com/404.yaml"
  
outputs:
  - path: "output.md"
`

const InvalidMCPServerConfig = `metadata:
  name: "Invalid MCP Server"
  
outputs:
  - path: ".mcp.json"
  
mcp_servers:
  - name: "no-transport"
    description: "Missing transport"
    command: "test"
    # Missing required transport field
    
  - name: "http-no-url"
    description: "HTTP without URL"
    transport: "http"
    # Missing required URL for HTTP transport
    
  - name: "stdio-with-url"
    description: "STDIO with URL"
    command: "test"
    transport: "stdio"
    url: "http://invalid.com"  # URL not allowed with stdio transport
`

const InvalidCommandConfig = `metadata:
  name: "Invalid Commands"
  
outputs:
  - path: "output.md"
  
commands:
  - # Missing required name field
    description: "No name command"
    
  - name: ""  # Empty name
    description: "Empty name command"
    
  - name: "duplicate"
    description: "First duplicate"
    
  - name: "duplicate"  # Duplicate command name
    description: "Second duplicate"
`

const InvalidAgentConfig = `metadata:
  name: "Invalid Agents"
  
outputs:
  - path: ".claude/agents/"
    type: "agent"
  
agents:
  - # Missing required name field
    description: "No name agent"
    
  - name: "invalid-tools"
    description: "Agent with invalid tools"
    tools: ["InvalidTool", "AnotherBadTool"]  # Non-existent tools
    
  - name: "bad-priority"
    description: "Agent with bad priority"
    priority: 999  # Invalid priority value
`
