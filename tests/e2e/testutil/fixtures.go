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

// V3 Configuration Fixtures
const V3BasicConfigYAML = `version: "3.0"
name: "v3-test-project"
description: "Basic V3 test configuration"
presets:
  - claude
gitignore: false
`

const V3BasicRuleMarkdown = `---
priority: high
---

# Basic Rule

This is a basic rule for testing V3 configuration.
`

const V3HighPriorityRuleMarkdown = `---
priority: critical
---

# High Priority Rule

This is a high priority rule for testing.
`

const V3ProjectContextMarkdown = `# Project Information

This is a test project for validating V3 MCP operations.
`

const V3ConfigWithMCPServersYAML = `version: "3.0"
name: "v3-mcp-test-project"
description: "V3 configuration with MCP servers"
presets:
  - claude
gitignore: false
`

const V3MCPServerConfigYAML = `version: "1.0"
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

const V3ConfigWithMultiplePresetsYAML = `version: "3.0"
name: "v3-multi-preset-project"
description: "V3 with multiple presets"
presets:
  - claude
  - cursor
gitignore: false
`

const V3InvalidConfigYAML = `version: "3.0"
name: "v3-invalid-project"
description: "This is invalid"
presets: not-a-list
`
