package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// V4 TOML Configuration Fixtures

const V4FullConfigTOML = `version = "4.0"
name = "v4-test-project"
description = "Full V4 test configuration with all presets"
gitignore = false
default = "full"
builtins = false

presets = [
  "claude", "cursor", "windsurf", "copilot", "gemini",
  "cline", "junie", "continue-dev", "codex", "opencode",
  "amp", "antigravity", "mcp"
]

[header]
style = "compact"

[profiles]
full = ["backend", "frontend"]
backend = ["backend"]

[[plugins]]
marketplace = "claude-plugins-official"
name = "github"
scope = "project"
enabled = true

[[plugins]]
marketplace = "openai-curated"
name = "gmail"
scope = "user"
enabled = true

[[plugins]]
marketplace = "my-org/team-plugins"
name = "internal-tool"
enabled = true

[[marketplaces]]
name = "team-tools"
source = "my-org/claude-plugins"
type = "github"

[[marketplaces]]
name = "local-dev"
source = "./plugins"
type = "local"

# MCP Servers
[[mcp_servers]]
name = "test-mcp-server"
description = "Test MCP server for integration tests"
command = "npx"
args = ["-y", "@test/mcp-server"]
transport = "stdio"
enabled = true

[mcp_servers.env]
API_KEY = "test-key"
DEBUG = "true"

[[mcp_servers]]
name = "http-mcp-server"
description = "HTTP transport MCP server"
command = "python"
args = ["-m", "http_server"]
transport = "http"
url = "http://localhost:8080"
enabled = true
`

// Content fixtures

const V4RuleHighPriority = `---
priority: critical
---

# Code Review Standards

All code must be reviewed before merging. Require at least one approval.
`

const V4RuleWithTargets = `---
priority: medium
targets:
  - CLAUDE.md
  - .cursor/rules/*
---

# Documentation Standards

Keep documentation up to date with code changes.
`

const V4ContextFile = `# Project Architecture

This project uses a hexagonal architecture with ports and adapters.
The core domain logic is isolated from infrastructure concerns.
`

const V4SkillFile = `---
name: deployment-workflow
description: Handles deployment workflows for staging and production
priority: high
---

# Deployment Workflow

Follow this workflow when deploying:
1. Run tests
2. Build artifacts
3. Deploy to staging
4. Verify staging
5. Deploy to production
`

const V4AgentFile = `---
name: security-reviewer
description: Reviews code for security vulnerabilities
model: sonnet
tools:
  - Read
  - Grep
  - Glob
---

You are a security reviewer. Analyze code for OWASP top 10 vulnerabilities.
Focus on injection, authentication, and data exposure issues.
`

const V4CommandFile = `---
description: Run the full test suite with coverage
usage: /run-tests [--coverage]
aliases:
  - test
  - check
targets:
  - claude
  - cursor
  - copilot
  - codex
  - continue-dev
---

Run the project's full test suite. If --coverage is specified, generate a coverage report.

Steps:
1. Run unit tests
2. Run integration tests
3. Generate coverage report if requested
`

// Domain content fixtures

const V4DomainBackendRule = `---
priority: high
---

# Backend API Standards

All API endpoints must have input validation and error handling.
Use structured logging for all request/response cycles.
`

const V4DomainBackendContext = `# Backend Architecture

The backend uses Go with Chi router and PostgreSQL.
Service layer handles business logic, repository layer handles data access.
`

const V4DomainBackendSkill = `---
name: api-design
description: Guides API endpoint design and implementation
---

# API Design Skill

When designing new API endpoints:
- Use RESTful conventions
- Include OpenAPI documentation
- Add rate limiting for public endpoints
`

const V4DomainBackendAgent = `---
name: backend-architect
description: Designs backend service architecture
model: opus
---

You are a backend architect. Design scalable service architectures
following domain-driven design principles.
`

const V4DomainBackendCommand = `---
description: Generate a new API endpoint scaffold
usage: /scaffold-endpoint <name>
targets:
  - claude
  - cursor
---

Generate boilerplate for a new API endpoint including handler, service, and repository layers.
`

const V4DomainFrontendRule = `---
priority: medium
---

# Frontend Component Standards

Use functional components with hooks. Keep components small and focused.
`

// SetupV4FullConfig creates a complete V4 configuration directory structure
// with all content types, domains, and TOML config for comprehensive testing.
func SetupV4FullConfig(t *testing.T, workingDir string) {
	t.Helper()

	aiRulesDir := filepath.Join(workingDir, ".ai-rulez")

	// Create directory structure
	dirs := []string{
		aiRulesDir,
		filepath.Join(aiRulesDir, "rules"),
		filepath.Join(aiRulesDir, "context"),
		filepath.Join(aiRulesDir, "skills", "deployment-workflow"),
		filepath.Join(aiRulesDir, "agents"),
		filepath.Join(aiRulesDir, "commands"),
		filepath.Join(aiRulesDir, "domains", "backend", "rules"),
		filepath.Join(aiRulesDir, "domains", "backend", "context"),
		filepath.Join(aiRulesDir, "domains", "backend", "skills", "api-design"),
		filepath.Join(aiRulesDir, "domains", "backend", "agents"),
		filepath.Join(aiRulesDir, "domains", "backend", "commands"),
		filepath.Join(aiRulesDir, "domains", "frontend", "rules"),
	}

	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err, "Failed to create directory: %s", dir)
	}

	// Write config files
	WriteFile(t, aiRulesDir, "config.toml", V4FullConfigTOML)

	// Write root content
	WriteFile(t, filepath.Join(aiRulesDir, "rules"), "code-review-standards.md", V4RuleHighPriority)
	WriteFile(t, filepath.Join(aiRulesDir, "rules"), "documentation-standards.md", V4RuleWithTargets)
	WriteFile(t, filepath.Join(aiRulesDir, "context"), "project-architecture.md", V4ContextFile)
	WriteFile(t, filepath.Join(aiRulesDir, "skills", "deployment-workflow"), "SKILL.md", V4SkillFile)
	WriteFile(t, filepath.Join(aiRulesDir, "agents"), "security-reviewer.md", V4AgentFile)
	WriteFile(t, filepath.Join(aiRulesDir, "commands"), "run-tests.md", V4CommandFile)

	// Write backend domain content
	WriteFile(t, filepath.Join(aiRulesDir, "domains", "backend", "rules"), "api-standards.md", V4DomainBackendRule)
	WriteFile(t, filepath.Join(aiRulesDir, "domains", "backend", "context"), "backend-architecture.md", V4DomainBackendContext)
	WriteFile(t, filepath.Join(aiRulesDir, "domains", "backend", "skills", "api-design"), "SKILL.md", V4DomainBackendSkill)
	WriteFile(t, filepath.Join(aiRulesDir, "domains", "backend", "agents"), "backend-architect.md", V4DomainBackendAgent)
	WriteFile(t, filepath.Join(aiRulesDir, "domains", "backend", "commands"), "scaffold-endpoint.md", V4DomainBackendCommand)

	// Write frontend domain content
	WriteFile(t, filepath.Join(aiRulesDir, "domains", "frontend", "rules"), "component-standards.md", V4DomainFrontendRule)
}
