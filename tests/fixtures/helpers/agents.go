package fixtures

const ConfigWithAgents = `metadata:
  name: "Project with Agents"

presets:
  - claude

outputs:
  - path: "CLAUDE.md"
    template:
      type: builtin
      value: default
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
    
  - name: "test-writer"
    description: "Writes comprehensive tests"
    priority: high
    tools: ["Read", "Write", "Edit", "Bash"]
    system_prompt: "You write comprehensive test cases"
    targets: ["**/*_test.go", "**/*.test.ts"]
    
  - name: "doc-writer"
    description: "Documentation specialist"
    priority: medium
    tools: ["Read", "Write"]
    system_prompt: "You write clear documentation"
    targets: ["*.md", "docs/**"]
`

const ConfigWithAgentTemplates = `metadata:
  name: "Agent Templates Test"

presets:
  - claude

outputs:
  - path: ".claude/agents/"
    type: "agent"

agents:
  - name: "inline-agent"
    description: "Agent with inline template"
    template:
      type: inline
      value: |
        You are a specialized agent.
        Focus on: {{.Description}}
        
  - name: "file-agent"
    description: "Agent with file template"
    template:
      type: file
      value: ./agent-template.txt
      
  - name: "builtin-agent"
    description: "Agent with builtin template"
    template:
      type: builtin
      value: code-reviewer
`

const ConfigWithComplexAgents = `metadata:
  name: "Complex Agents Test"

presets:
  - claude

outputs:
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{{.Name}}_agent.md"

agents:
  - name: "multi-tool-agent"
    id: "multi-tool-001"
    description: "Agent with many tools"
    priority: critical
    tools: 
      - "Read"
      - "Write"
      - "Edit"
      - "Bash"
      - "Grep"
      - "WebSearch"
      - "TodoWrite"
    system_prompt: |
      You are a powerful agent with many capabilities.
      Use your tools wisely and efficiently.
    targets:
      - "src/**"
      - "tests/**"
      - "docs/**"
      
  - name: "disabled-agent"
    description: "This agent is disabled"
    enabled: false
    tools: ["Read"]
    system_prompt: "This agent should not appear in output"
`
