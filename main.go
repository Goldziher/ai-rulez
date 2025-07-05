package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version is the current version of ai-rulez (set at build time)
var Version = "dev"

var (
	cfgFile         string
	recursive       bool
	dryRun          bool
	updateGitignore bool
	rootCmd         = &cobra.Command{
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
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(listProfilesCmd)
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
be generated without writing any files.

With the --update-gitignore flag, it will automatically update .gitignore files
in config directories to include generated output files.`,
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

		// Update .gitignore if requested
		if updateGitignore {
			if err := gitignore.UpdateGitignoreFiles(configFile, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error updating .gitignore: %v\n", err)
				os.Exit(1)
			}
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

		// Update .gitignore if requested
		if updateGitignore {
			if err := gitignore.UpdateGitignoreFiles(configFile, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error updating .gitignore: %v\n", err)
				continue
			}
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

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add rules or sections to configuration",
	Long:  `Add new rules or sections to your AI rules configuration file.`,
}

// addRuleCmd represents the add rule subcommand
var addRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Add a new rule to configuration",
	Long: `Add a new rule to your AI rules configuration file.
The rule name is provided as an argument, and the content can be provided
via stdin or will open an editor for you to enter the rule content.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ruleName := args[0]
		priority, _ := cmd.Flags().GetInt("priority")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Read content from stdin or prompt
		fmt.Println("Enter rule content (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
			os.Exit(1)
		}

		// Add new rule
		newRule := config.Rule{
			Name:     ruleName,
			Priority: priority,
			Content:  content,
		}
		cfg.Rules = append(cfg.Rules, newRule)

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added rule '%s' with priority %d to %s\n", ruleName, priority, configFile)
	},
}

// addSectionCmd represents the add section subcommand
var addSectionCmd = &cobra.Command{
	Use:   "section [title]",
	Short: "Add a new section to configuration",
	Long: `Add a new section to your AI rules configuration file.
The section title is provided as an argument, and the content can be provided
via stdin or will open an editor for you to enter the section content.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sectionTitle := args[0]
		priority, _ := cmd.Flags().GetInt("priority")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Read content from stdin or prompt
		fmt.Println("Enter section content (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
			os.Exit(1)
		}

		// Add new section
		newSection := config.Section{
			Title:    sectionTitle,
			Priority: priority,
			Content:  content,
		}
		cfg.Sections = append(cfg.Sections, newSection)

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added section '%s' with priority %d to %s\n", sectionTitle, priority, configFile)
	},
}

// addOutputCmd represents the add output subcommand
var addOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Add a new output file to configuration",
	Long: `Add a new output file to your AI rules configuration.
The filename is provided as an argument, and you can optionally specify
a template to use for rendering the output.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		template, _ := cmd.Flags().GetString("template")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Check if output already exists
		for _, output := range cfg.Outputs {
			if output.File == filename {
				fmt.Fprintf(os.Stderr, "Error: Output file '%s' already exists in configuration\n", filename)
				os.Exit(1)
			}
		}

		// Add new output
		newOutput := config.Output{
			File:     filename,
			Template: template,
		}
		cfg.Outputs = append(cfg.Outputs, newOutput)

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added output '%s'", filename)
		if template != "" {
			fmt.Printf(" with template '%s'", template)
		}
		fmt.Printf(" to %s\n", configFile)
	},
}

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update existing rules, sections, or outputs",
	Long:  `Update existing rules, sections, or outputs in your AI rules configuration file.`,
}

// updateRuleCmd represents the update rule subcommand
var updateRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Update an existing rule",
	Long: `Update an existing rule in your AI rules configuration file.
You can update the content, priority, or both. If no flags are provided,
you'll be prompted to enter new content via stdin.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ruleName := args[0]
		newContent, _ := cmd.Flags().GetString("content")
		priority, _ := cmd.Flags().GetInt("priority")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Find the rule to update
		ruleIndex := -1
		for i, rule := range cfg.Rules {
			if rule.Name == ruleName {
				ruleIndex = i
				break
			}
		}

		if ruleIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Rule '%s' not found\n", ruleName)
			os.Exit(1)
		}

		// Update content if not provided via flag
		if newContent == "" && priority == 0 {
			fmt.Printf("Current content: %s\n", cfg.Rules[ruleIndex].Content)
			fmt.Println("Enter new rule content (press Ctrl+D when done, or press Enter to keep current):")
			content, err := readFromStdin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
				os.Exit(1)
			}
			if strings.TrimSpace(content) != "" {
				newContent = content
			}
		}

		// Update the rule
		if newContent != "" {
			cfg.Rules[ruleIndex].Content = newContent
		}
		if priority > 0 {
			cfg.Rules[ruleIndex].Priority = priority
		}

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated rule '%s' in %s\n", ruleName, configFile)
	},
}

// updateSectionCmd represents the update section subcommand
var updateSectionCmd = &cobra.Command{
	Use:   "section [title]",
	Short: "Update an existing section",
	Long: `Update an existing section in your AI rules configuration file.
You can update the content, priority, or both. If no flags are provided,
you'll be prompted to enter new content via stdin.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sectionTitle := args[0]
		newContent, _ := cmd.Flags().GetString("content")
		priority, _ := cmd.Flags().GetInt("priority")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Find the section to update
		sectionIndex := -1
		for i, section := range cfg.Sections {
			if section.Title == sectionTitle {
				sectionIndex = i
				break
			}
		}

		if sectionIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Section '%s' not found\n", sectionTitle)
			os.Exit(1)
		}

		// Update content if not provided via flag
		if newContent == "" && priority == 0 {
			fmt.Printf("Current content: %s\n", cfg.Sections[sectionIndex].Content)
			fmt.Println("Enter new section content (press Ctrl+D when done, or press Enter to keep current):")
			content, err := readFromStdin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
				os.Exit(1)
			}
			if strings.TrimSpace(content) != "" {
				newContent = content
			}
		}

		// Update the section
		if newContent != "" {
			cfg.Sections[sectionIndex].Content = newContent
		}
		if priority > 0 {
			cfg.Sections[sectionIndex].Priority = priority
		}

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated section '%s' in %s\n", sectionTitle, configFile)
	},
}

// updateOutputCmd represents the update output subcommand
var updateOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Update an existing output file configuration",
	Long: `Update an existing output file in your AI rules configuration.
You can update the template used for the output file.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		template, _ := cmd.Flags().GetString("template")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Find the output to update
		outputIndex := -1
		for i, output := range cfg.Outputs {
			if output.File == filename {
				outputIndex = i
				break
			}
		}

		if outputIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Output file '%s' not found\n", filename)
			os.Exit(1)
		}

		// Update the output
		cfg.Outputs[outputIndex].Template = template

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated output '%s' template to '%s' in %s\n", filename, template, configFile)
	},
}

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete rules, sections, or outputs",
	Long:  `Delete existing rules, sections, or outputs from your AI rules configuration file.`,
}

// deleteRuleCmd represents the delete rule subcommand
var deleteRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Delete an existing rule",
	Long:  `Delete an existing rule from your AI rules configuration file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ruleName := args[0]
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Find and remove the rule
		ruleIndex := -1
		for i, rule := range cfg.Rules {
			if rule.Name == ruleName {
				ruleIndex = i
				break
			}
		}

		if ruleIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Rule '%s' not found\n", ruleName)
			os.Exit(1)
		}

		// Remove the rule
		cfg.Rules = append(cfg.Rules[:ruleIndex], cfg.Rules[ruleIndex+1:]...)

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted rule '%s' from %s\n", ruleName, configFile)
	},
}

// deleteSectionCmd represents the delete section subcommand
var deleteSectionCmd = &cobra.Command{
	Use:   "section [title]",
	Short: "Delete an existing section",
	Long:  `Delete an existing section from your AI rules configuration file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sectionTitle := args[0]
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Find and remove the section
		sectionIndex := -1
		for i, section := range cfg.Sections {
			if section.Title == sectionTitle {
				sectionIndex = i
				break
			}
		}

		if sectionIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Section '%s' not found\n", sectionTitle)
			os.Exit(1)
		}

		// Remove the section
		cfg.Sections = append(cfg.Sections[:sectionIndex], cfg.Sections[sectionIndex+1:]...)

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted section '%s' from %s\n", sectionTitle, configFile)
	},
}

// deleteOutputCmd represents the delete output subcommand
var deleteOutputCmd = &cobra.Command{
	Use:   "output [filename]",
	Short: "Delete an existing output file configuration",
	Long:  `Delete an existing output file from your AI rules configuration.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		// Load existing configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Find and remove the output
		outputIndex := -1
		for i, output := range cfg.Outputs {
			if output.File == filename {
				outputIndex = i
				break
			}
		}

		if outputIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Output file '%s' not found\n", filename)
			os.Exit(1)
		}

		// Remove the output
		cfg.Outputs = append(cfg.Outputs[:outputIndex], cfg.Outputs[outputIndex+1:]...)

		// Save configuration
		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted output '%s' from %s\n", filename, configFile)
	},
}

func init() {
	generateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Process all config files recursively")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing files")
	generateCmd.Flags().BoolVar(&updateGitignore, "update-gitignore", false, "Update .gitignore files to include generated output files")
	initCmd.Flags().StringP("template", "t", "basic", "Template to use (basic, react, typescript)")

	// Add subcommands to add command
	addCmd.AddCommand(addRuleCmd)
	addCmd.AddCommand(addSectionCmd)
	addCmd.AddCommand(addOutputCmd)

	// Add subcommands to update command
	updateCmd.AddCommand(updateRuleCmd)
	updateCmd.AddCommand(updateSectionCmd)
	updateCmd.AddCommand(updateOutputCmd)

	// Add subcommands to delete command
	deleteCmd.AddCommand(deleteRuleCmd)
	deleteCmd.AddCommand(deleteSectionCmd)
	deleteCmd.AddCommand(deleteOutputCmd)

	// Add flags for add rule command
	addRuleCmd.Flags().IntP("priority", "p", 5, "Priority level for the rule (1-10)")
	addRuleCmd.Flags().StringP("config", "c", "", "Config file to add rule to (auto-discover if not provided)")

	// Add flags for add section command
	addSectionCmd.Flags().IntP("priority", "p", 5, "Priority level for the section")
	addSectionCmd.Flags().StringP("config", "c", "", "Config file to add section to (auto-discover if not provided)")

	// Add flags for add output command
	addOutputCmd.Flags().StringP("template", "t", "", "Template to use for the output (optional)")
	addOutputCmd.Flags().StringP("config", "c", "", "Config file to add output to (auto-discover if not provided)")

	// Add flags for update rule command
	updateRuleCmd.Flags().StringP("content", "", "", "New content for the rule (optional, will prompt if not provided)")
	updateRuleCmd.Flags().IntP("priority", "p", 0, "New priority level for the rule (optional)")
	updateRuleCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")

	// Add flags for update section command
	updateSectionCmd.Flags().StringP("content", "", "", "New content for the section (optional, will prompt if not provided)")
	updateSectionCmd.Flags().IntP("priority", "p", 0, "New priority level for the section (optional)")
	updateSectionCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")

	// Add flags for update output command
	updateOutputCmd.Flags().StringP("template", "t", "", "New template for the output (required)")
	updateOutputCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")
	_ = updateOutputCmd.MarkFlagRequired("template")

	// Add flags for delete commands
	deleteRuleCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")
	deleteSectionCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")
	deleteOutputCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")
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

// listProfilesCmd represents the list-profiles command
var listProfilesCmd = &cobra.Command{
	Use:   "list-profiles",
	Short: "List available built-in profiles",
	Long: `List all available built-in profiles that can be used in configuration files.

Profiles provide pre-configured sets of rules for specific project types:
- default: Core engineering best practices
- web-app: Frontend web application development
- api: REST API and backend service development  
- cli: Command-line tool development
- library: Reusable library and package development

Use profiles in your configuration with:
  profile: "web-app"
  # or
  profile: ["web-app", "api"]`,
	Run: func(cmd *cobra.Command, args []string) {
		profiles, err := config.ListAvailableProfiles()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing profiles: %v\n", err)
			os.Exit(1)
		}

		if len(profiles) == 0 {
			fmt.Println("No profiles available")
			return
		}

		fmt.Printf("Available profiles (%d):\n\n", len(profiles))

		for _, profileName := range profiles {
			profile, err := config.LoadProfile(profileName)
			if err != nil {
				fmt.Printf("  • %s (error loading: %v)\n", profileName, err)
				continue
			}

			description := "No description available"
			if profile.Metadata.Description != "" {
				description = profile.Metadata.Description
			}

			fmt.Printf("  • %s\n", profileName)
			fmt.Printf("    %s\n", description)
			fmt.Printf("    Rules: %d\n\n", len(profile.Rules))
		}

		fmt.Println("Usage:")
		fmt.Println("  profile: \"web-app\"           # Single profile")
		fmt.Println("  profile: [\"web-app\", \"api\"]   # Multiple profiles")
	},
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

	// Tool: Add rule
	addRuleTool := mcp.NewTool("add_rule",
		mcp.WithDescription("Add a new rule to the configuration file"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the rule"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The content of the rule"),
		),
		mcp.WithNumber("priority",
			mcp.Description("Priority level for the rule (default: 5)"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(addRuleTool, handleAddRule)

	// Tool: Add section
	addSectionTool := mcp.NewTool("add_section",
		mcp.WithDescription("Add a new section to the configuration file"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("The title of the section"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The content of the section"),
		),
		mcp.WithNumber("priority",
			mcp.Description("Priority level for the section (default: 5)"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(addSectionTool, handleAddSection)

	// Tool: Add output
	addOutputTool := mcp.NewTool("add_output",
		mcp.WithDescription("Add a new output file to the configuration"),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("The output filename"),
		),
		mcp.WithString("template",
			mcp.Description("Template to use for the output (optional)"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(addOutputTool, handleAddOutput)

	// Tool: Update rule
	updateRuleTool := mcp.NewTool("update_rule",
		mcp.WithDescription("Update an existing rule in the configuration"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the rule to update"),
		),
		mcp.WithString("content",
			mcp.Description("New content for the rule (optional)"),
		),
		mcp.WithNumber("priority",
			mcp.Description("New priority level for the rule (optional)"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(updateRuleTool, handleUpdateRule)

	// Tool: Update section
	updateSectionTool := mcp.NewTool("update_section",
		mcp.WithDescription("Update an existing section in the configuration"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("The title of the section to update"),
		),
		mcp.WithString("content",
			mcp.Description("New content for the section (optional)"),
		),
		mcp.WithNumber("priority",
			mcp.Description("New priority level for the section (optional)"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(updateSectionTool, handleUpdateSection)

	// Tool: Update output
	updateOutputTool := mcp.NewTool("update_output",
		mcp.WithDescription("Update an existing output file in the configuration"),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("The filename of the output to update"),
		),
		mcp.WithString("template",
			mcp.Required(),
			mcp.Description("New template for the output"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(updateOutputTool, handleUpdateOutput)

	// Tool: Delete rule
	deleteRuleTool := mcp.NewTool("delete_rule",
		mcp.WithDescription("Delete an existing rule from the configuration"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the rule to delete"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(deleteRuleTool, handleDeleteRule)

	// Tool: Delete section
	deleteSectionTool := mcp.NewTool("delete_section",
		mcp.WithDescription("Delete an existing section from the configuration"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("The title of the section to delete"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(deleteSectionTool, handleDeleteSection)

	// Tool: Delete output
	deleteOutputTool := mcp.NewTool("delete_output",
		mcp.WithDescription("Delete an existing output file from the configuration"),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("The filename of the output to delete"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(deleteOutputTool, handleDeleteOutput)
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

// readFromStdin reads content from standard input until EOF
func readFromStdin() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var content strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				content.WriteString(line)
				break
			}
			return "", err
		}
		content.WriteString(line)
	}

	return strings.TrimSpace(content.String()), nil
}

func handleAddRule(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Rule name is required"), nil
	}

	content := request.GetString("content", "")
	if content == "" {
		return mcp.NewToolResultError("Rule content is required"), nil
	}

	priority := int(request.GetFloat("priority", 5))

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Add new rule
	newRule := config.Rule{
		Name:     name,
		Priority: priority,
		Content:  content,
	}
	cfg.Rules = append(cfg.Rules, newRule)

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"rule": map[string]interface{}{
			"name":     name,
			"priority": priority,
		},
		"total_rules": len(cfg.Rules),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleAddSection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	title := request.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("Section title is required"), nil
	}

	content := request.GetString("content", "")
	if content == "" {
		return mcp.NewToolResultError("Section content is required"), nil
	}

	priority := int(request.GetFloat("priority", 5))

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Add new section
	newSection := config.Section{
		Title:    title,
		Priority: priority,
		Content:  content,
	}
	cfg.Sections = append(cfg.Sections, newSection)

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"section": map[string]interface{}{
			"title":    title,
			"priority": priority,
		},
		"total_sections": len(cfg.Sections),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleAddOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	filename := request.GetString("filename", "")
	if filename == "" {
		return mcp.NewToolResultError("Output filename is required"), nil
	}

	template := request.GetString("template", "")

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Check if output already exists
	for _, output := range cfg.Outputs {
		if output.File == filename {
			return mcp.NewToolResultError(fmt.Sprintf("Output file '%s' already exists in configuration", filename)), nil
		}
	}

	// Add new output
	newOutput := config.Output{
		File:     filename,
		Template: template,
	}
	cfg.Outputs = append(cfg.Outputs, newOutput)

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"output": map[string]interface{}{
			"file":     filename,
			"template": template,
		},
		"total_outputs": len(cfg.Outputs),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleUpdateRule(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Rule name is required"), nil
	}

	newContent := request.GetString("content", "")
	priority := int(request.GetFloat("priority", 0))

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Find the rule to update
	ruleIndex := -1
	for i, rule := range cfg.Rules {
		if rule.Name == name {
			ruleIndex = i
			break
		}
	}

	if ruleIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Rule '%s' not found", name)), nil
	}

	// Update the rule
	if newContent != "" {
		cfg.Rules[ruleIndex].Content = newContent
	}
	if priority > 0 {
		cfg.Rules[ruleIndex].Priority = priority
	}

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"rule": map[string]interface{}{
			"name":     name,
			"priority": cfg.Rules[ruleIndex].Priority,
			"updated":  true,
		},
		"total_rules": len(cfg.Rules),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleUpdateSection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	title := request.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("Section title is required"), nil
	}

	newContent := request.GetString("content", "")
	priority := int(request.GetFloat("priority", 0))

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Find the section to update
	sectionIndex := -1
	for i, section := range cfg.Sections {
		if section.Title == title {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Section '%s' not found", title)), nil
	}

	// Update the section
	if newContent != "" {
		cfg.Sections[sectionIndex].Content = newContent
	}
	if priority > 0 {
		cfg.Sections[sectionIndex].Priority = priority
	}

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"section": map[string]interface{}{
			"title":    title,
			"priority": cfg.Sections[sectionIndex].Priority,
			"updated":  true,
		},
		"total_sections": len(cfg.Sections),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleUpdateOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	filename := request.GetString("filename", "")
	if filename == "" {
		return mcp.NewToolResultError("Output filename is required"), nil
	}

	template := request.GetString("template", "")
	if template == "" {
		return mcp.NewToolResultError("Template is required"), nil
	}

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Find the output to update
	outputIndex := -1
	for i, output := range cfg.Outputs {
		if output.File == filename {
			outputIndex = i
			break
		}
	}

	if outputIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Output file '%s' not found", filename)), nil
	}

	// Update the output
	cfg.Outputs[outputIndex].Template = template

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"output": map[string]interface{}{
			"file":     filename,
			"template": template,
			"updated":  true,
		},
		"total_outputs": len(cfg.Outputs),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleDeleteRule(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Rule name is required"), nil
	}

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Find and remove the rule
	ruleIndex := -1
	for i, rule := range cfg.Rules {
		if rule.Name == name {
			ruleIndex = i
			break
		}
	}

	if ruleIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Rule '%s' not found", name)), nil
	}

	// Remove the rule
	cfg.Rules = append(cfg.Rules[:ruleIndex], cfg.Rules[ruleIndex+1:]...)

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"deleted":     name,
		"total_rules": len(cfg.Rules),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleDeleteSection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	title := request.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("Section title is required"), nil
	}

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Find and remove the section
	sectionIndex := -1
	for i, section := range cfg.Sections {
		if section.Title == title {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Section '%s' not found", title)), nil
	}

	// Remove the section
	cfg.Sections = append(cfg.Sections[:sectionIndex], cfg.Sections[sectionIndex+1:]...)

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":        true,
		"config_file":    configFile,
		"deleted":        title,
		"total_sections": len(cfg.Sections),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleDeleteOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get parameters
	filename := request.GetString("filename", "")
	if filename == "" {
		return mcp.NewToolResultError("Output filename is required"), nil
	}

	// Get config file path
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("No configuration file found: %v", err)), nil
		}
		configFile = foundConfig
	}

	// Load existing configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	// Find and remove the output
	outputIndex := -1
	for i, output := range cfg.Outputs {
		if output.File == filename {
			outputIndex = i
			break
		}
	}

	if outputIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Output file '%s' not found", filename)), nil
	}

	// Remove the output
	cfg.Outputs = append(cfg.Outputs[:outputIndex], cfg.Outputs[outputIndex+1:]...)

	// Save configuration
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":       true,
		"config_file":   configFile,
		"deleted":       filename,
		"total_outputs": len(cfg.Outputs),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}
