package testutil

// Sample configurations for testing

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

// Provider configurations for init tests
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
