package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/hooks"
	"github.com/Goldziher/ai-rulez/internal/importer"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	formatFlag      string
	domainsFlag     string
	skipContentFlag bool
	skipMCPFlag     bool
	fromFlag        string
	setupHooks      bool
	autoYes         bool
)

var InitCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new V3 AI rules configuration",
	Long: `Initialize a new V3 AI rules configuration for your project.
This creates a .ai-rulez/ directory structure with configuration files,
rules, context, and skills for your selected AI assistants.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runInit,
}

func init() {
	InitCmd.Flags().StringVar(&formatFlag, "format", "yaml", "Config format: yaml or json")
	InitCmd.Flags().StringVar(&domainsFlag, "domains", "", "Comma-separated list of domain directories to create")
	InitCmd.Flags().BoolVar(&skipContentFlag, "skip-content", false, "Skip creating example content files")
	InitCmd.Flags().BoolVar(&skipMCPFlag, "skip-mcp", false, "Skip creating example MCP servers")
	InitCmd.Flags().StringVar(&fromFlag, "from", "", "Import from existing tool files (e.g., 'auto', '.claude,.cursor')")
	InitCmd.Flags().BoolVar(&setupHooks, "setup-hooks", false, "Automatically configure git hooks for ai-rulez validation")
	InitCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically answer yes to prompts")
}

func runInit(cmd *cobra.Command, args []string) {
	projectName := getProjectName(args)

	// Check if .ai-rulez/ already exists
	if _, err := os.Stat(".ai-rulez"); err == nil {
		logger.Info(".ai-rulez/ directory already exists")
		if !shouldOverwriteConfig(".ai-rulez/") {
			logger.Info("Operation canceled. Remove or rename the existing directory to initialize a new configuration")
			os.Exit(1)
		}

		// Remove existing directory
		if err := os.RemoveAll(".ai-rulez"); err != nil {
			logger.Error("Failed to remove existing .ai-rulez/ directory", "error", err)
			os.Exit(1)
		}
		logger.Info("Existing .ai-rulez/ directory removed")
	}

	// Handle --from flag for importing from existing tool files
	if fromFlag != "" {
		workingDir, err := os.Getwd()
		if err != nil {
			logger.Error("Failed to get working directory", "error", err)
			os.Exit(1)
		}

		aiRulezDir := filepath.Join(workingDir, ".ai-rulez")

		imp := importer.NewImporter(workingDir, aiRulezDir)
		if err := imp.Import(fromFlag); err != nil {
			logger.Error("Failed to import from sources", "error", err)
			os.Exit(1)
		}

		displayImportSuccessMessage(fromFlag)
		return
	}

	// Create V3 structure
	if err := createV3Structure(projectName); err != nil {
		logger.Error("Failed to create V3 structure", "error", err)
		os.Exit(1)
	}

	// Create domain directories if specified
	if domainsFlag != "" {
		domains := parseDomains(domainsFlag)
		if err := createDomainDirectories(domains); err != nil {
			logger.Error("Failed to create domain directories", "error", err)
			os.Exit(1)
		}
	}

	// Create example content unless --skip-content is specified
	if !skipContentFlag {
		if err := createExampleContent(); err != nil {
			logger.Error("Failed to create example content", "error", err)
			os.Exit(1)
		}
	}

	// Create example MCP servers unless --skip-mcp is specified
	if !skipMCPFlag {
		if err := createExampleMCPFiles(); err != nil {
			logger.Error("Failed to create example MCP files", "error", err)
			os.Exit(1)
		}
	}

	displayV3SuccessMessage(projectName)
}

// createV3Structure creates the basic .ai-rulez/ directory structure
func createV3Structure(projectName string) error {
	// Create base directories
	dirs := []string{
		".ai-rulez",
		".ai-rulez/rules",
		".ai-rulez/context",
		".ai-rulez/skills",
		".ai-rulez/agents",
		".ai-rulez/domains",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Generate and write config file
	var configContent string
	var configPath string

	if formatFlag == "json" {
		configContent = generateV3ConfigJSON(projectName)
		configPath = ".ai-rulez/config.json"
	} else {
		configContent = generateV3Config(projectName)
		configPath = ".ai-rulez/config.yaml"
	}

	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Debug("Created V3 structure", "config", configPath)
	return nil
}

// generateV3Config generates a V3 YAML configuration template
func generateV3Config(projectName string) string {
	var builder strings.Builder

	builder.WriteString(`# AI-Rulez V3 Configuration
# Directory-based configuration with domain scoping
# Documentation: https://github.com/Goldziher/ai-rulez

# Schema reference for IDE validation
$schema: https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-v3.schema.json

# Version (required)
version: "3.0"

# Project name (required)
name: "`)
	builder.WriteString(projectName)
	builder.WriteString(`"

# Optional description
# description: "AI-powered development governance for `)
	builder.WriteString(projectName)
	builder.WriteString(`"

# Presets: built-in tools or custom outputs
# Built-in presets: claude, cursor, windsurf, copilot, gemini, cline, continue-dev, junie
presets:
  - claude

# Default profile to use when generating
# default: full

# Named profiles (domain combinations)
# profiles:
#   full: [backend, frontend, qa]
#   backend: [backend]
#   frontend: [frontend]

# Gitignore management
# gitignore: true
`)

	return builder.String()
}

// generateV3ConfigJSON generates a V3 JSON configuration template
func generateV3ConfigJSON(projectName string) string {
	return fmt.Sprintf(`{
  "$schema": "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-v3.schema.json",
  "$comment": "AI-Rulez V3 Configuration - Directory-based configuration with domain scoping",
  "version": "3.0",
  "name": "%s",
  "description": "AI-powered development governance for %s",
  "presets": ["claude"],
  "gitignore": true
}
`, projectName, projectName)
}

// createDomainDirectories creates domain subdirectories
func createDomainDirectories(domains []string) error {
	for _, domain := range domains {
		dirs := []string{
			filepath.Join(".ai-rulez", "domains", domain),
			filepath.Join(".ai-rulez", "domains", domain, "rules"),
			filepath.Join(".ai-rulez", "domains", domain, "context"),
			filepath.Join(".ai-rulez", "domains", domain, "skills"),
			filepath.Join(".ai-rulez", "domains", domain, "agents"),
		}

		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		logger.Debug("Created domain directory", "domain", domain)
	}

	return nil
}

// createExampleContent creates example rule, context, and skill files
func createExampleContent() error {
	// Create example rule
	ruleContent := `---
priority: high
---

# Code Quality Standards

Follow these coding standards:
- Use descriptive variable names
- Add comments for complex logic
- Write unit tests for new functions
- Keep functions small and focused (typically < 50 lines)
- Handle errors explicitly, never silently
`

	if err := os.WriteFile(".ai-rulez/rules/code-quality.md", []byte(ruleContent), 0o644); err != nil {
		return fmt.Errorf("failed to write example rule: %w", err)
	}

	// Create example context
	contextContent := `---
priority: medium
---

# Project Architecture

This project follows a modular architecture with clear separation of concerns.

## Directory Structure

- **cmd/**: Command-line interface and entry points
- **internal/**: Private application code
- **pkg/**: Public library code
- **tests/**: Test files and fixtures

## Key Principles

- Dependency injection for better testability
- Interface-based design for flexibility
- Clear separation between business logic and infrastructure
`

	if err := os.WriteFile(".ai-rulez/context/architecture.md", []byte(contextContent), 0o644); err != nil {
		return fmt.Errorf("failed to write example context: %w", err)
	}

	// Create example skill directory and SKILL.md
	skillDir := ".ai-rulez/skills/code-reviewer"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	skillContent := `---
name: code-reviewer
description: Specialized agent for code review and quality assurance
priority: high
---

# Code Reviewer

You are a senior code reviewer focused on ensuring quality and maintainability.

## Responsibilities

- Review code for quality, security, and performance
- Check for adherence to coding standards
- Identify potential bugs and edge cases
- Suggest improvements and best practices

## Guidelines

- Provide specific, actionable feedback
- Explain the reasoning behind suggestions
- Focus on high-impact improvements
- Be constructive and helpful in tone
`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		return fmt.Errorf("failed to write example skill: %w", err)
	}

	logger.Debug("Created example content files")
	return nil
}

// parseDomains parses the comma-separated domains flag
func parseDomains(domainsStr string) []string {
	parts := strings.Split(domainsStr, ",")
	domains := make([]string, 0, len(parts))

	for _, part := range parts {
		domain := strings.TrimSpace(part)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	return domains
}

// displayV3SuccessMessage displays a success message after V3 initialization
func displayV3SuccessMessage(projectName string) {
	logger.Info("✅ Created .ai-rulez/ directory structure", "project", projectName)
	logger.Info("\nDirectory structure:")
	logger.Info("  .ai-rulez/")
	logger.Info("  ├── config.yaml (or config.json)")
	logger.Info("  ├── rules/         # Base rules (always included)")
	logger.Info("  ├── context/       # Base context (always included)")
	logger.Info("  ├── skills/        # Base skills (always included)")
	logger.Info("  ├── agents/        # Base agents (always included)")
	logger.Info("  └── domains/       # Domain-specific content")

	if !skipContentFlag {
		logger.Info("\nExample content created:")
		logger.Info("  - rules/code-quality.md")
		logger.Info("  - context/architecture.md")
		logger.Info("  - skills/code-reviewer/SKILL.md")
	}

	if !skipMCPFlag {
		logger.Info("\nExample MCP servers created:")
		logger.Info("  - mcp.yaml         # Root MCP servers (ai-rulez + GitHub examples)")
		if domainsFlag != "" {
			logger.Info("  - domains/*/mcp.yaml  # Domain-specific MCP servers")
		}
	}

	if domainsFlag != "" {
		domains := parseDomains(domainsFlag)
		logger.Info("\nDomain directories created:")
		for _, domain := range domains {
			logger.Info(fmt.Sprintf("  - domains/%s/", domain))
		}
	}

	logger.Info("\nNext steps:")
	logger.Info("  1. Edit .ai-rulez/config.yaml to customize presets and profiles")
	logger.Info("  2. Add your rules, context, skills, and agents to the appropriate directories")
	logger.Info("  3. Edit .ai-rulez/mcp.yaml to configure MCP servers")
	logger.Info("  4. Run 'ai-rulez generate' to create tool-specific outputs")

	if setupHooks {
		handleHooksSetup()
	}
}

// displayImportSuccessMessage displays a success message after import
func displayImportSuccessMessage(sources string) {
	logger.Info("✅ Successfully imported content to .ai-rulez/")
	logger.Info(fmt.Sprintf("   Sources: %s", sources))

	logger.Info("\nImported structure:")
	logger.Info("  .ai-rulez/")
	logger.Info("  ├── config.yaml")
	logger.Info("  ├── rules/         # Imported rules")
	logger.Info("  ├── context/       # Imported context")
	logger.Info("  └── skills/        # Imported skills")

	logger.Info("\nNext steps:")
	logger.Info("  1. Review imported content in .ai-rulez/")
	logger.Info("  2. Edit .ai-rulez/config.yaml to customize presets")
	logger.Info("  3. Run 'ai-rulez generate' to create tool-specific outputs")
}

// generateExampleMCPYAML generates a template mcp.yaml with GitHub example
func generateExampleMCPYAML() string {
	var builder strings.Builder

	builder.WriteString(`# AI-Rulez V3 MCP (Model Context Protocol) Servers
# Configuration for MCP server integrations
# Documentation: https://github.com/Goldziher/ai-rulez

# Schema reference for IDE validation
$schema: https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-v3-mcp.schema.json

# Version (required)
version: "3.0"

# MCP Servers
mcp_servers:
  # ai-rulez MCP server for managing your .ai-rulez/ configuration
  - name: ai-rulez
    description: AI-Rulez MCP server for configuration management
    command: npx
    args:
      - "-y"
      - "ai-rulez@latest"
      - "mcp"
    transport: stdio
    enabled: true

  # Example: GitHub integration for repository operations
  - name: github
    description: GitHub integration for repository operations and automation
    command: npx
    args:
      - "-y"
      - "@modelcontextprotocol/server-github"
    env:
      # Set GITHUB_PERSONAL_ACCESS_TOKEN environment variable or use ${GITHUB_TOKEN}
      GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
    transport: stdio
    enabled: true
`)

	return builder.String()
}

// generateExampleDomainMCPYAML generates a domain-specific MCP template
func generateExampleDomainMCPYAML(domainName string) string {
	var builder strings.Builder

	builder.WriteString(`# AI-Rulez V3 MCP Servers - `)
	builder.WriteString(domainName)
	builder.WriteString(` Domain
# Domain-specific MCP server configurations
# Documentation: https://github.com/Goldziher/ai-rulez

# Schema reference for IDE validation
$schema: https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-v3-mcp.schema.json

# Version (required)
version: "3.0"

# Domain-specific MCP Servers
mcp_servers:
  # Example: Database server for `)
	builder.WriteString(domainName)
	builder.WriteString(` domain
  - name: `)
	builder.WriteString(domainName)
	builder.WriteString(`-db
    description: Database access for `)
	builder.WriteString(domainName)
	builder.WriteString(` services
    command: uvx
    args:
      - "mcp-server-postgres"
      - "--connection-string"
      - "postgresql://user:pass@localhost:5432/`)
	builder.WriteString(strings.ToLower(domainName))
	builder.WriteString(`"
    transport: stdio
    enabled: false  # Enable this when database is available
`)

	return builder.String()
}

// createExampleMCPFiles creates example MCP YAML files
func createExampleMCPFiles() error {
	// Create root mcp.yaml
	rootMCPContent := generateExampleMCPYAML()
	if err := os.WriteFile(".ai-rulez/mcp.yaml", []byte(rootMCPContent), 0o644); err != nil {
		return fmt.Errorf("failed to write root mcp.yaml: %w", err)
	}

	// Create domain-specific mcp.yaml files if domains were created
	if domainsFlag != "" {
		domains := parseDomains(domainsFlag)
		for _, domain := range domains {
			domainMCPContent := generateExampleDomainMCPYAML(domain)
			domainMCPPath := filepath.Join(".ai-rulez", "domains", domain, "mcp.yaml")
			if err := os.WriteFile(domainMCPPath, []byte(domainMCPContent), 0o644); err != nil {
				return fmt.Errorf("failed to write domain mcp.yaml for %s: %w", domain, err)
			}
		}
	}

	logger.Debug("Created example MCP files")
	return nil
}

func getProjectName(args []string) string {
	projectName := "MyProject"
	if len(args) > 0 {
		projectName = args[0]
	} else {
		if cwd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(cwd)
		}
	}
	return projectName
}

func shouldOverwriteConfig(filename string) bool {
	if autoYes || os.Getenv("CI") != "" || os.Getenv("NO_INTERACTIVE") != "" {
		logger.Info("Auto-overwriting existing configuration directory (--yes or CI environment)")
		return true
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		logger.Info("Cannot prompt for input, canceling operation")
		return false
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		logger.Info("Non-interactive terminal, canceling operation")
		return false
	}

	fmt.Printf("Overwrite existing directory '%s'? (y/N): ", filename)

	var response string
	_, err = fmt.Scanln(&response)
	if err != nil && err.Error() != "unexpected newline" {
		logger.Info("Failed to read input, canceling operation")
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

func handleHooksSetup() {
	logger.Info("\nDetecting git hook managers...")

	hookSystem := hooks.DetectGitHooks()
	if hookSystem == "" {
		logger.Info("  - No git hook manager detected")
		logger.Info("    Install lefthook, pre-commit, or husky to enable automatic validation")
		return
	}

	logger.Info(fmt.Sprintf("  - Found %s configuration", hooks.GetHookSystemName(hookSystem)))

	if err := hooks.SetupHooks(); err != nil {
		logger.Warn(fmt.Sprintf("    Failed to setup %s: %v", hooks.GetHookSystemName(hookSystem), err))
	} else {
		logger.Info(fmt.Sprintf("  ✅ Successfully configured %s for ai-rulez validation", hooks.GetHookSystemName(hookSystem)))
		logger.Info("    Your AI rules will be validated automatically on git commit")
	}
}
