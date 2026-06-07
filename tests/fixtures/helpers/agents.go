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

// V4ModelByPresetConfigTOML exercises the per-preset model override chain:
// agent `<preset>_model` frontmatter wins, otherwise `defaults.model_by_preset`
// for that preset, otherwise the legacy `model:` field.
const V4ModelByPresetConfigTOML = `version = "4.0"
name = "model-by-preset-test"
description = "E2E coverage for per-preset model overrides"
gitignore = false
builtins = false

presets = ["claude", "copilot"]

[defaults.model_by_preset]
claude = "haiku"
copilot = "gpt-5"
`

// V4AgentWithPerPresetModels carries explicit per-preset model overrides plus a
// legacy `model:` field. The per-preset entries must win over the legacy field
// in their matching preset output.
const V4AgentWithPerPresetModels = `---
name: agent-overrides
description: Agent that pins a different model per preset
model: sonnet
claude_model: opus
copilot_model: gpt-4
tools:
  - Read
---

Pinned per-preset.
`

// V4AgentWithDefaultedModel carries no model frontmatter at all, so each preset
// must fall through to its `defaults.model_by_preset` entry.
const V4AgentWithDefaultedModel = `---
name: agent-defaulted
description: Agent with no model frontmatter
tools:
  - Read
---

Falls through to defaults.model_by_preset.
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
