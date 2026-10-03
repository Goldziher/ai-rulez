package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/hooks"
	"github.com/Goldziher/ai-rulez/internal/importer"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/schema"
	"github.com/spf13/cobra"
)

var (
	formatFlag       string
	domainsFlag      string
	skipContentFlag  bool
	fromFlag         string
	setupHooks       bool
	autoYes          bool
	initConfigDirArg string
)

var InitCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new AI rules configuration",
	Long: `Initialize a new AI rules configuration for your project.
This creates a .ai-rulez/ directory structure with configuration files,
rules, context, and skills for your selected AI assistants.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runInit,
}

const (
	formatTOML = "toml"
	formatYAML = "yaml"
	formatJSON = "json"
)

func init() {
	InitCmd.Flags().StringVarP(&formatFlag, "format", "f", formatTOML, "Config format: toml, yaml, or json")
	InitCmd.Flags().StringVarP(&domainsFlag, "domains", "d", "", "Comma-separated list of domain directories to create")
	InitCmd.Flags().BoolVarP(&skipContentFlag, "skip-content", "s", false, "Skip creating example content files")
	InitCmd.Flags().StringVarP(&fromFlag, "from", "F", "", "Import from existing tool files (e.g., 'auto', '.claude,.cursor')")
	InitCmd.Flags().BoolVarP(&setupHooks, "setup-hooks", "H", false, "Automatically configure git hooks for ai-rulez validation")
	InitCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically answer yes to prompts")
	InitCmd.Flags().StringVar(&initConfigDirArg, "config-dir", "", "Configuration directory to create (default: .ai-rulez; use .config/ai-rulez for the .config/ convention)")
}

// initConfigDir returns the configuration directory to scaffold: the
// --config-dir value when set (e.g. ".config/ai-rulez"), else ".ai-rulez".
func initConfigDir() string {
	if initConfigDirArg != "" {
		return filepath.Clean(filepath.FromSlash(initConfigDirArg))
	}
	return ".ai-rulez"
}

func runInit(cmd *cobra.Command, args []string) {
	projectName := getProjectName(args)
	configDir := initConfigDir()

	// Check if the configuration directory already exists
	if _, err := os.Stat(configDir); err == nil {
		logger.Info(configDir + "/ directory already exists")
		if !shouldOverwriteConfig(configDir + "/") {
			logger.Info("Operation canceled. Remove or rename the existing directory to initialize a new configuration")
			os.Exit(1)
		}

		// Remove existing directory
		if err := os.RemoveAll(configDir); err != nil {
			logger.Error("Failed to remove existing "+configDir+"/ directory", "error", err)
			os.Exit(1)
		}
		logger.Info("Existing " + configDir + "/ directory removed")
	}

	// Handle --from flag for importing from existing tool files
	if fromFlag != "" {
		workingDir, err := os.Getwd()
		if err != nil {
			logger.Error("Failed to get working directory", "error", err)
			os.Exit(1)
		}

		aiRulezDir := filepath.Join(workingDir, filepath.FromSlash(configDir))

		imp := importer.NewImporter(workingDir, aiRulezDir)
		if err := imp.Import(fromFlag); err != nil {
			logger.Error("Failed to import from sources", "error", err)
			os.Exit(1)
		}

		displayImportSuccessMessage(fromFlag, configDir)
		return
	}

	// Create directory structure
	if err := createStructure(projectName, configDir); err != nil {
		logger.Error("Failed to create structure", "error", err)
		os.Exit(1)
	}

	// Create domain directories if specified
	if domainsFlag != "" {
		domains := parseDomains(domainsFlag)
		if err := createDomainDirectories(domains, configDir); err != nil {
			logger.Error("Failed to create domain directories", "error", err)
			os.Exit(1)
		}
	}

	// Create example content unless --skip-content is specified
	if !skipContentFlag {
		if err := createExampleContent(configDir); err != nil {
			logger.Error("Failed to create example content", "error", err)
			os.Exit(1)
		}
	}

	displaySuccessMessage(projectName, configDir)
}

// createStructure creates the basic configuration directory structure
func createStructure(projectName, configDir string) error {
	// Create base directories
	dirs := []string{
		configDir,
		filepath.Join(configDir, "rules"),
		filepath.Join(configDir, "context"),
		filepath.Join(configDir, "skills"),
		filepath.Join(configDir, "agents"),
		filepath.Join(configDir, "domains"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Generate and write config file
	var configContent string
	var configPath string

	switch formatFlag {
	case formatJSON:
		configContent = generateConfigJSON(projectName)
		configPath = filepath.Join(configDir, "config.json")
	case formatTOML:
		configContent = generateConfigTOML(projectName)
		configPath = filepath.Join(configDir, "config.toml")
	default: // yaml
		configContent = generateConfig(projectName)
		configPath = filepath.Join(configDir, "config.yaml")
	}

	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Debug("Created structure", "config", configPath)
	return nil
}

// generateConfig generates a YAML configuration template
// builtinPresetList names every built-in preset, for the comment in a new config.
func builtinPresetList() string {
	return strings.Join(config.IndividualPresetNames(), ", ")
}

func generateConfig(projectName string) string {
	var builder strings.Builder

	builder.WriteString(`# AI-Rulez Configuration
# Directory-based configuration with domain scoping
# Documentation: https://github.com/Goldziher/ai-rulez

# Schema reference for IDE validation
$schema: `)
	builder.WriteString(schema.SchemaURL(schema.ConfigSchemaFile))
	builder.WriteString(`

# Version (required)
version: "4.0"

# Project name (required)
name: "`)
	builder.WriteString(projectName)
	builder.WriteString(`"

# Optional description
# description: "AI-powered development governance for `)
	builder.WriteString(projectName)
	builder.WriteString(`"

# Presets: built-in tools or custom outputs
# Built-in presets: `)
	builder.WriteString(builtinPresetList())
	builder.WriteString(`
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

# MCP Servers (optional)
# mcp_servers:
#   - name: ai-rulez
#     command: npx
#     args: ["-y", "ai-rulez@latest", "mcp"]
`)

	return builder.String()
}

// generateConfigJSON generates a JSON configuration template
func generateConfigJSON(projectName string) string {
	return fmt.Sprintf(`{
  "$schema": "%s",
  "$comment": "AI-Rulez Configuration - Directory-based configuration with domain scoping",
  "version": "4.0",
  "name": "%s",
  "description": "AI-powered development governance for %s",
  "presets": ["claude"],
  "gitignore": true
}
`, schema.SchemaURL(schema.ConfigSchemaFile), projectName, projectName)
}

// generateConfigTOML generates a TOML configuration template
func generateConfigTOML(projectName string) string {
	var builder strings.Builder
	builder.WriteString(`# AI-Rulez Configuration
# Directory-based configuration with domain scoping
# Documentation: https://github.com/Goldziher/ai-rulez

# Version (required)
version = "4.0"

# Project name (required)
name = "`)
	builder.WriteString(projectName)
	builder.WriteString(`"

# Optional description
# description = "AI-powered development governance for `)
	builder.WriteString(projectName)
	builder.WriteString(`"

# Presets: built-in tools or custom outputs
# Built-in presets: `)
	builder.WriteString(builtinPresetList())
	builder.WriteString(`
presets = ["claude"]

# Default profile to use when generating
# default = "full"

# Named profiles (domain combinations)
# [profiles]
# full = ["backend", "frontend", "qa"]
# backend = ["backend"]
# frontend = ["frontend"]

# Gitignore management
# gitignore = true

# MCP Servers (optional)
# [[mcp_servers]]
# name = "ai-rulez"
# command = "npx"
# args = ["-y", "ai-rulez@latest", "mcp"]
`)
	return builder.String()
}

// createDomainDirectories creates domain subdirectories
func createDomainDirectories(domains []string, configDir string) error {
	for _, domain := range domains {
		dirs := []string{
			filepath.Join(configDir, "domains", domain),
			filepath.Join(configDir, "domains", domain, "rules"),
			filepath.Join(configDir, "domains", domain, "context"),
			filepath.Join(configDir, "domains", domain, "skills"),
			filepath.Join(configDir, "domains", domain, "agents"),
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
func createExampleContent(configDir string) error {
	createSkill := func(skillID, content string) error {
		skillDir := filepath.Join(configDir, "skills", skillID)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return fmt.Errorf("failed to create skill directory %s: %w", skillID, err)
		}

		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			return fmt.Errorf("failed to write example skill %s: %w", skillID, err)
		}

		return nil
	}

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

	if err := os.WriteFile(filepath.Join(configDir, "rules", "code-quality.md"), []byte(ruleContent), 0o644); err != nil {
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

	if err := os.WriteFile(filepath.Join(configDir, "context", "architecture.md"), []byte(contextContent), 0o644); err != nil {
		return fmt.Errorf("failed to write example context: %w", err)
	}

	codeReviewerSkill := `---
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

	if err := createSkill("code-reviewer", codeReviewerSkill); err != nil {
		return err
	}

	aiRulezSkill := `---
name: ai-rulez
description: Use AI-Rulez correctly in user projects, including CLI, MCP, configuration, and generation workflows
priority: high
---

# AI-Rulez

Use this skill when working in a project that is managed by AI-Rulez.

## Responsibilities

- Detect whether the project uses AI-Rulez (.ai-rulez/) or a legacy V2 config.
- Edit source files in .ai-rulez/ instead of patching generated assistant files directly
- Prefer the AI-Rulez MCP server for safe reads and CRUD operations when it is available
- Use the CLI to validate, generate, and inspect configuration changes
- Keep generated outputs in sync with configuration changes

## Workflow

1. Check for .ai-rulez/config.yaml (or config.toml), .ai-rulez/skills/, domain folders.
2. If MCP is configured in the config, prefer the MCP server for reading and modifying AI-Rulez content.
3. Update the relevant source files under .ai-rulez/: rules, context, skills, agents, domains, or config.
4. Run ai-rulez validate when changing configuration structure.
5. Run ai-rulez generate after source changes so assistant-specific outputs stay current.
6. If MCP is available, start it with npx -y ai-rulez@latest mcp (or the repo’s helper) and use it for CRUD instead of manual edits when possible.

## Core Commands

- ai-rulez init — scaffold .ai-rulez/ for a project.
- ai-rulez add|remove|list rule|context|skill|agent — manage content files.
- ai-rulez validate — ensure config and tree structure are sound.
- ai-rulez generate [--profile <name>] — render tool presets after edits.
- ai-rulez migrate — convert legacy ai-rulez.yaml to .ai-rulez/.

## Guidelines

- Treat .ai-rulez/ as the source of truth.
- Generated files such as AGENTS.md, CLAUDE.md, or .cursor/ outputs should only change via generation.
- Use ai-rulez init to bootstrap, generate to render outputs, validate to check structure, and migrate for format migration.
- Remember that root content is always included, while domains are controlled by profiles.
- MCP can expose read, CRUD, generate, and validate operations for assistants.
- When changing presets, profiles, or domains in config.yaml, rerun validate then generate so downstream files stay in sync.
`

	aiRulezSkill = strings.ReplaceAll(aiRulezSkill, ".ai-rulez/", configDir+"/")
	if err := createSkill("ai-rulez", aiRulezSkill); err != nil {
		return err
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

// displaySuccessMessage displays a success message after initialization
func displaySuccessMessage(projectName, configDir string) {
	logger.Info("✅ Created "+configDir+"/ directory structure", "project", projectName)
	logger.Info("\nDirectory structure:")
	logger.Info("  " + configDir + "/")

	// Determine config filename based on format flag
	var configFilename string
	switch formatFlag {
	case formatYAML:
		configFilename = configFileYAML
	case formatJSON:
		configFilename = configFileJSON
	default:
		configFilename = configFileTOML
	}

	logger.Info(fmt.Sprintf("  ├── %s", configFilename))
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
		logger.Info("  - skills/ai-rulez/SKILL.md")
	}

	if domainsFlag != "" {
		domains := parseDomains(domainsFlag)
		logger.Info("\nDomain directories created:")
		for _, domain := range domains {
			logger.Info(fmt.Sprintf("  - domains/%s/", domain))
		}
	}

	logger.Info("\nNext steps:")
	logger.Info(fmt.Sprintf("  1. Edit %s/%s to customize presets, profiles, and MCP servers", configDir, configFilename))
	logger.Info("  2. Add your rules, context, skills, and agents to the appropriate directories")
	logger.Info("  3. Run 'ai-rulez generate' to create tool-specific outputs")

	if setupHooks {
		handleHooksSetup()
	}
}

// displayImportSuccessMessage displays a success message after import
func displayImportSuccessMessage(sources, configDir string) {
	logger.Info("✅ Successfully imported content to " + configDir + "/")
	logger.Info(fmt.Sprintf("   Sources: %s", sources))

	logger.Info("\nImported structure:")
	logger.Info("  " + configDir + "/")
	logger.Info("  ├── config.yaml")
	logger.Info("  ├── rules/         # Imported rules")
	logger.Info("  ├── context/       # Imported context")
	logger.Info("  └── skills/        # Imported skills")

	logger.Info("\nNext steps:")
	logger.Info("  1. Review imported content in " + configDir + "/")
	logger.Info("  2. Edit " + configDir + "/config.yaml to customize presets")
	logger.Info("  3. Run 'ai-rulez generate' to create tool-specific outputs")
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
