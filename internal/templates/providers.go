package templates

import (
	"strings"
)

type ProviderConfig struct {
	Claude      bool
	Cursor      bool
	Windsurf    bool
	Copilot     bool
	Gemini      bool
	Amp         bool
	Codex       bool
	Opencode    bool
	Cline       bool
	ContinueDev bool
	Junie       bool
}

func (p ProviderConfig) HasAny() bool {
	return p.Claude || p.Cursor || p.Windsurf || p.Copilot ||
		p.Gemini || p.Amp || p.Codex || p.Opencode || p.Cline || p.ContinueDev || p.Junie
}

func getDefaultOutputPath(providers ProviderConfig) string {
	if providers.Claude {
		return "CLAUDE.md"
	}
	if providers.Cursor {
		return ".cursorrules"
	}
	if providers.Gemini {
		return "GEMINI.md"
	}
	if providers.Amp || providers.Codex || providers.Opencode {
		return "AGENTS.md"
	}
	if providers.Windsurf {
		return ".windsurf/rules/01-main.md"
	}
	if providers.Copilot {
		return ".github/copilot-instructions.md"
	}
	if providers.Cline {
		return ".clinerules/01-main.md"
	}
	if providers.ContinueDev {
		return ".continue/rules/01-main.md"
	}
	if providers.Junie {
		return ".junie/guidelines.md"
	}
	return "ai-rules.md"
}

func writeProviderOutputs(builder *strings.Builder, providers ProviderConfig) {
	defaultPath := getDefaultOutputPath(providers)

	if providers.Claude && defaultPath != "CLAUDE.md" {
		builder.WriteString(`  - path: "CLAUDE.md"
`)
	}

	if providers.Cursor {
		builder.WriteString(`  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "{name}.mdc"
`)
	}

	if providers.Windsurf {
		builder.WriteString(`  - path: ".windsurf/rules/"
    type: "rule"
    naming_scheme: "{name}.md"
`)
	}

	if providers.Copilot {
		builder.WriteString(`  - path: ".github/copilot-instructions.md"
`)
	}

	if providers.Gemini {
		builder.WriteString(`  - path: "GEMINI.md"
`)
	}

	if providers.Amp || providers.Codex || providers.Opencode {
		builder.WriteString(`  - path: "AGENTS.md"
`)
	}

	if providers.Cline {
		builder.WriteString(`  - path: ".clinerules/"
    type: "rule"
    naming_scheme: "{name}.md"
`)
	}

	if providers.ContinueDev {
		if defaultPath != ".continue/rules/01-main.md" {
			builder.WriteString(`  - path: ".continue/rules/"
    type: "rule"
    naming_scheme: "{priority:02d}-{name}.md"
  - path: ".continue/prompts/ai_rulez_prompts.yaml"
    type: "agent"
    naming_scheme: "ai_rulez_prompts.yaml"
`)
		} else {
			builder.WriteString(`  - path: ".continue/prompts/ai_rulez_prompts.yaml"
    type: "agent"
    naming_scheme: "ai_rulez_prompts.yaml"
`)
		}
	}

	if providers.Junie {
		builder.WriteString(`  - path: ".junie/guidelines.md"
`)
	}

	if providers.Claude {
		builder.WriteString(`  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"
`)
	}
}

func writeCommentedExamples(builder *strings.Builder, providers ProviderConfig) {
	builder.WriteString(`  
  # Additional output examples (uncomment as needed):
`)

	if !providers.Claude {
		builder.WriteString(`  # - path: "CLAUDE.md"              # For Claude AI assistant
`)
	}
	if !providers.Cursor {
		builder.WriteString(`  # - path: ".cursorrules"           # For Cursor editor
`)
	}
	if !providers.Gemini {
		builder.WriteString(`  # - path: "GEMINI.md"              # For Google Gemini
`)
	}
	if !providers.Amp && !providers.Codex && !providers.Opencode {
		builder.WriteString(`  # - path: "AGENTS.md"              # For Amp/Codex/Opencode
`)
	}
	if !providers.Junie {
		builder.WriteString(`  # - path: ".junie/guidelines.md"   # For JetBrains Junie
`)
	}
}

func GeneratePresetConfigTemplate(projectName string, presets []string) string {
	var builder strings.Builder

	builder.WriteString(`# AI-Rulez Configuration v2.0
# This file configures AI agent behavior for your project
# Documentation: https://github.com/Goldziher/ai-rulez
#
# Quick Start:
# 1. Review and adjust the required fields below
# 2. Uncomment and customize optional sections as needed
# 3. Run 'ai-rulez generate' to create output files
# 4. Run 'ai-rulez validate' to check configuration

$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

# ========================================
# REQUIRED FIELDS (do not comment out)
# ========================================

# Project metadata (REQUIRED - at minimum, name is required)
metadata:
  name: "`)
	builder.WriteString(projectName)
	builder.WriteString(`"
  # version: "1.0.0"                    # Optional: semantic version
  # description: "Brief description"    # Optional: project description

# Presets to use (REQUIRED - at least one preset)
presets:
`)

	for _, preset := range presets {
		builder.WriteString("  - \"")
		builder.WriteString(preset)
		builder.WriteString("\"\n")
	}

	builder.WriteString(`
# ========================================
# Everything below is OPTIONAL
# Uncomment and customize sections as needed
# ========================================

# ========================================
# Rules: Define coding standards and guidelines
# ========================================

# rules:
#   - name: "Code Standards"
#     priority: high
#     content: |
#       Follow these coding standards:
#       - Use descriptive variable names
#       - Add comments for complex logic
#       - Write unit tests for new functions

# ========================================
# Sections: Add documentation sections
# ========================================

# sections:
#   - name: "Architecture Overview"
#     priority: high
#     content: |
#       This project follows a modular architecture...

# ========================================
# Agents: Define specialized AI agents (Claude only)
# ========================================

# agents:
#   - name: "Code Reviewer"
#     description: "Specialized agent for code review"
#     priority: high
#     tools: ["read", "write"]
#     system_prompt: |
#       You are a senior code reviewer. Focus on:
#       - Code quality and best practices
#       - Performance optimizations
#       - Security considerations
`)

	return builder.String()
}

func GenerateConfigWithPresets(projectName string, presets []string) string {
	var builder strings.Builder
	builder.WriteString(`# AI-Rulez Configuration v2.0
# This file configures AI agent behavior for your project
# Documentation: https://github.com/Goldziher/ai-rulez
#
# Quick Start:
# 1. Review and adjust the required fields below
# 2. Uncomment and customize optional sections as needed
# 3. Run 'ai-rulez generate' to create output files
# 4. Run 'ai-rulez validate' to check configuration

$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

# ========================================
# REQUIRED FIELDS (do not comment out)
# ========================================

# Project metadata (REQUIRED - at minimum, name is required)
metadata:
  name: "`)
	builder.WriteString(projectName)
	builder.WriteString(`"
  # version: "1.0.0"                    # Optional: semantic version
  # description: "Brief description"    # Optional: project description

# Presets to use (REQUIRED - at least one preset)
presets:
`)
	for _, preset := range presets {
		builder.WriteString(`  - "`)
		builder.WriteString(preset)
		builder.WriteString(`"
`)
	}

	builder.WriteString(`
# ========================================
# Everything below is OPTIONAL
# Uncomment and customize sections as needed
# ========================================

# ========================================
# Rules: Define coding standards and guidelines
# ========================================

# rules:
#   - name: "Code Standards"
#     priority: high
#     content: |
#       Follow these coding standards:
#       - Use descriptive variable names
#       - Add comments for complex logic
#       - Write unit tests for new functions

# ========================================
# Sections: Add documentation sections
# ========================================

# sections:
#   - name: "Architecture Overview"
#     priority: high
#     content: |
#       This project follows a modular architecture...

# ========================================
# Agents: Define specialized AI agents (Claude only)
# ========================================

# agents:
#   - name: "Code Reviewer"
#     description: "Specialized agent for code review"
#     priority: high
#     tools: ["read", "write"]
#     system_prompt: |
#       You are a senior code reviewer. Focus on:
#       - Code quality and best practices
#       - Performance optimizations
#       - Security considerations
`)

	return builder.String()
}

func GenerateConfigTemplate(projectName string, providers ProviderConfig) string {
	var builder strings.Builder

	builder.WriteString(`# AI-Rulez Configuration v2.0
# This file configures AI agent behavior for your project
# Documentation: https://github.com/Goldziher/ai-rulez
#
# Quick Start:
# 1. Review and adjust the required fields below
# 2. Uncomment and customize optional sections as needed
# 3. Run 'ai-rulez generate' to create output files
# 4. Run 'ai-rulez validate' to check configuration

$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

# ========================================
# REQUIRED FIELDS (do not comment out)
# ========================================

# Project metadata (REQUIRED - at minimum, name is required)
metadata:
  name: "`)
	builder.WriteString(projectName)
	builder.WriteString(`"
  # version: "1.0.0"                    # Optional: semantic version
  # description: "Brief description"    # Optional: project description

# Output files to generate (REQUIRED - at least one output)
outputs:
  - path: "`)
	builder.WriteString(getDefaultOutputPath(providers))
	builder.WriteString(`"
`)

	writeProviderOutputs(&builder, providers)

	writeCommentedExamples(&builder, providers)

	builder.WriteString(`  
  # Directory outputs for multiple files:
  # - path: ".claude/agents/"          # Generate agent files
  #   type: "agent"                    # Type: agent or rule
  #   naming_scheme: "{name}.md"       # How to name files

# ========================================
# Everything below is OPTIONAL
# Uncomment and customize sections as needed
# ========================================

# ========================================
# AGENTS - Specialized AI personalities
# ========================================
# Define different AI agents for specific tasks
# Agents are like sub-personalities with focused expertise

# agents:
#   # Example 1: System architecture and design specialist
#   - name: "architect"
#     description: "System design and architecture specialist"
#     priority: high  # Options: critical, high, medium, low, minimal
#     system_prompt: |
#       You are a system architect specializing in scalable, maintainable solutions.
#       Focus on: design patterns, tech stack decisions, system boundaries.
#       Always consider trade-offs, future scalability, and maintainability.
#       Provide high-level guidance before implementation details.
#     
#   # Example 2: Software implementation specialist
#   - name: "swe"
#     description: "Software engineering implementation specialist"
#     priority: high
#     # tools: ["Read", "Write", "Edit", "Bash"]  # Optional: limit available tools
#     system_prompt: |
#       You are an expert software engineer focused on clean, efficient implementation.
#       Follow existing code conventions, write comprehensive tests, handle errors properly.
#       Prioritize readability and maintainability over clever solutions.
#
#   # Example 3: Code quality and review specialist
#   - name: "reviewer"
#     description: "Code quality and standards enforcement"
#     priority: medium
#     system_prompt: |
#       You are a senior code reviewer ensuring quality and maintainability.
#       Check for: security issues, performance problems, code smells, missing tests.
#       Provide specific, actionable feedback with examples.
#       Run linters and tests after making changes.

# ========================================
# RULES - Coding standards and practices
# ========================================
# Define project-specific rules and guidelines
# Rules are instructions that guide AI behavior

# rules:
#   # Example 1: Core development workflow
#   - name: "Development Process"
#     priority: critical  # Highest priority rule
#     content: |
#       Follow this workflow for all changes:
#       1. Read and understand existing code before making changes
#       2. Write implementation following project conventions
#       3. Add comprehensive tests for new functionality
#       4. Run linter and fix any issues
#       5. Ensure all tests pass before completing task
#   
#   # Example 2: Code quality standards
#   - name: "Code Quality Standards"
#     priority: high
#     content: |
#       - Keep functions small and focused (typically < 50 lines)
#       - Use descriptive, meaningful variable and function names
#       - Avoid deep nesting (max 3 levels)
#       - Handle errors explicitly, never silently
#       - Comment complex logic and business rules
#       - Follow DRY principle but prioritize readability
#   
#   # Example 3: Testing requirements
#   - name: "Testing Standards"
#     priority: high
#     # targets: ["*.test.*", "*.spec.*"]  # Optional: apply only to test files
#     content: |
#       - Write tests for all new features and bug fixes
#       - Maintain minimum 80% code coverage
#       - Test both happy path and edge cases
#       - Use descriptive test names that explain the scenario
#       - Mock external dependencies appropriately
#       - Keep tests independent and repeatable

# ========================================
# SECTIONS - Project documentation
# ========================================
# Provide context and documentation for AI assistants
# Sections contain project-specific information

# sections:
#   # Example 1: Project structure and organization
#   - name: "Codebase Structure"
#     priority: critical
#     content: |
#       ## Project Layout
#       
#       src/
#         api/        # REST API endpoints and routing
#         services/   # Business logic and core functionality
#         models/     # Data models and schemas
#         utils/      # Helper functions and utilities
#         config/     # Configuration management
#       tests/        # Test files (unit, integration, e2e)
#       docs/         # Project documentation
#       scripts/      # Build and deployment scripts
#       
#       ## Key Entry Points
#       - src/index.ts - Application main entry point
#       - src/app.ts - Express app configuration
#       - src/config.ts - Environment configuration
#       
#       ## Configuration Files
#       - .env.example - Environment variables template
#       - tsconfig.json - TypeScript configuration
#       - package.json - Dependencies and scripts
#   
#   # Example 2: Development conventions and standards
#   - name: "Coding Conventions"
#     priority: high
#     content: |
#       ## Naming Conventions
#       - Files: kebab-case (e.g., user-service.ts)
#       - Classes: PascalCase (e.g., UserService)
#       - Interfaces: PascalCase with 'I' prefix (e.g., IUserData)
#       - Functions/methods: camelCase (e.g., getUserById)
#       - Constants: UPPER_SNAKE_CASE (e.g., MAX_RETRY_COUNT)
#       - Private members: underscore prefix (e.g., _privateMethod)
#       
#       ## Git Workflow
#       - Branch naming: feature/*, bugfix/*, hotfix/*
#       - Commit format: "type(scope): description"
#         Types: feat, fix, docs, style, refactor, test, chore
#       - PR requires: passing tests, code review, updated docs
#       
#       ## Code Style
#       - Use TypeScript strict mode
#       - Prefer const over let, avoid var
#       - Use async/await over promises
#       - Prefer functional programming where appropriate
#   
#   # Example 3: Architecture decisions and rationale
#   - name: "Architecture Decisions"
#     priority: medium
#     content: |
#       ## Technology Stack
#       - **Node.js + TypeScript**: Type safety with JavaScript ecosystem
#       - **Express.js**: Minimal, flexible web framework
#       - **PostgreSQL**: ACID compliance for critical data
#       - **Redis**: Session management and caching layer
#       - **Docker**: Containerization for consistency
#       
#       ## Design Patterns
#       - Repository pattern for data access layer
#       - Dependency injection for better testability
#       - Event-driven architecture for async operations
#       - Factory pattern for object creation
#       
#       ## Security Considerations
#       - JWT for stateless authentication
#       - Rate limiting on all endpoints
#       - Input validation using Joi/Zod
#       - SQL injection prevention via parameterized queries

# ========================================
# COMMANDS - Custom slash commands
# ========================================
# Define custom commands for AI assistants to use
# Commands provide shortcuts for common tasks

# commands:
#   # Example 1: Start a new focused task
#   - name: "task"
#     description: "Start a new focused task"
#     # aliases: ["t", "todo"]  # Optional: alternative names
#     usage: "/task <description>"
#     system_prompt: |
#       You are starting a new task. Focus only on the described objective.
#       Break down the task, implement systematically, test thoroughly.
#       Ignore previous context unless specifically relevant.
#   
#   # Example 2: Generate boilerplate code
#   - name: "generate"
#     aliases: ["gen", "create"]
#     description: "Generate boilerplate code"
#     usage: "/generate <type> <name>"
#     system_prompt: |
#       Generate clean, well-structured boilerplate code.
#       Follow project conventions, include appropriate tests.
#       Types: component, service, model, controller, test
#   
#   # Example 3: Refactor existing code
#   - name: "refactor"
#     description: "Refactor code for better quality"
#     usage: "/refactor <file_or_function>"
#     system_prompt: |
#       Analyze and refactor the specified code.
#       Focus on: readability, performance, maintainability, testability.
#       Ensure all tests still pass after refactoring.
#       Document significant changes made.

# ========================================
# MCP SERVERS - External tool integrations
# ========================================
# Model Context Protocol servers provide additional capabilities
# These are external tools that AI can interact with

# mcp_servers:
#   # Example 1: AI-rulez MCP server for configuration management
#   - name: "ai-rulez"
#     description: "Manage AI-rulez configuration via MCP"
#     command: "uvx"  # Or "npx" depending on what's available
#     args: ["ai-rulez", "mcp"]
#     transport: "stdio"  # Communication method (stdio, http, sse)
#     enabled: true
#   
#   # Example 2: PostgreSQL database server
#   - name: "postgres"
#     description: "PostgreSQL database access"
#     command: "uvx"
#     args: ["mcp-server-postgres", "--connection-string", "${DATABASE_URL}"]
#     env:
#       DATABASE_URL: "postgresql://localhost:5432/mydb"
#     enabled: true
#   
#   # Example 3: GitHub API integration
#   - name: "github"
#     description: "GitHub API for PR and issue management"
#     command: "npx"
#     args: ["-y", "@modelcontextprotocol/server-github"]
#     env:
#       GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
#     # transport: "stdio"  # Default
#     enabled: true

`)

	return builder.String()
}
