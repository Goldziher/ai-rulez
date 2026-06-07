package testutil

import (
	helpers "github.com/Goldziher/ai-rulez/tests/fixtures/helpers"
)

const BasicConfig = helpers.BasicConfig
const MinimalConfig = helpers.MinimalConfig
const ConfigWithAgents = helpers.ConfigWithAgents
const ConfigWithTargets = helpers.ConfigWithTargets
const InvalidYAMLConfig = helpers.InvalidYAMLConfig
const InvalidSchemaConfig = helpers.InvalidSchemaConfig
const ConfigWithMCPServers = helpers.ConfigWithMCPServers
const ConfigWithCommands = helpers.ConfigWithCommands
const ConfigWithMCPAndCommands = helpers.ConfigWithMCPAndCommands
const V4ModelByPresetConfigTOML = helpers.V4ModelByPresetConfigTOML
const V4AgentWithPerPresetModels = helpers.V4AgentWithPerPresetModels
const V4AgentWithDefaultedModel = helpers.V4AgentWithDefaultedModel

// Configuration Fixtures
const BasicConfigYAML = `version: "4.0"
name: "test-project"
description: "Basic test configuration"
presets:
  - claude
gitignore: false
`

const BasicRuleMarkdown = `---
priority: high
---

# Basic Rule

This is a basic rule for testing configuration.
`

const HighPriorityRuleMarkdown = `---
priority: critical
---

# High Priority Rule

This is a high priority rule for testing.
`

const ProjectContextMarkdown = `# Project Information

This is a test project for validating MCP operations.
`

const ConfigWithMCPServersYAML = `version: "4.0"
name: "mcp-test-project"
description: "configuration with MCP servers"
presets:
  - claude
gitignore: false
mcp_servers:
  - name: test-server
    command: python
    args:
      - "-m"
      - test_server
    transport: stdio
    enabled: true
    env:
      DEBUG: "true"
  - name: github-server
    command: npx
    args:
      - "-y"
      - "@modelcontextprotocol/server-github"
    transport: stdio
    enabled: false
    env:
      GITHUB_TOKEN: "${GITHUB_TOKEN}"
`

const ConfigWithMultiplePresetsYAML = `version: "4.0"
name: "multi-preset-project"
description: "Multiple preset configuration"
presets:
  - claude
  - cursor
gitignore: false
`

const InvalidConfigYAML = `version: "4.0"
name: "invalid-project"
description: "This is invalid"
presets: not-a-list
`
