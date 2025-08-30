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
	"time"

	"github.com/Goldziher/ai-rulez/internal/agents"
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

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
		fmtError(err)
		os.Exit(1)
	}
}

func fmtError(err error) {
	errors.FormatError(os.Stderr, err, false, true)
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ai-rulez.yaml)")

	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
}

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
			if _, err := os.Stat(configFile); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Error: Configuration file '%s' not found\n", configFile)
				os.Exit(1)
			}
		} else {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		fmt.Println("Using config file:", configFile)

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		if dryRun {
			fmt.Println("\n=== DRY RUN MODE ===")
			fmt.Printf("Configuration: %s (v%s)\n", cfg.Metadata.Name, cfg.Metadata.Version)
			if cfg.Metadata.Description != "" {
				fmt.Printf("Description: %s\n", cfg.Metadata.Description)
			}
			fmt.Printf("\nWould generate %d output file(s):\n", len(cfg.Outputs))
			for _, output := range cfg.Outputs {
				fmt.Printf("  - %s", output.GetPath())
				if output.Template != "" {
					fmt.Printf(" (using template: %s)", output.Template)
				}
				fmt.Println()
			}
			fmt.Printf("\nTotal rules: %d\n", len(cfg.Rules))
			fmt.Printf("Total sections: %d\n", len(cfg.Sections))
			return
		}

		gen := generator.NewWithConfigFile(configFile)
		if err := gen.GenerateAll(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating files: %v\n", err)
			os.Exit(1)
		}

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
	configs, err := config.FindAllConfigFiles(".")
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}

	if len(configs) == 0 {
		fmtError(errors.ConfigNotFound("."))
		os.Exit(1)
	}

	sort.Strings(configs)

	fmt.Printf("Found %d configuration file(s)\n", len(configs))

	successCount := 0
	for _, configFile := range configs {
		fmt.Printf("\n--- Processing: %s ---\n", configFile)

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			continue
		}

		if dryRun {
			fmt.Printf("Configuration: %s (v%s)\n", cfg.Metadata.Name, cfg.Metadata.Version)
			fmt.Printf("Would generate %d output file(s)\n", len(cfg.Outputs))
			successCount++
			continue
		}

		gen := generator.NewWithConfigFile(configFile)
		if err := gen.GenerateAll(cfg); err != nil {
			fmtError(err)
			continue
		}

		if updateGitignore {
			if err := gitignore.UpdateGitignoreFiles(configFile, cfg); err != nil {
				fmtError(err)
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
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Configuration is valid: %s\n", configFile)
		fmt.Printf("  Name: %s\n", cfg.Metadata.Name)
		fmt.Printf("  Version: %s\n", cfg.Metadata.Version)
		fmt.Printf("  Rules: %d\n", len(cfg.Rules))
		fmt.Printf("  Sections: %d\n", len(cfg.Sections))
		fmt.Printf("  Outputs: %d\n", len(cfg.Outputs))
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of ai-rulez",
	Long:  `Print the version of ai-rulez CLI tool.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("ai-rulez version %s\n", Version)
	},
}

var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new AI rules project",
	Long: `Initialize a new AI rules project with a basic configuration file
and example rules. This creates an ai_rulez.yaml file in the current directory.

Provider Flags:
  --claude     Enable Claude (CLAUDE.md + agents) [DEFAULT]
  --amp        Enable AMP - Sourcegraph (AGENTS.md)
  --codex      Enable Codex - OpenAI (AGENTS.md)
  --cursor     Enable Cursor (AGENTS.md + .cursor/rules/)
  --windsurf   Enable Windsurf (.windsurf/)
  --gemini     Enable Gemini (GEMINI.md)
  --copilot       Enable GitHub Copilot (.github/copilot-instructions.md)
  --cline         Enable Cline (.clinerules/)
  --continue-dev  Enable Continue.dev (.continue/rules/)

Convenience Flags:
  --preset <tool>  Use a preset for a specific tool (e.g., --preset claude)
                   Available presets: claude, amp, codex, cursor, gemini, 
                   windsurf, copilot, cline, continue
  --all            Enable all supported providers
  --popular        Enable Claude, Cursor, Windsurf, Copilot (most common)

Optionally, use --setup-hooks to automatically configure git hooks for ai-rulez
validation if lefthook, pre-commit, or husky is detected in your project.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := "My Project"
		if len(args) > 0 {
			projectName = args[0]
		}

		listAgents, _ := cmd.Flags().GetBool("list-agents")
		if listAgents {
			available, _ := agents.DetectAvailableAgents()
			if len(available) == 0 {
				fmt.Println("No AI agents found. Install one of: claude, amp, codex, cursor, gemini")
			} else {
				fmt.Println("Available AI agents:")
				for _, agent := range available {
					fmt.Printf("  - %s: %s\n", agent.ID, agent.Display)
				}
			}
			os.Exit(0)
		}

		if _, err := os.Stat("ai_rulez.yaml"); err == nil {
			fmtError(errors.New(errors.ErrorTypeCommand, "init project",
				fmt.Errorf("configuration file already exists")).
				WithPath("ai_rulez.yaml").
				WithSuggestion("Use a different directory or remove the existing file").
				WithSuggestion("Try 'ai-rulez generate' if you want to regenerate files"))
			os.Exit(1)
		}

		presetFlag, _ := cmd.Flags().GetString("preset")
		var providers ProviderConfig

		if presetFlag != "" {
			if err := agents.ValidatePreset(presetFlag); err != nil {
				fmtError(errors.New(errors.ErrorTypeCommand, "init", err))
				os.Exit(1)
			}

			presetProviders := agents.GetPresetProviders(presetFlag)
			presetOptions := agents.GetPresetOptions(presetFlag)

			providers = ProviderConfig{
				Enabled:      presetProviders,
				WithAgents:   presetOptions.WithAgents,
				WithSections: presetOptions.WithSections,
				NoComments:   presetOptions.NoComments,
			}
		} else {
			providers = parseProviderFlags(cmd)
		}

		noAgent, _ := cmd.Flags().GetBool("no-agent")
		useAgentFlag, _ := cmd.Flags().GetString("use-agent")

		var configContent string
		var useAIAgent bool

		if !noAgent && (useAgentFlag != "" || shouldPromptForAgent()) {
			configContent, useAIAgent = handleAgentGeneration(cmd, projectName, providers, useAgentFlag)
		}

		if useAIAgent && configContent != "" {
			if err := os.WriteFile("ai_rulez.yaml", []byte(configContent), 0o644); err != nil {
				fmtError(errors.FileWrite("ai_rulez.yaml", err))
				os.Exit(1)
			}
			fmt.Printf("✓ Generated ai_rulez.yaml with AI assistance\n")
		} else {
			if err := createProviderConfigFile(projectName, "ai_rulez.yaml", providers); err != nil {
				fmtError(err)
				os.Exit(1)
			}
			fmt.Printf("✓ Initialized new AI rules project: %s\n", projectName)
			fmt.Println("  - Created ai_rulez.yaml")
		}

		if len(providers.Enabled) > 0 {
			fmt.Printf("  - Configured for: %s\n", strings.Join(providers.Enabled, ", "))
		}

		fmt.Println("  - Run 'ai-rulez generate' to create rule files")

		setupHooks, _ := cmd.Flags().GetBool("setup-hooks")
		if setupHooks {
			fmt.Println("\nDetecting git hook managers...")

			if detectLefthook() {
				fmt.Println("  - Found lefthook configuration")
				if err := setupLefthookConfig(); err != nil {
					fmt.Printf("    Warning: Failed to setup lefthook: %v\n", err)
				}
			} else if detectPreCommit() {
				fmt.Println("  - Found pre-commit configuration")
				if err := setupPreCommitConfig(); err != nil {
					fmt.Printf("    Warning: Failed to setup pre-commit: %v\n", err)
				}
			} else if detectHusky() {
				fmt.Println("  - Found husky configuration")
				if err := setupHuskyConfig(); err != nil {
					fmt.Printf("    Warning: Failed to setup husky: %v\n", err)
				}
			} else {
				fmt.Println("  - No git hook manager detected")
				fmt.Println("    Install lefthook, pre-commit, or husky to enable automatic validation")
			}
		}
	},
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add rules or sections to configuration",
	Long:  `Add new rules or sections to your AI rules configuration file.`,
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Println("Enter rule content (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
			os.Exit(1)
		}

		newRule := config.Rule{
			Name:     ruleName,
			Priority: priority,
			Content:  content,
		}
		cfg.Rules = append(cfg.Rules, newRule)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added rule '%s' with priority %d to %s\n", ruleName, priority, configFile)
	},
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Println("Enter section content (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading content: %v\n", err)
			os.Exit(1)
		}

		newSection := config.Section{
			Title:    sectionTitle,
			Priority: priority,
			Content:  content,
		}
		cfg.Sections = append(cfg.Sections, newSection)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added section '%s' with priority %d to %s\n", sectionTitle, priority, configFile)
	},
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		for _, output := range cfg.Outputs {
			if output.GetPath() == filename {
				fmt.Fprintf(os.Stderr, "Error: Output file '%s' already exists in configuration\n", filename)
				os.Exit(1)
			}
		}

		newOutput := config.Output{
			Path:     filename,
			Template: template,
		}
		cfg.Outputs = append(cfg.Outputs, newOutput)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added output '%s'", filename)
		if template != "" {
			fmt.Printf(" with template '%s'", template)
		}
		fmt.Printf(" to %s\n", configFile)
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update existing rules, sections, or outputs",
	Long:  `Update existing rules, sections, or outputs in your AI rules configuration file.`,
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

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

		if newContent != "" {
			cfg.Rules[ruleIndex].Content = newContent
		}
		if priority > 0 {
			cfg.Rules[ruleIndex].Priority = priority
		}

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated rule '%s' in %s\n", ruleName, configFile)
	},
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

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

		if newContent != "" {
			cfg.Sections[sectionIndex].Content = newContent
		}
		if priority > 0 {
			cfg.Sections[sectionIndex].Priority = priority
		}

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated section '%s' in %s\n", sectionTitle, configFile)
	},
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		outputIndex := -1
		for i, output := range cfg.Outputs {
			if output.GetPath() == filename {
				outputIndex = i
				break
			}
		}

		if outputIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Output file '%s' not found\n", filename)
			os.Exit(1)
		}

		cfg.Outputs[outputIndex].Template = template

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated output '%s' template to '%s' in %s\n", filename, template, configFile)
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete rules, sections, or outputs",
	Long:  `Delete existing rules, sections, or outputs from your AI rules configuration file.`,
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

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

		cfg.Rules = append(cfg.Rules[:ruleIndex], cfg.Rules[ruleIndex+1:]...)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted rule '%s' from %s\n", ruleName, configFile)
	},
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

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

		cfg.Sections = append(cfg.Sections[:sectionIndex], cfg.Sections[sectionIndex+1:]...)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted section '%s' from %s\n", sectionTitle, configFile)
	},
}

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
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		outputIndex := -1
		for i, output := range cfg.Outputs {
			if output.GetPath() == filename {
				outputIndex = i
				break
			}
		}

		if outputIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Output file '%s' not found\n", filename)
			os.Exit(1)
		}

		cfg.Outputs = append(cfg.Outputs[:outputIndex], cfg.Outputs[outputIndex+1:]...)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted output '%s' from %s\n", filename, configFile)
	},
}

var addAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Add a new agent to configuration",
	Long: `Add a new agent to your AI rules configuration file.
The agent name is provided as an argument, and you can specify
description, priority, tools, and system prompt. The system prompt
will be read from stdin if not provided via flag.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentName := args[0]
		description, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetInt("priority")
		tools, _ := cmd.Flags().GetStringSlice("tools")
		systemPrompt, _ := cmd.Flags().GetString("system-prompt")
		id, _ := cmd.Flags().GetString("id")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		for _, agent := range cfg.Agents {
			if agent.Name == agentName {
				fmt.Fprintf(os.Stderr, "Error: Agent '%s' already exists in configuration\n", agentName)
				os.Exit(1)
			}
		}

		if systemPrompt == "" {
			fmt.Println("Enter agent system prompt (press Ctrl+D when done):")
			content, err := readFromStdin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading system prompt: %v\n", err)
				os.Exit(1)
			}
			systemPrompt = content
		}

		newAgent := config.Agent{
			ID:           id,
			Name:         agentName,
			Description:  description,
			Priority:     priority,
			Tools:        tools,
			SystemPrompt: systemPrompt,
		}
		cfg.Agents = append(cfg.Agents, newAgent)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added agent '%s'", agentName)
		if priority > 0 {
			fmt.Printf(" with priority %d", priority)
		}
		fmt.Printf(" to %s\n", configFile)
	},
}

var updateAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Update an existing agent",
	Long: `Update an existing agent in your AI rules configuration file.
You can update the description, priority, tools, or system prompt.
If no system prompt is provided via flag, you'll be prompted to enter
it via stdin.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentName := args[0]
		description, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetInt("priority")
		tools, _ := cmd.Flags().GetStringSlice("tools")
		systemPrompt, _ := cmd.Flags().GetString("system-prompt")
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		agentIndex := -1
		for i, agent := range cfg.Agents {
			if agent.Name == agentName {
				agentIndex = i
				break
			}
		}

		if agentIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Agent '%s' not found\n", agentName)
			os.Exit(1)
		}

		if systemPrompt == "" && description == "" && priority == 0 && len(tools) == 0 {
			fmt.Printf("Current system prompt: %s\n", cfg.Agents[agentIndex].SystemPrompt)
			fmt.Println("Enter new system prompt (press Ctrl+D when done, or press Enter to keep current):")
			content, err := readFromStdin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading system prompt: %v\n", err)
				os.Exit(1)
			}
			if strings.TrimSpace(content) != "" {
				systemPrompt = content
			}
		}

		if description != "" {
			cfg.Agents[agentIndex].Description = description
		}
		if priority > 0 {
			cfg.Agents[agentIndex].Priority = priority
		}
		if len(tools) > 0 {
			cfg.Agents[agentIndex].Tools = tools
		}
		if systemPrompt != "" {
			cfg.Agents[agentIndex].SystemPrompt = systemPrompt
		}

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated agent '%s' in %s\n", agentName, configFile)
	},
}

var deleteAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Delete an existing agent",
	Long: `Delete an existing agent from your AI rules configuration file.
This will permanently remove the agent and cannot be undone.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentName := args[0]
		configFile, _ := cmd.Flags().GetString("config")

		if configFile == "" {
			foundConfig, err := config.FindConfigFile(".")
			if err != nil {
				fmtError(err)
				os.Exit(1)
			}
			configFile = foundConfig
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}

		agentIndex := -1
		for i, agent := range cfg.Agents {
			if agent.Name == agentName {
				agentIndex = i
				break
			}
		}

		if agentIndex == -1 {
			fmt.Fprintf(os.Stderr, "Error: Agent '%s' not found\n", agentName)
			os.Exit(1)
		}

		cfg.Agents = append(cfg.Agents[:agentIndex], cfg.Agents[agentIndex+1:]...)

		if err := config.SaveConfig(cfg, configFile); err != nil {
			fmtError(err)
			os.Exit(1)
		}

		fmt.Printf("✓ Deleted agent '%s' from %s\n", agentName, configFile)
	},
}

func init() {
	generateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Process all config files recursively")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing files")
	generateCmd.Flags().BoolVar(&updateGitignore, "update-gitignore", false, "Update .gitignore files to include generated output files")

	initCmd.Flags().Bool("claude", false, "Enable Claude (CLAUDE.md + agents)")
	initCmd.Flags().Bool("amp", false, "Enable AMP - Sourcegraph (AGENTS.md)")
	initCmd.Flags().Bool("codex", false, "Enable Codex - OpenAI (AGENTS.md)")
	initCmd.Flags().Bool("cursor", false, "Enable Cursor (AGENTS.md + .cursor/rules/)")
	initCmd.Flags().Bool("windsurf", false, "Enable Windsurf (.windsurf/)")
	initCmd.Flags().Bool("gemini", false, "Enable Gemini (GEMINI.md)")
	initCmd.Flags().Bool("copilot", false, "Enable GitHub Copilot (.github/copilot-instructions.md)")
	initCmd.Flags().Bool("cline", false, "Enable Cline (.clinerules/)")
	initCmd.Flags().Bool("continue-dev", false, "Enable Continue.dev (.continue/rules/)")

	initCmd.Flags().Bool("all", false, "Enable all major providers")
	initCmd.Flags().Bool("popular", false, "Enable Claude, Cursor, Windsurf, Copilot")

	initCmd.Flags().Bool("with-agents", false, "Include example agent configurations")
	initCmd.Flags().Bool("with-sections", false, "Include example sections in config")
	initCmd.Flags().Bool("no-comments", false, "Skip explanatory comments in generated files")

	initCmd.Flags().String("preset", "", "Use a preset configuration for a specific tool (claude, cursor, gemini, etc.)")

	initCmd.Flags().String("use-agent", "", "Use AI agent(s) to generate config (comma-separated: claude,amp,codex,cursor,gemini)")
	initCmd.Flags().Bool("no-agent", false, "Skip AI agent detection and usage")
	initCmd.Flags().Bool("list-agents", false, "List available AI agents and exit")

	initCmd.Flags().Bool("setup-hooks", false, "Auto-detect and setup git hooks (lefthook, pre-commit, or husky)")

	addCmd.AddCommand(addRuleCmd)
	addCmd.AddCommand(addSectionCmd)
	addCmd.AddCommand(addOutputCmd)
	addCmd.AddCommand(addAgentCmd)

	updateCmd.AddCommand(updateRuleCmd)
	updateCmd.AddCommand(updateSectionCmd)
	updateCmd.AddCommand(updateOutputCmd)
	updateCmd.AddCommand(updateAgentCmd)

	deleteCmd.AddCommand(deleteRuleCmd)
	deleteCmd.AddCommand(deleteSectionCmd)
	deleteCmd.AddCommand(deleteOutputCmd)
	deleteCmd.AddCommand(deleteAgentCmd)

	addRuleCmd.Flags().IntP("priority", "p", 5, "Priority level for the rule (1-10)")
	addRuleCmd.Flags().StringP("config", "c", "", "Config file to add rule to (auto-discover if not provided)")

	addSectionCmd.Flags().IntP("priority", "p", 5, "Priority level for the section")
	addSectionCmd.Flags().StringP("config", "c", "", "Config file to add section to (auto-discover if not provided)")

	addOutputCmd.Flags().StringP("template", "t", "", "Template to use for the output (optional)")
	addOutputCmd.Flags().StringP("config", "c", "", "Config file to add output to (auto-discover if not provided)")

	updateRuleCmd.Flags().StringP("content", "", "", "New content for the rule (optional, will prompt if not provided)")
	updateRuleCmd.Flags().IntP("priority", "p", 0, "New priority level for the rule (optional)")
	updateRuleCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")

	updateSectionCmd.Flags().StringP("content", "", "", "New content for the section (optional, will prompt if not provided)")
	updateSectionCmd.Flags().IntP("priority", "p", 0, "New priority level for the section (optional)")
	updateSectionCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")

	updateOutputCmd.Flags().StringP("template", "t", "", "New template for the output (required)")
	updateOutputCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")
	if err := updateOutputCmd.MarkFlagRequired("template"); err != nil {
		fmt.Fprintf(os.Stderr, "Error marking flag as required: %v\n", err)
	}

	deleteRuleCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")
	deleteSectionCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")
	deleteOutputCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")

	addAgentCmd.Flags().StringP("description", "d", "", "Description of the agent")
	addAgentCmd.Flags().IntP("priority", "p", 5, "Priority level for the agent (1-10)")
	addAgentCmd.Flags().StringSliceP("tools", "t", []string{}, "Comma-separated list of tools the agent can use")
	addAgentCmd.Flags().StringP("system-prompt", "s", "", "System prompt for the agent (will prompt via stdin if not provided)")
	addAgentCmd.Flags().StringP("id", "", "", "Optional unique identifier for the agent")
	addAgentCmd.Flags().StringP("config", "c", "", "Config file to add agent to (auto-discover if not provided)")

	updateAgentCmd.Flags().StringP("description", "d", "", "New description for the agent")
	updateAgentCmd.Flags().IntP("priority", "p", 0, "New priority level for the agent (1-10)")
	updateAgentCmd.Flags().StringSliceP("tools", "t", []string{}, "New comma-separated list of tools the agent can use")
	updateAgentCmd.Flags().StringP("system-prompt", "s", "", "New system prompt for the agent (will prompt via stdin if not provided)")
	updateAgentCmd.Flags().StringP("config", "c", "", "Config file to update (auto-discover if not provided)")

	deleteAgentCmd.Flags().StringP("config", "c", "", "Config file to delete from (auto-discover if not provided)")
}

type ProviderConfig struct {
	Enabled      []string
	WithAgents   bool
	WithSections bool
	NoComments   bool
}

func parseProviderFlags(cmd *cobra.Command) ProviderConfig {
	providerConfig := ProviderConfig{
		Enabled: []string{},
	}

	providerConfig.WithAgents, _ = cmd.Flags().GetBool("with-agents")
	providerConfig.WithSections, _ = cmd.Flags().GetBool("with-sections")
	providerConfig.NoComments, _ = cmd.Flags().GetBool("no-comments")

	all, _ := cmd.Flags().GetBool("all")
	popular, _ := cmd.Flags().GetBool("popular")

	if all {
		providerConfig.Enabled = []string{"claude", "amp", "codex", "cursor", "windsurf", "gemini", "copilot", "cline", "continue-dev"}
		return providerConfig
	}

	if popular {
		providerConfig.Enabled = []string{"claude", "cursor", "windsurf", "copilot"}
		return providerConfig
	}

	hasExplicitProvider := false
	providers := []string{"claude", "amp", "codex", "cursor", "windsurf", "gemini", "copilot", "cline", "continue-dev"}
	for _, provider := range providers {
		enabled, _ := cmd.Flags().GetBool(provider)
		if enabled {
			providerConfig.Enabled = append(providerConfig.Enabled, provider)
			hasExplicitProvider = true
			if provider == "claude" && !cmd.Flags().Changed("with-agents") {
				providerConfig.WithAgents = true
			}
		}
	}

	if !hasExplicitProvider {
		providerConfig.Enabled = []string{"claude"}
		providerConfig.WithAgents = true
	}

	return providerConfig
}

func createProviderConfigFile(projectName, filename string, providers ProviderConfig) error {
	template := generateConfigTemplate(projectName, providers)

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.FileWrite(dir, err).
			WithContext("operation", "create directory").
			WithSuggestion("Check if you have write permissions for the parent directory")
	}

	if err := os.WriteFile(filename, []byte(template), 0o644); err != nil {
		return errors.FileWrite(filename, err)
	}

	return nil
}

func generateConfigTemplate(projectName string, providers ProviderConfig) string {
	var builder strings.Builder

	builder.WriteString(`$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

`)

	builder.WriteString(fmt.Sprintf(`metadata:
  name: "%s"
  version: "1.0.0"
  description: "AI assistant rules configuration"

`, projectName))

	if !providers.NoComments {
		builder.WriteString(`# Uncomment and customize remote includes to share rules across projects:
# includes:
#   - "https://raw.githubusercontent.com/myorg/standards/main/coding-standards.yaml"
#   - "./local-rules.yaml"

`)
	}

	builder.WriteString(`# Output configurations - where to generate AI assistant files
outputs:`)

	var processedProviders []string

	for _, provider := range providers.Enabled {
		switch provider {
		case "claude":
			builder.WriteString(`
  # Claude configuration
  - file: "CLAUDE.md"`)
			if providers.WithAgents {
				builder.WriteString(`
  
  # Claude agents (specialized sub-assistants)
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"`)
			}
		case "amp", "codex", "cursor":
			if !contains(processedProviders, "AGENTS.md") {
				builder.WriteString(`
  
  # AGENTS.md (shared by AMP, Codex, Cursor)
  - file: "AGENTS.md"`)
				processedProviders = append(processedProviders, "AGENTS.md")
			}
			if provider == "cursor" {
				builder.WriteString(`
  
  # Cursor editor rules
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "rules.mdc"`)
			}
		case "windsurf":
			builder.WriteString(`
  
  # Windsurf editor (new directory format)
  - path: ".windsurf/"
    type: "rule"
    naming_scheme: "{name}.md"`)
		case "gemini":
			builder.WriteString(`
  
  # Google Gemini
  - file: "GEMINI.md"`)
		case "copilot":
			builder.WriteString(`
  
  # GitHub Copilot repository instructions
  - file: ".github/copilot-instructions.md"`)
		case "cline":
			builder.WriteString(`
  
  # Cline rules (directory format)
  - path: ".clinerules/"
    type: "rule"
    naming_scheme: "{priority:02d}-{name}.md"`)
		case "continue-dev":
			builder.WriteString(`
  
  # Continue.dev rules (directory format with frontmatter)
  - path: ".continue/rules/"
    type: "rule"
    naming_scheme: "{priority:02d}-{name}.md"`)
		}
	}

	builder.WriteString("\n\n")

	if providers.WithAgents && sliceContains(providers.Enabled, "claude") {
		if providers.NoComments {
			builder.WriteString(`agents:`)
		} else {
			builder.WriteString(`# AI agents (specialized sub-assistants for Claude)
agents:`)
		}
		builder.WriteString(`
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
      - Add inline comments for non-obvious code
      - Include usage examples in documentation

  - name: "Error Handling"
    priority: 8
    content: |
      - Always handle errors appropriately
      - Provide meaningful error messages to users
      - Log errors with sufficient context for debugging
      - Fail fast and fail clearly
      - Use proper exception handling patterns

  - name: "Security"
    priority: 7
    content: |
      - Validate all user inputs and sanitize data
      - Use parameterized queries to prevent SQL injection
      - Implement proper authentication and authorization
      - Keep dependencies up to date
      - Follow security best practices for your technology stack
`)

	if providers.WithSections {
		builder.WriteString(`
sections:
  - title: "Project Overview"
    priority: 20
    content: |
      ## Getting Started
      
      This project uses ai-rulez for managing AI assistant configurations.
      
      **Key Commands:**
      - ` + "`ai-rulez generate`" + ` - Generate AI assistant files
      - ` + "`ai-rulez validate`" + ` - Validate configuration
      - ` + "`ai-rulez add rule \"Name\"`" + ` - Add new rule interactively
      
      **Configuration:**
      Edit this file (ai_rulez.yaml) to customize rules and outputs for your project.
`)
	}

	return builder.String()
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for AI assistant integration",
	Long: `Start an MCP (Model Context Protocol) server that exposes ai-rulez functionality 
to AI assistants like Claude Desktop, Cursor, and other MCP-compatible tools.

The server runs in stdio mode and provides tools for:
- Retrieving rules, sections, and agents with filtering options
- Generating output files with gitignore integration
- Validating configurations
- Adding, updating, and deleting rules, sections, outputs, and agents
- Support for rule/section/agent IDs for precise local overrides

Configure in your AI assistant by adding this server to the MCP configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		runMCPServer()
	},
}

func runMCPServer() {
	s := server.NewMCPServer(
		"ai-rulez",
		Version,
		server.WithToolCapabilities(false),
	)

	addAIRulezTools(s)

	if err := server.ServeStdio(s); err != nil {
		fmtError(errors.MCPError("start MCP server", err))
		os.Exit(1)
	}
}

func addAIRulezTools(s *server.MCPServer) {
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

	getSectionsTool := mcp.NewTool("get_sections",
		mcp.WithDescription("Get documentation sections from configuration"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(getSectionsTool, handleGetSections)

	generateTool := mcp.NewTool("generate_output",
		mcp.WithDescription("Generate AI rules output files"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Show what would be generated without writing files (default: false)"),
		),
		mcp.WithBoolean("update_gitignore",
			mcp.Description("Update .gitignore files to include generated output files (default: false)"),
		),
		mcp.WithBoolean("recursive",
			mcp.Description("Process all config files recursively (default: false)"),
		),
	)
	s.AddTool(generateTool, handleGenerate)

	validateTool := mcp.NewTool("validate_config",
		mcp.WithDescription("Validate AI rules configuration file"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(validateTool, handleValidate)

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
		mcp.WithString("id",
			mcp.Description("Optional unique identifier for precise overriding in local configs"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(addRuleTool, handleAddRule)

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
		mcp.WithString("id",
			mcp.Description("Optional unique identifier for precise overriding in local configs"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(addSectionTool, handleAddSection)

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

	getAgentsTool := mcp.NewTool("get_agents",
		mcp.WithDescription("Get AI agents from configuration"),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
		mcp.WithString("name_filter",
			mcp.Description("Filter agents by name (case-insensitive substring match, optional)"),
		),
	)
	s.AddTool(getAgentsTool, handleGetAgents)

	addAgentTool := mcp.NewTool("add_agent",
		mcp.WithDescription("Add a new agent to the configuration file"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the agent"),
		),
		mcp.WithString("system_prompt",
			mcp.Required(),
			mcp.Description("The system prompt for the agent"),
		),
		mcp.WithString("description",
			mcp.Description("Description of the agent (optional)"),
		),
		mcp.WithNumber("priority",
			mcp.Description("Priority level for the agent (default: 5)"),
		),
		mcp.WithString("tools",
			mcp.Description("Comma-separated list of tools the agent can use (optional)"),
		),
		mcp.WithString("id",
			mcp.Description("Optional unique identifier for precise overriding in local configs"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(addAgentTool, handleAddAgent)

	updateAgentTool := mcp.NewTool("update_agent",
		mcp.WithDescription("Update an existing agent in the configuration"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the agent to update"),
		),
		mcp.WithString("system_prompt",
			mcp.Description("New system prompt for the agent (optional)"),
		),
		mcp.WithString("description",
			mcp.Description("New description for the agent (optional)"),
		),
		mcp.WithNumber("priority",
			mcp.Description("New priority level for the agent (optional)"),
		),
		mcp.WithString("tools",
			mcp.Description("New comma-separated list of tools the agent can use (optional)"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(updateAgentTool, handleUpdateAgent)

	deleteAgentTool := mcp.NewTool("delete_agent",
		mcp.WithDescription("Delete an existing agent from the configuration"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name of the agent to delete"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (optional, will auto-discover if not provided)"),
		),
	)
	s.AddTool(deleteAgentTool, handleDeleteAgent)

	initProjectTool := mcp.NewTool("init_project",
		mcp.WithDescription("Initialize a new AI rules project with configuration file"),
		mcp.WithString("project_name",
			mcp.Description("Name of the project (default: 'My Project')"),
		),
		mcp.WithString("config_file",
			mcp.Description("Path to configuration file (default: 'ai_rulez.yaml')"),
		),
		mcp.WithBoolean("claude",
			mcp.Description("Enable Claude (CLAUDE.md + agents) - default if no providers specified"),
		),
		mcp.WithBoolean("cursor",
			mcp.Description("Enable Cursor (.cursor/rules/)"),
		),
		mcp.WithBoolean("windsurf",
			mcp.Description("Enable Windsurf (.windsurfrules)"),
		),
		mcp.WithBoolean("gemini",
			mcp.Description("Enable Gemini (GEMINI.md)"),
		),
		mcp.WithBoolean("copilot",
			mcp.Description("Enable GitHub Copilot (.github/copilot-instructions.md)"),
		),
		mcp.WithBoolean("cline",
			mcp.Description("Enable Cline (.clinerules/)"),
		),
		mcp.WithBoolean("continue",
			mcp.Description("Enable Continue.dev (.continuerules)"),
		),
		mcp.WithBoolean("all",
			mcp.Description("Enable all major providers"),
		),
		mcp.WithBoolean("popular",
			mcp.Description("Enable Claude, Cursor, Windsurf, Copilot (most common)"),
		),
		mcp.WithBoolean("minimal",
			mcp.Description("Only Claude without agents or sections"),
		),
		mcp.WithBoolean("with_agents",
			mcp.Description("Include example agent configurations"),
		),
		mcp.WithBoolean("with_sections",
			mcp.Description("Include example sections in config"),
		),
		mcp.WithBoolean("no_comments",
			mcp.Description("Skip explanatory comments in generated files"),
		),
		mcp.WithBoolean("setup_hooks",
			mcp.Description("Auto-detect and setup git hooks (lefthook, pre-commit, or husky)"),
		),
	)
	s.AddTool(initProjectTool, handleInitProject)

	versionTool := mcp.NewTool("get_version",
		mcp.WithDescription("Get the version of ai-rulez"),
	)
	s.AddTool(versionTool, handleGetVersion)
}

func handleGetRules(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
	}

	minPriority := request.GetFloat("min_priority", 0)
	nameFilter := request.GetString("name_filter", "")

	var filteredRules []config.Rule
	for _, rule := range cfg.Rules {
		if minPriority > 0 && float64(rule.Priority) < minPriority {
			continue
		}
		if nameFilter != "" && !strings.Contains(strings.ToLower(rule.Name), strings.ToLower(nameFilter)) {
			continue
		}
		filteredRules = append(filteredRules, rule)
	}

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
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
	}

	result := map[string]interface{}{
		"config_file":    configFile,
		"total_sections": len(cfg.Sections),
		"sections":       cfg.Sections,
		"metadata":       cfg.Metadata,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

//nolint:gocyclo // This function handles multiple generation scenarios
func handleGenerate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	recursive := request.GetBool("recursive", false)
	dryRun := request.GetBool("dry_run", false)
	updateGitignore := request.GetBool("update_gitignore", false)

	if recursive {
		configs, err := config.FindAllConfigFiles(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}

		if len(configs) == 0 {
			return mcp.NewToolResultError("No configuration files found"), nil
		}

		sort.Strings(configs)

		result := map[string]interface{}{
			"success":          true,
			"recursive":        true,
			"configs_found":    len(configs),
			"configs":          configs,
			"dry_run":          dryRun,
			"update_gitignore": updateGitignore,
		}

		if dryRun {
			result["message"] = "Dry run: would process all config files recursively"
			jsonResult, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		successCount := 0
		errorMsgs := []string{}

		for _, configFile := range configs {
			cfg, err := config.LoadConfig(configFile)
			if err != nil {
				errorMsgs = append(errorMsgs, fmt.Sprintf("%s: %v", configFile, err))
				continue
			}

			gen := generator.NewWithConfigFile(configFile)
			err = gen.GenerateAll(cfg)
			if err != nil {
				errorMsgs = append(errorMsgs, fmt.Sprintf("%s: %v", configFile, err))
				continue
			}
			successCount++
		}

		result["processed"] = successCount
		result["failed"] = len(errorMsgs)
		if len(errorMsgs) > 0 {
			result["errors"] = errorMsgs
		}

		jsonResult, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(jsonResult)), nil
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
	}

	if dryRun {
		result := map[string]interface{}{
			"config_file":      configFile,
			"dry_run":          true,
			"would_generate":   len(cfg.Outputs),
			"outputs":          cfg.Outputs,
			"metadata":         cfg.Metadata,
			"total_rules":      len(cfg.Rules),
			"total_sections":   len(cfg.Sections),
			"update_gitignore": updateGitignore,
		}
		jsonResult, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(jsonResult)), nil
	}

	gen := generator.NewWithConfigFile(configFile)
	err = gen.GenerateAll(cfg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error generating files: %v", err)), nil
	}

	if updateGitignore {
		if err := gitignore.UpdateGitignoreFiles(configFile, cfg); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error updating .gitignore: %v", err)), nil
		}
	}

	result := map[string]interface{}{
		"config_file":      configFile,
		"files_generated":  len(cfg.Outputs),
		"outputs":          cfg.Outputs,
		"success":          true,
		"update_gitignore": updateGitignore,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleValidate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

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

func shouldPromptForAgent() bool {
	if os.Getenv("CI") != "" || os.Getenv("NO_INTERACTIVE") != "" {
		return false
	}

	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func handleAgentGeneration(cmd *cobra.Command, projectName string, providers ProviderConfig, useAgentFlag string) (string, bool) {
	var selectedAgents []agents.AgentInfo
	var err error

	if useAgentFlag != "" {
		selectedAgents, err = agents.ParseAgentList(useAgentFlag)
		if err != nil {
			fmt.Printf("Error parsing agent list: %v\n", err)
			return "", false
		}
	} else {
		available, err := agents.DetectAvailableAgents()
		if err != nil || len(available) == 0 {
			return "", false
		}

		fmt.Println("\nDetected AI assistants:")
		for i, agent := range available {
			fmt.Printf("  %d. %s\n", i+1, agent.Display)
		}
		fmt.Print("\nSelect AI assistant [1-", len(available), "] or press Enter to skip: ")

		var choice string
		fmt.Scanln(&choice)

		if choice == "" {
			return "", false
		}

		var idx int
		if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(available) {
			fmt.Println("Invalid selection")
			return "", false
		}

		selectedAgents = []agents.AgentInfo{available[idx-1]}
	}

	for _, agent := range selectedAgents {
		fmt.Printf("\nUsing %s to generate configuration...\n", agent.Display)

		prompt := agents.GeneratePrompt(agent, projectName, providers.Enabled)

		maxRetries := 3
		timeout := 30 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			output, err := agents.InvokeAgent(agent, prompt, timeout)
			if err != nil {
				fmt.Printf("  Attempt %d failed: %v\n", attempt, err)
				continue
			}

			yamlContent := agents.ExtractYAMLFromResponse(output)

			validationErrors, err := agents.ValidateGeneratedConfig(yamlContent)
			if err == nil {
				return yamlContent, true
			}

			if len(validationErrors) > 0 && attempt < maxRetries {
				fmt.Printf("  Validation failed, retrying with feedback...\n")
				prompt = agents.GenerateFeedbackPrompt(prompt, yamlContent, validationErrors)
			}
		}

		fmt.Printf("  Failed to generate valid configuration with %s\n", agent.Display)
	}

	return "", false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func detectLefthook() bool {
	files := []string{"lefthook.yaml", "lefthook.yml", ".lefthook.yml"}
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			return true
		}
	}
	return false
}

func detectPreCommit() bool {
	files := []string{".pre-commit-config.yaml", ".pre-commit-config.yml"}
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			return true
		}
	}
	return false
}

func detectHusky() bool {
	if info, err := os.Stat(".husky"); err == nil && info.IsDir() {
		return true
	}
	return false
}

func setupLefthookConfig() error {
	configFile := ""
	files := []string{"lefthook.yaml", "lefthook.yml", ".lefthook.yml"}
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			configFile = file
			break
		}
	}

	if configFile == "" {
		return fmt.Errorf("lefthook configuration file not found")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read lefthook config: %w", err)
	}

	var configData map[string]interface{}
	if err := yaml.Unmarshal(data, &configData); err != nil {
		return fmt.Errorf("failed to parse lefthook config: %w", err)
	}

	if preCommit, ok := configData["pre-commit"].(map[string]interface{}); ok {
		if commands, ok := preCommit["commands"].(map[string]interface{}); ok {
			if _, exists := commands["ai-rulez"]; !exists {
				commands["ai-rulez"] = map[string]interface{}{
					"glob":      "**/*.{ai-rulez,ai_rulez}.{yaml,yml}",
					"run":       "ai-rulez generate --dry-run",
					"fail_text": "AI rules validation failed",
				}

				updatedData, err := yaml.Marshal(configData)
				if err != nil {
					return fmt.Errorf("failed to marshal updated config: %w", err)
				}

				if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
					return fmt.Errorf("failed to write updated config: %w", err)
				}

				fmt.Printf("✓ Added ai-rulez hook to %s\n", configFile)
			} else {
				fmt.Printf("  - ai-rulez hook already exists in %s\n", configFile)
			}
		}
	} else {
		if configData["pre-commit"] == nil {
			configData["pre-commit"] = make(map[string]interface{})
		}
		preCommit, ok := configData["pre-commit"].(map[string]interface{})
		if !ok {
			preCommit = make(map[string]interface{})
			configData["pre-commit"] = preCommit
		}
		preCommit["commands"] = map[string]interface{}{
			"ai-rulez": map[string]interface{}{
				"glob":      "**/*.{ai-rulez,ai_rulez}.{yaml,yml}",
				"run":       "ai-rulez generate --dry-run",
				"fail_text": "AI rules validation failed",
			},
		}

		updatedData, err := yaml.Marshal(configData)
		if err != nil {
			return fmt.Errorf("failed to marshal updated config: %w", err)
		}

		if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
			return fmt.Errorf("failed to write updated config: %w", err)
		}

		fmt.Printf("✓ Added ai-rulez hook to %s\n", configFile)
	}

	return nil
}

func setupHuskyConfig() error {
	if _, err := os.Stat(".husky"); os.IsNotExist(err) {
		return fmt.Errorf(".husky directory not found")
	}

	preCommitPath := ".husky/pre-commit"
	var hookContent string

	if data, err := os.ReadFile(preCommitPath); err == nil {
		hookContent = string(data)

		if strings.Contains(hookContent, "ai-rulez") {
			fmt.Printf("  - ai-rulez hook already exists in %s\n", preCommitPath)
			return nil
		}

		if !strings.HasSuffix(hookContent, "\n") {
			hookContent += "\n"
		}
		hookContent += "\n# Validate AI rules configuration\n"
		hookContent += "echo 'Validating AI rules...'\n"
		hookContent += "npx ai-rulez generate --dry-run || exit 1\n"
	} else {
		hookContent = `#!/usr/bin/env sh
. "$(dirname -- "$0")/_/husky.sh"

# Validate AI rules configuration
echo 'Validating AI rules...'
npx ai-rulez generate --dry-run || exit 1
`
	}

	if err := os.WriteFile(preCommitPath, []byte(hookContent), 0o755); err != nil {
		return fmt.Errorf("failed to write husky pre-commit hook: %w", err)
	}

	fmt.Printf("✓ Added ai-rulez hook to %s\n", preCommitPath)
	fmt.Println("  - Using npx ai-rulez for validation")
	fmt.Println("  - Make sure ai-rulez is in your package.json devDependencies")

	return nil
}

func setupPreCommitConfig() error {
	configFile := ""
	files := []string{".pre-commit-config.yaml", ".pre-commit-config.yml"}
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			configFile = file
			break
		}
	}

	if configFile == "" {
		return fmt.Errorf("pre-commit configuration file not found")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read pre-commit config: %w", err)
	}

	var configData map[string]interface{}
	if err := yaml.Unmarshal(data, &configData); err != nil {
		return fmt.Errorf("failed to parse pre-commit config: %w", err)
	}

	repos, ok := configData["repos"].([]interface{})
	if !ok {
		repos = []interface{}{}
	}

	aiRulezExists := false
	for _, repo := range repos {
		if repoMap, ok := repo.(map[string]interface{}); ok {
			if repoURL, ok := repoMap["repo"].(string); ok {
				if strings.Contains(repoURL, "Goldziher/ai-rulez") {
					aiRulezExists = true
					break
				}
			}
		}
	}

	if !aiRulezExists {
		aiRulezRepo := map[string]interface{}{
			"repo": "https://github.com/Goldziher/ai-rulez",
			"rev":  "v1.4.3",
			"hooks": []interface{}{
				map[string]interface{}{
					"id": "ai-rulez-validate",
				},
			},
		}
		repos = append(repos, aiRulezRepo)
		configData["repos"] = repos

		updatedData, err := yaml.Marshal(configData)
		if err != nil {
			return fmt.Errorf("failed to marshal updated config: %w", err)
		}

		if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
			return fmt.Errorf("failed to write updated config: %w", err)
		}

		fmt.Printf("✓ Added ai-rulez hooks to %s\n", configFile)
		fmt.Println("  - Using ai-rulez-validate hook")
		fmt.Println("  - Run 'pre-commit install' to activate the hooks")
	} else {
		fmt.Printf("  - ai-rulez hooks already exist in %s\n", configFile)
	}

	return nil
}

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
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Rule name is required"), nil
	}

	content := request.GetString("content", "")
	if content == "" {
		return mcp.NewToolResultError("Rule content is required"), nil
	}

	priority := int(request.GetFloat("priority", 5))
	id := request.GetString("id", "")

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	newRule := config.Rule{
		ID:       id,
		Name:     name,
		Priority: priority,
		Content:  content,
	}
	cfg.Rules = append(cfg.Rules, newRule)

	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"rule": map[string]interface{}{
			"id":       id,
			"name":     name,
			"priority": priority,
		},
		"total_rules": len(cfg.Rules),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleAddSection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := request.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("Section title is required"), nil
	}

	content := request.GetString("content", "")
	if content == "" {
		return mcp.NewToolResultError("Section content is required"), nil
	}

	priority := int(request.GetFloat("priority", 5))
	id := request.GetString("id", "")

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	newSection := config.Section{
		ID:       id,
		Title:    title,
		Priority: priority,
		Content:  content,
	}
	cfg.Sections = append(cfg.Sections, newSection)

	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"section": map[string]interface{}{
			"id":       id,
			"title":    title,
			"priority": priority,
		},
		"total_sections": len(cfg.Sections),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleAddOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filename := request.GetString("filename", "")
	if filename == "" {
		return mcp.NewToolResultError("Output filename is required"), nil
	}

	template := request.GetString("template", "")

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	for _, output := range cfg.Outputs {
		if output.GetPath() == filename {
			return mcp.NewToolResultError(fmt.Sprintf("Output file '%s' already exists in configuration", filename)), nil
		}
	}

	newOutput := config.Output{
		Path:     filename,
		Template: template,
	}
	cfg.Outputs = append(cfg.Outputs, newOutput)

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
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Rule name is required"), nil
	}

	newContent := request.GetString("content", "")
	priority := int(request.GetFloat("priority", 0))

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

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

	if newContent != "" {
		cfg.Rules[ruleIndex].Content = newContent
	}
	if priority > 0 {
		cfg.Rules[ruleIndex].Priority = priority
	}

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
	title := request.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("Section title is required"), nil
	}

	newContent := request.GetString("content", "")
	priority := int(request.GetFloat("priority", 0))

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

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

	if newContent != "" {
		cfg.Sections[sectionIndex].Content = newContent
	}
	if priority > 0 {
		cfg.Sections[sectionIndex].Priority = priority
	}

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
	filename := request.GetString("filename", "")
	if filename == "" {
		return mcp.NewToolResultError("Output filename is required"), nil
	}

	template := request.GetString("template", "")
	if template == "" {
		return mcp.NewToolResultError("Template is required"), nil
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	outputIndex := -1
	for i, output := range cfg.Outputs {
		if output.GetPath() == filename {
			outputIndex = i
			break
		}
	}

	if outputIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Output file '%s' not found", filename)), nil
	}

	cfg.Outputs[outputIndex].Template = template

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
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Rule name is required"), nil
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

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

	cfg.Rules = append(cfg.Rules[:ruleIndex], cfg.Rules[ruleIndex+1:]...)

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
	title := request.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("Section title is required"), nil
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

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

	cfg.Sections = append(cfg.Sections[:sectionIndex], cfg.Sections[sectionIndex+1:]...)

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
	filename := request.GetString("filename", "")
	if filename == "" {
		return mcp.NewToolResultError("Output filename is required"), nil
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	outputIndex := -1
	for i, output := range cfg.Outputs {
		if output.GetPath() == filename {
			outputIndex = i
			break
		}
	}

	if outputIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Output file '%s' not found", filename)), nil
	}

	cfg.Outputs = append(cfg.Outputs[:outputIndex], cfg.Outputs[outputIndex+1:]...)

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

func handleGetAgents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	nameFilter := request.GetString("name_filter", "")

	var agentsList []config.Agent
	for i := range cfg.Agents {
		if nameFilter == "" || strings.Contains(strings.ToLower(cfg.Agents[i].Name), strings.ToLower(nameFilter)) {
			agentsList = append(agentsList, cfg.Agents[i])
		}
	}

	result := map[string]interface{}{
		"config_file":  configFile,
		"total_agents": len(agentsList),
		"agents":       agentsList,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleAddAgent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Agent name is required"), nil
	}

	systemPrompt := request.GetString("system_prompt", "")
	if systemPrompt == "" {
		return mcp.NewToolResultError("System prompt is required"), nil
	}

	description := request.GetString("description", "")
	priority := int(request.GetFloat("priority", 5))
	toolsStr := request.GetString("tools", "")
	id := request.GetString("id", "")

	var tools []string
	if toolsStr != "" {
		tools = strings.Split(toolsStr, ",")
		for i, tool := range tools {
			tools[i] = strings.TrimSpace(tool)
		}
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			return mcp.NewToolResultError(fmt.Sprintf("Agent '%s' already exists", name)), nil
		}
	}

	newAgent := config.Agent{
		ID:           id,
		Name:         name,
		Description:  description,
		Priority:     priority,
		Tools:        tools,
		SystemPrompt: systemPrompt,
	}
	cfg.Agents = append(cfg.Agents, newAgent)

	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":      true,
		"config_file":  configFile,
		"agent_name":   name,
		"priority":     priority,
		"total_agents": len(cfg.Agents),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleUpdateAgent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Agent name is required"), nil
	}

	description := request.GetString("description", "")
	priority := int(request.GetFloat("priority", 0))
	systemPrompt := request.GetString("system_prompt", "")
	toolsStr := request.GetString("tools", "")

	var tools []string
	if toolsStr != "" {
		tools = strings.Split(toolsStr, ",")
		for i, tool := range tools {
			tools[i] = strings.TrimSpace(tool)
		}
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	agentIndex := -1
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			agentIndex = i
			break
		}
	}

	if agentIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Agent '%s' not found", name)), nil
	}

	if description != "" {
		cfg.Agents[agentIndex].Description = description
	}
	if priority > 0 {
		cfg.Agents[agentIndex].Priority = priority
	}
	if systemPrompt != "" {
		cfg.Agents[agentIndex].SystemPrompt = systemPrompt
	}
	if len(tools) > 0 {
		cfg.Agents[agentIndex].Tools = tools
	}

	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"config_file": configFile,
		"agent_name":  name,
		"updated":     true,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleDeleteAgent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("Agent name is required"), nil
	}

	configFile := request.GetString("config_file", "")
	if configFile == "" {
		foundConfig, err := config.FindConfigFile(".")
		if err != nil {
			return mcp.NewToolResultError(errors.FormatErrorSimple(err)), nil
		}
		configFile = foundConfig
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading configuration: %v", err)), nil
	}

	agentIndex := -1
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			agentIndex = i
			break
		}
	}

	if agentIndex == -1 {
		return mcp.NewToolResultError(fmt.Sprintf("Agent '%s' not found", name)), nil
	}

	cfg.Agents = append(cfg.Agents[:agentIndex], cfg.Agents[agentIndex+1:]...)

	if err := config.SaveConfig(cfg, configFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error saving configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":      true,
		"config_file":  configFile,
		"deleted":      name,
		"total_agents": len(cfg.Agents),
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

//nolint:gocyclo // This function handles complex initialization logic with multiple providers
func handleInitProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectName := request.GetString("project_name", "My Project")
	configFile := request.GetString("config_file", "ai_rulez.yaml")

	if _, err := os.Stat(configFile); err == nil {
		return mcp.NewToolResultError(fmt.Sprintf("Configuration file already exists: %s", configFile)), nil
	}

	providers := ProviderConfig{
		Enabled:      []string{},
		WithAgents:   request.GetBool("with_agents", false),
		WithSections: request.GetBool("with_sections", false),
		NoComments:   request.GetBool("no_comments", false),
	}

	all := request.GetBool("all", false)
	popular := request.GetBool("popular", false)
	minimal := request.GetBool("minimal", false)

	//nolint:gocritic // if-else chain is clearer here than switch
	if all {
		providers.Enabled = []string{"claude", "cursor", "windsurf", "gemini", "copilot", "cline", "continue"}
	} else if popular {
		providers.Enabled = []string{"claude", "cursor", "windsurf", "copilot"}
	} else if minimal {
		providers.Enabled = []string{"claude"}
		providers.WithAgents = false
		providers.WithSections = false
	} else {
		if request.GetBool("claude", false) {
			providers.Enabled = append(providers.Enabled, "claude")
		}
		if request.GetBool("cursor", false) {
			providers.Enabled = append(providers.Enabled, "cursor")
		}
		if request.GetBool("windsurf", false) {
			providers.Enabled = append(providers.Enabled, "windsurf")
		}
		if request.GetBool("gemini", false) {
			providers.Enabled = append(providers.Enabled, "gemini")
		}
		if request.GetBool("copilot", false) {
			providers.Enabled = append(providers.Enabled, "copilot")
		}
		if request.GetBool("cline", false) {
			providers.Enabled = append(providers.Enabled, "cline")
		}
		if request.GetBool("continue", false) {
			providers.Enabled = append(providers.Enabled, "continue")
		}

		if len(providers.Enabled) == 0 {
			providers.Enabled = []string{"claude"}
		}
	}

	if err := createProviderConfigFile(projectName, configFile, providers); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error creating configuration: %v", err)), nil
	}

	result := map[string]interface{}{
		"success":       true,
		"project_name":  projectName,
		"config_file":   configFile,
		"providers":     providers.Enabled,
		"with_agents":   providers.WithAgents,
		"with_sections": providers.WithSections,
		"no_comments":   providers.NoComments,
	}

	setupHooks := request.GetBool("setup_hooks", false)
	if setupHooks {
		messages := []string{}

		//nolint:gocritic // if-else chain is clearer here than switch
		if detectLefthook() {
			messages = append(messages, "Found lefthook configuration")
			if err := setupLefthookConfig(); err != nil {
				result["setup_hooks_error"] = fmt.Sprintf("Error setting up lefthook: %v", err)
			} else {
				messages = append(messages, "✓ Added ai-rulez validation to lefthook.yml")
			}
		} else if detectPreCommit() {
			messages = append(messages, "Found pre-commit configuration")
			if err := setupPreCommitConfig(); err != nil {
				result["setup_hooks_error"] = fmt.Sprintf("Error setting up pre-commit: %v", err)
			} else {
				messages = append(messages, "✓ Added ai-rulez validation to .pre-commit-config.yaml")
			}
		} else if detectHusky() {
			messages = append(messages, "Found husky configuration")
			if err := setupHuskyConfig(); err != nil {
				result["setup_hooks_error"] = fmt.Sprintf("Error setting up husky: %v", err)
			} else {
				messages = append(messages, "✓ Added ai-rulez validation to .husky/pre-commit")
			}
		} else {
			messages = append(messages, "No git hook manager detected (lefthook, pre-commit, or husky)")
		}

		result["setup_hooks_messages"] = messages
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func handleGetVersion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result := map[string]interface{}{
		"version": Version,
		"success": true,
	}

	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}
