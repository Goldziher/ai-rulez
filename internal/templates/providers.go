package templates

import (
	"fmt"
	"strings"
)

// ProviderConfig contains the configuration for different AI providers
type ProviderConfig struct {
	Claude      bool
	Cursor      bool
	Windsurf    bool
	Copilot     bool
	Gemini      bool
	Amp         bool
	Codex       bool
	Cline       bool
	ContinueDev bool
}

// HasAny returns true if any provider is enabled
func (p ProviderConfig) HasAny() bool {
	return p.Claude || p.Cursor || p.Windsurf || p.Copilot ||
		p.Gemini || p.Amp || p.Codex || p.Cline || p.ContinueDev
}

// GenerateConfigTemplate generates a configuration template for the given project and providers
func GenerateConfigTemplate(projectName string, providers ProviderConfig) string {
	var builder strings.Builder

	builder.WriteString(`$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

`)

	builder.WriteString(fmt.Sprintf(`metadata:
  name: "%s"
  version: "1.0.0"
  description: "AI assistant rules configuration"

`, projectName))

	builder.WriteString(`# Output configurations - where to generate AI assistant files
outputs:`)

	// Process each enabled provider
	if providers.Claude {
		builder.WriteString(`
  # Claude configuration
  - path: "CLAUDE.md"
  
  # Claude agents (specialized sub-assistants)
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"`)
	}

	// AMP, Codex, and Cursor share AGENTS.md
	if providers.Amp || providers.Codex || providers.Cursor {
		builder.WriteString(`
  
  # AGENTS.md (shared by AMP, Codex, Cursor)
  - path: "AGENTS.md"`)
	}

	if providers.Cursor {
		builder.WriteString(`
  
  # Cursor editor rules
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "rules.mdc"`)
	}

	if providers.Windsurf {
		builder.WriteString(`
  
  # Windsurf editor (new directory format)
  - path: ".windsurf/"
    type: "rule"
    naming_scheme: "{name}.md"`)
	}

	if providers.Gemini {
		builder.WriteString(`
  
  # Google Gemini
  - path: "GEMINI.md"`)
	}

	if providers.Copilot {
		builder.WriteString(`
  
  # GitHub Copilot repository instructions
  - path: ".github/copilot-instructions.md"`)
	}

	if providers.Cline {
		builder.WriteString(`
  
  # Cline rules (directory format)
  - path: ".clinerules/"
    type: "rule"
    naming_scheme: "{priority:02d}-{name}.md"`)
	}

	if providers.ContinueDev {
		builder.WriteString(`
  
  # Continue.dev rules (directory format with frontmatter)
  - path: ".continue/rules/"
    type: "rule"
    naming_scheme: "{priority:02d}-{name}.md"`)
	}

	builder.WriteString("\n\n")

	// Add agents section if Claude is enabled
	if providers.Claude {
		builder.WriteString(`# AI agents (specialized sub-assistants for Claude)
agents:
  - name: "code-reviewer"
    description: "Code review and quality analysis specialist"
    system_prompt: |
      You are a senior code reviewer focusing on:
      - Code quality and maintainability
      - Security vulnerabilities
      - Performance implications
      - Best practices and conventions
      
      Always provide constructive feedback with specific suggestions.
  
  - name: "test-writer"
    description: "Testing and quality assurance specialist"
    system_prompt: |
      You are a testing expert specializing in:
      - Writing comprehensive test cases
      - Test automation and CI/CD
      - Quality assurance processes
      - Test-driven development (TDD)

`)
	}

	// Add default rules
	builder.WriteString(`rules:
  - name: "Code Quality"
    priority: 10
    content: |
      - Write clean, readable, and maintainable code
      - Follow consistent coding standards and conventions
      - Use meaningful variable and function names
      - Keep functions small and focused
      - Avoid code duplication and follow DRY principles

  - name: "Testing Standards"
    priority: 9
    content: |
      - Write comprehensive unit tests for all new functionality
      - Maintain test coverage above 80%
      - Test edge cases and error conditions
      - Use descriptive test names that explain the scenario
      - Follow testing best practices (AAA pattern, mocking, etc.)

  - name: "Documentation"
    priority: 8
    content: |
      - Document all public APIs and complex logic
      - Keep README and documentation up to date
      - Write clear, descriptive commit messages
      - Include usage examples for complex functionality
      - Document design decisions and architecture

  - name: "Error Handling"
    priority: 7
    content: |
      - Always handle errors explicitly
      - Provide meaningful error messages with context
      - Use appropriate error types and logging levels
      - Avoid silent failures and swallowing exceptions
      - Implement proper error recovery where possible

`)

	// Add sections
	builder.WriteString(`sections:
  - name: "Project Context"
    priority: 10
    content: |
      This project follows industry best practices for software development.
      All code should be production-ready, well-tested, and properly documented.
      We prioritize maintainability, performance, and security.

  - name: "Development Guidelines"
    priority: 9
    content: |
      - Follow the project's established coding conventions
      - Run tests before committing code
      - Keep dependencies up to date and minimal
      - Use feature branches and pull requests for changes
      - Review code before merging to main branch
`)

	return builder.String()
}
