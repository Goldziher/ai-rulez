package testutil

// Sample configurations for testing

const BasicConfig = `metadata:
  name: "Test Project"
  version: "1.0.0"
  description: "A test project for ai-rulez"

outputs:
  - file: "CLAUDE.md"

rules:
  - name: "Basic Rule"
    priority: 5
    content: "This is a basic rule for testing"

  - name: "High Priority Rule"  
    priority: 9
    content: "This is a high priority rule"

sections:
  - title: "Development Guidelines"
    priority: 8
    content: "Follow these guidelines for development"
`

const MinimalConfig = `metadata:
  name: "Minimal Project"

outputs:
  - file: "output.md"

rules:
  - name: "Only Rule"
    content: "Single rule content"
`

const ConfigWithAgents = `metadata:
  name: "Project with Agents"

outputs:
  - file: "CLAUDE.md"
  - path: ".claude/agents/"
    type: "agent"

rules:
  - name: "Code Quality"
    priority: 8
    content: "Maintain high code quality"

agents:
  - name: "code-reviewer"
    description: "Reviews code for quality and best practices"
    priority: 9
    tools: ["Read", "Edit", "Grep"]
    system_prompt: "You are a code reviewer focused on quality"
`

const ConfigWithTargets = `metadata:
  name: "Project with Targets"

outputs:
  - file: "frontend.md"
  - file: "backend.md"

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
  - file: "output.md"

rules:
  - name: "Local Rule"
    content: "This is a local rule"
`

const InvalidYAMLConfig = `metadata:
  name: "Invalid Config"
  invalid_yaml: [unclosed_list
outputs:
  - file: "test.md"
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
