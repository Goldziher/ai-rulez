package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version is the current version of ai-rulez (set at build time)
var Version = "dev"

var (
	cfgFile   string
	recursive bool
	dryRun    bool
	rootCmd   = &cobra.Command{
		Use:     "ai-rulez",
		Version: Version,
		Short:   "A CLI tool for managing AI assistant rules",
		Long: `ai-rulez is a CLI tool that helps you manage and generate AI assistant rules
from YAML configuration files. It supports generating rules for different AI assistants
like Claude, Cursor, and Windsurf.`,
	}
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ai-rulez.yaml)")

	// Add commands
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(mcpCmd)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".ai-rulez")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate [config-file]",
	Short: "Generate AI rules files",
	Long: `Generate AI rules files from your configuration.
	
This command will search for configuration files starting from the current
directory and traversing up. Supported file names:
  - .ai-rulez.yaml, .ai-rulez.yml
  - ai-rulez.yaml, ai-rulez.yml
  - .ai_rulez.yaml, .ai_rulez.yml
  - ai_rulez.yaml, ai_rulez.yml

With the -r/--recursive flag, it will find and process all configuration
files in the current directory tree.

With the --dry-run flag, it will validate the configuration and show what would
be generated without writing any files.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		if recursive {
			runRecursiveGenerate()
			return
		}

		var configFile string
		if len(args) > 0 {
			configFile = args[0]
			// Check if specified config file exists
			if _, err := os.Stat(configFile); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Error: Configuration file '%s' not found\n", configFile)
				os.Exit(1)
			}
		} else {
			// Find config file
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Show which config file we're using
		fmt.Println("Using config file:", configFile)

		// Load configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Configuration is already validated during LoadConfig

		if dryRun {
			fmt.Println("\n=== DRY RUN MODE ===")
			fmt.Printf("Configuration: %s (v%s)\n", cfg.Metadata.Name, cfg.Metadata.Version)
			if cfg.Metadata.Description != "" {
				fmt.Printf("Description: %s\n", cfg.Metadata.Description)
			}
			fmt.Printf("\nWould generate %d output file(s):\n", len(cfg.Outputs))
			for _, output := range cfg.Outputs {
				fmt.Printf("  - %s", output.File)
				if output.Template != "" {
					fmt.Printf(" (using template: %s)", output.Template)
				}
				fmt.Println()
			}
			fmt.Printf("\nTotal rules: %d\n", len(cfg.Rules))
			fmt.Printf("Total sections: %d\n", len(cfg.Sections))
			return
		}

		// Generate files
		gen := generator.NewWithBaseDir(filepath.Dir(configFile))
		if err := gen.GenerateAll(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating files: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Generated %d file(s) successfully\n", len(cfg.Outputs))
	},
}

func runRecursiveGenerate() {
	// Find all config files
	configs, err := config.FindAllConfigFiles(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding configuration files: %v\n", err)
		os.Exit(1)
	}

	if len(configs) == 0 {
		fmt.Fprintf(os.Stderr, "No configuration files found\n")
		os.Exit(1)
	}

	// Sort configs for consistent output
	sort.Strings(configs)

	fmt.Printf("Found %d configuration file(s)\n", len(configs))

	successCount := 0
	for _, configFile := range configs {
		fmt.Printf("\n--- Processing: %s ---\n", configFile)

		// Load configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			continue
		}

		// Configuration is already validated during LoadConfig

		if dryRun {
			fmt.Printf("Configuration: %s (v%s)\n", cfg.Metadata.Name, cfg.Metadata.Version)
			fmt.Printf("Would generate %d output file(s)\n", len(cfg.Outputs))
			successCount++
			continue
		}

		// Generate files
		gen := generator.NewWithBaseDir(filepath.Dir(configFile))
		if err := gen.GenerateAll(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating files: %v\n", err)
			continue
		}

		fmt.Printf("✓ Generated %d file(s) successfully\n", len(cfg.Outputs))
		successCount++
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Successfully processed %d/%d configuration(s)\n", successCount, len(configs))

	if successCount < len(configs) {
		os.Exit(1)
	}
}

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate AI rules configuration",
	Long: `Validate your AI rules configuration file for syntax errors,
schema compliance, and logical issues like circular dependencies.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		var configFile string
		if len(args) > 0 {
			configFile = args[0]
		} else {
			// Find config file
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load and validate configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Configuration is already validated during LoadConfig

		fmt.Printf("✓ Configuration is valid: %s\n", configFile)
		fmt.Printf("  Name: %s\n", cfg.Metadata.Name)
		fmt.Printf("  Version: %s\n", cfg.Metadata.Version)
		fmt.Printf("  Rules: %d\n", len(cfg.Rules))
		fmt.Printf("  Sections: %d\n", len(cfg.Sections))
		fmt.Printf("  Outputs: %d\n", len(cfg.Outputs))
	},
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of ai-rulez",
	Long:  `Print the version of ai-rulez CLI tool.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("ai-rulez version %s\n", Version)
	},
}

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new AI rules project",
	Long: `Initialize a new AI rules project with a basic configuration file
and example rules. This creates an ai_rulez.yaml file in the current directory.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := "My Project"
		if len(args) > 0 {
			projectName = args[0]
		}

		// Check if ai_rulez.yaml already exists
		if _, err := os.Stat("ai_rulez.yaml"); err == nil {
			fmt.Fprintf(os.Stderr, "Error: ai_rulez.yaml already exists in current directory\n")
			os.Exit(1)
		}

		// Get template type from flag
		template, _ := cmd.Flags().GetString("template")

		// Create configuration based on template
		var cfg *config.Config
		switch template {
		case "react":
			cfg = createReactTemplate(projectName)
		case "typescript":
			cfg = createTypescriptTemplate(projectName)
		default:
			cfg = createBasicTemplate(projectName)
		}

		// Save configuration
		if err := config.SaveConfig(cfg, "ai_rulez.yaml"); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating configuration file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Initialized new AI rules project: %s\n", projectName)
		fmt.Println("  - Created ai_rulez.yaml")
		fmt.Println("  - Run 'ai-rulez generate' to create rule files")
	},
}

func init() {
	generateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Process all config files recursively")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing files")
	initCmd.Flags().StringP("template", "t", "basic", "Template to use (basic, react, typescript)")
}

func createBasicTemplate(projectName string) *config.Config {
	return &config.Config{
		Metadata: config.Metadata{
			Name:        projectName,
			Version:     "1.0.0",
			Description: "AI assistant rules configuration",
		},
		Outputs: []config.Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{
				Name:     "Code Quality",
				Priority: 10,
				Content:  "Write clean, readable, and maintainable code following best practices.",
			},
			{
				Name:     "Documentation",
				Priority: 5,
				Content:  "Document functions, classes, and complex logic with clear comments.",
			},
			{
				Name:     "Testing",
				Priority: 5,
				Content:  "Write unit tests for all new functionality.",
			},
		},
	}
}

func createReactTemplate(projectName string) *config.Config {
	return &config.Config{
		Metadata: config.Metadata{
			Name:        projectName,
			Version:     "1.0.0",
			Description: "React project AI assistant rules",
		},
		Outputs: []config.Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{
				Name:     "React Best Practices",
				Priority: 10,
				Content:  "Use functional components with hooks. Prefer composition over inheritance.",
			},
			{
				Name:     "Component Structure",
				Priority: 10,
				Content:  "Keep components small and focused. Extract custom hooks for reusable logic.",
			},
			{
				Name:     "State Management",
				Priority: 5,
				Content:  "Use useState for local state, useContext for shared state, consider Redux for complex apps.",
			},
			{
				Name:     "Performance",
				Priority: 5,
				Content:  "Use React.memo, useMemo, and useCallback to optimize performance when needed.",
			},
			{
				Name:     "Testing",
				Priority: 5,
				Content:  "Write unit tests with React Testing Library. Test behavior, not implementation.",
			},
		},
	}
}

func createTypescriptTemplate(projectName string) *config.Config {
	return &config.Config{
		Metadata: config.Metadata{
			Name:        projectName,
			Version:     "1.0.0",
			Description: "TypeScript project AI assistant rules",
		},
		Outputs: []config.Output{
			{File: "claude.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
		Rules: []config.Rule{
			{
				Name:     "Type Safety",
				Priority: 10,
				Content:  "Use strict TypeScript settings. Avoid 'any' type unless absolutely necessary.",
			},
			{
				Name:     "Interface Design",
				Priority: 10,
				Content:  "Define clear interfaces for data structures. Use union types for controlled variations.",
			},
			{
				Name:     "Generic Programming",
				Priority: 5,
				Content:  "Use generics to create reusable, type-safe functions and classes.",
			},
			{
				Name:     "Error Handling",
				Priority: 5,
				Content:  "Use Result/Option patterns or proper error types instead of throwing exceptions.",
			},
			{
				Name:     "Documentation",
				Priority: 3,
				Content:  "Use TSDoc comments for public APIs. Document complex type definitions.",
			},
		},
	}
}

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI assistant integration",
	Long: `Start an MCP (Model Context Protocol) server that exposes ai-rulez functionality 
to AI assistants like Claude Desktop, Cursor, and other MCP-compatible tools.

The server runs in stdio mode and provides tools for:
- Retrieving rules and sections
- Generating output files
- Validating configurations
- Listing available templates

Configure in your AI assistant by adding this server to the MCP configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		runMCPServer()
	},
}

func runMCPServer() {
	// Create MCP server
	s := server.NewMCPServer(
		"ai-rulez",
		Version,
		server.WithToolCapabilities(false),
	)

	// Add ai-rulez tools
	addAIRulezTools(s)

	// Start stdio server
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func addAIRulezTools(s *server.MCPServer) {
	// Tool: Get rules
	getRulesTool := mcp.NewTool("get_rules",
		mcp.WithDescription("Get AI assistant rules from configuration"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
		mcp.WithNumber("min_priority",
			mcp.Description("Minimum priority level to include (optional)"),
		),
		mcp.WithString("name_filter",
			mcp.Description("Filter rules by name (case-insensitive substring match, optional)"),
		),
	)
	s.AddTool(getRulesTool, handleGetRules)

	// Tool: Get sections
	getSectionsTool := mcp.NewTool("get_sections",
		mcp.WithDescription("Get documentation sections from configuration"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(getSectionsTool, handleGetSections)

	// Tool: Generate output
	generateTool := mcp.NewTool("generate_output",
		mcp.WithDescription("Generate AI rules output files"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Show what would be generated without writing files (default: false)"),
		),
	)
	s.AddTool(generateTool, handleGenerate)

	// Tool: Validate config
	validateTool := mcp.NewTool("validate_config",
		mcp.WithDescription("Validate AI rules configuration file"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(validateTool, handleValidate)

	// Tool: List templates
	templatesTool := mcp.NewTool("list_templates",
		mcp.WithDescription("List available project templates for initialization"),
	)
	s.AddTool(templatesTool, handleListTemplates)
}

func handleGetRules(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Apply filters
	minPriority := request.GetFloat("min_priority", 0)
	nameFilter := request.GetString("name_filter", "")

	var filteredRules []config.Rule
	for _, rule := range cfg.Rules {
		// Priority filter
		if minPriority > 0 && float64(rule.Priority) < minPriority {
			continue
		}
		// Name filter
		if nameFilter != "" && !strings.Contains(strings.ToLower(rule.Name), strings.ToLower(nameFilter)) {
			continue
		}
		filteredRules = append(filteredRules, rule)
	}

	// Format response
	result := map[string]interface{}{
		"config_file": configFile,
		"total_rules": len(cfg.Rules),
		"rules_shown": len(filteredRules),
		"rules":       filteredRules,
		"metadata":    cfg.Metadata,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleGetSections(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Format response
	result := map[string]interface{}{
		"config_file":    configFile,
		"total_sections": len(cfg.Sections),
		"sections":       cfg.Sections,
		"metadata":       cfg.Metadata,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleGenerate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Check dry run flag
	dryRun := request.GetBool("dry_run", false)

	if dryRun {
		result := map[string]interface{}{
			"config_file":    configFile,
			"dry_run":        true,
			"would_generate": len(cfg.Outputs),
			"outputs":        cfg.Outputs,
			"metadata":       cfg.Metadata,
			"total_rules":    len(cfg.Rules),
			"total_sections": len(cfg.Sections),
		}
		jsonResult, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(jsonResult)), nil
	}

	// Generate files
	gen := generator.NewWithBaseDir(filepath.Dir(configFile))
	err = gen.GenerateAll(cfg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error generating files: %v", err)), nil
	}

	result := map[string]interface{}{
		"config_file":     configFile,
		"files_generated": len(cfg.Outputs),
		"outputs":         cfg.Outputs,
		"success":         true,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleValidate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load and validate configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Configuration validation failed: %v", err)), nil
	}

	result := map[string]interface{}{
		"config_file":    configFile,
		"valid":          true,
		"metadata":       cfg.Metadata,
		"total_rules":    len(cfg.Rules),
		"total_sections": len(cfg.Sections),
		"total_outputs":  len(cfg.Outputs),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleListTemplates(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	templates := []map[string]interface{}{
		{
			"name":        "basic",
			"description": "Basic AI rules template with code quality, documentation, and testing rules",
			"outputs":     []string{"claude.md", ".cursorrules", ".windsurfrules"},
		},
		{
			"name":        "react",
			"description": "React project template with component structure, state management, and performance rules",
			"outputs":     []string{"claude.md", ".cursorrules", ".windsurfrules"},
		},
		{
			"name":        "typescript",
			"description": "TypeScript project template with type safety, interface design, and error handling rules",
			"outputs":     []string{"claude.md", ".cursorrules", ".windsurfrules"},
		},
	}

	result := map[string]interface{}{
		"available_templates": templates,
		"total_templates":     len(templates),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}
