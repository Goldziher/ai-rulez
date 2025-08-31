package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/agents"
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/hooks"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	// Provider flags
	allProviders     bool
	popularProviders bool
	withAgents       bool
	withSections     bool
	noComments       bool
	preset           string
	useAgent         string
	noAgent          bool
	listAgents       bool
	setupHooks       bool // Added for git hooks setup

	// Individual provider flags
	claudeFlag      bool
	cursorFlag      bool
	windsurfFlag    bool
	copilotFlag     bool
	geminiFlag      bool
	ampFlag         bool
	codexFlag       bool
	clineFlag       bool
	continueDevFlag bool
)

// InitCmd represents the init command
var InitCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new AI rules configuration",
	Long: `Initialize a new AI rules configuration for your project.
This creates an ai_rulez.yaml file with sensible defaults and 
configures outputs for your selected AI assistants.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runInit,
}

func init() {
	// Provider selection flags
	InitCmd.Flags().BoolVar(&allProviders, "all", false, "Enable all supported providers")
	InitCmd.Flags().BoolVar(&popularProviders, "popular", false, "Enable popular providers (Claude, Cursor, Windsurf, Copilot)")
	InitCmd.Flags().StringVar(&preset, "preset", "", "Use a preset configuration (claude, cursor, windsurf, copilot, etc.)")

	// Individual provider flags
	InitCmd.Flags().BoolVar(&claudeFlag, "claude", false, "Include Claude configuration")
	InitCmd.Flags().BoolVar(&cursorFlag, "cursor", false, "Include Cursor configuration")
	InitCmd.Flags().BoolVar(&windsurfFlag, "windsurf", false, "Include Windsurf configuration")
	InitCmd.Flags().BoolVar(&copilotFlag, "copilot", false, "Include GitHub Copilot configuration")
	InitCmd.Flags().BoolVar(&geminiFlag, "gemini", false, "Include Gemini configuration")
	InitCmd.Flags().BoolVar(&ampFlag, "amp", false, "Include AMP configuration")
	InitCmd.Flags().BoolVar(&codexFlag, "codex", false, "Include Codex configuration")
	InitCmd.Flags().BoolVar(&clineFlag, "cline", false, "Include Cline configuration")
	InitCmd.Flags().BoolVar(&continueDevFlag, "continue-dev", false, "Include Continue.dev configuration")

	// Configuration options
	InitCmd.Flags().BoolVar(&withAgents, "with-agents", false, "Include agent configurations (for Claude)")
	InitCmd.Flags().BoolVar(&withSections, "with-sections", true, "Include documentation sections")
	InitCmd.Flags().BoolVar(&noComments, "no-comments", false, "Omit explanatory comments from the generated config")

	// AI agent flags
	InitCmd.Flags().StringVar(&useAgent, "use-agent", "", "Use an AI agent to generate configuration (claude, gemini, etc.)")
	InitCmd.Flags().BoolVar(&noAgent, "no-agent", false, "Skip AI agent detection and usage")
	InitCmd.Flags().BoolVar(&listAgents, "list-agents", false, "List available AI agents and exit")

	// Git hooks setup
	InitCmd.Flags().BoolVar(&setupHooks, "setup-hooks", false, "Automatically configure git hooks for ai-rulez validation if lefthook, pre-commit, or husky is detected")
}

func runInit(cmd *cobra.Command, args []string) {
	if handleListAgents() {
		return
	}

	checkExistingConfig()
	projectName := getProjectName(args)
	providerConfig := parseProviderFlags(cmd)

	if tryAgentGeneration(cmd, projectName, providerConfig) {
		return
	}

	generateAndWriteConfig(projectName, providerConfig)
	displaySuccessMessage(projectName, providerConfig)

	// Setup git hooks if requested
	if setupHooks {
		handleHooksSetup()
	}
}

func handleListAgents() bool {
	if listAgents {
		agents.ListAvailableAgents()
		return true
	}
	return false
}

func checkExistingConfig() {
	if _, err := config.FindConfigFile("."); err == nil {
		logger.Error("Configuration file already exists in this directory")
		logger.Info("Remove or rename the existing file to initialize a new configuration")
		os.Exit(1)
	}
}

func getProjectName(args []string) string {
	projectName := "MyProject"
	if len(args) > 0 {
		projectName = args[0]
	} else {
		// Try to get from current directory name
		if cwd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(cwd)
		}
	}
	return projectName
}

func tryAgentGeneration(cmd *cobra.Command, projectName string, providerConfig templates.ProviderConfig) bool {
	if !noAgent && (useAgent != "" || agents.ShouldPromptForAgent()) {
		generatedConfig, handled := agents.HandleAgentGeneration(cmd, projectName, providerConfig, useAgent)
		if handled {
			writeConfigFile(generatedConfig)
			logger.Info("✅ Configuration generated successfully with AI assistance")
			return true
		}
	}
	return false
}

func generateAndWriteConfig(projectName string, providerConfig templates.ProviderConfig) {
	configContent := templates.GenerateConfigTemplate(projectName, providerConfig)
	writeConfigFile(configContent)
}

func writeConfigFile(content string) {
	if err := os.WriteFile("ai_rulez.yaml", []byte(content), 0o644); err != nil {
		logger.LogError("Failed to write configuration file", oops.
			With("path", "ai_rulez.yaml").
			Wrapf(err, "write file"))
		os.Exit(1)
	}
}

func displaySuccessMessage(projectName string, providerConfig templates.ProviderConfig) {
	logger.Info("✅ Created ai_rulez.yaml", "project", projectName)
	logger.Info("Configuration includes:")

	displayProviderIncludes(providerConfig)
	displayNextSteps()
}

func displayProviderIncludes(providerConfig templates.ProviderConfig) {
	if providerConfig.Claude {
		logger.Info("  - Claude (CLAUDE.md)")
		if withAgents {
			logger.Info("  - Claude agents (.claude/agents/)")
		}
	}
	if providerConfig.Cursor {
		logger.Info("  - Cursor (.cursor/rules/)")
	}
	if providerConfig.Windsurf {
		logger.Info("  - Windsurf (.windsurf/)")
	}
	if providerConfig.Copilot {
		logger.Info("  - GitHub Copilot (.github/copilot-instructions.md)")
	}
	if providerConfig.Gemini {
		logger.Info("  - Gemini (GEMINI.md)")
	}
	if providerConfig.Amp || providerConfig.Codex {
		logger.Info("  - AMP/Codex (AGENTS.md)")
	}
	if providerConfig.Cline {
		logger.Info("  - Cline (.clinerules/)")
	}
	if providerConfig.ContinueDev {
		logger.Info("  - Continue.dev (.continue/rules/)")
	}
}

func displayNextSteps() {
	logger.Info("\nNext steps:")
	logger.Info("  1. Edit ai_rulez.yaml to customize your rules and sections")
	logger.Info("  2. Run 'ai-rulez generate' to create the output files")

	if hooks.DetectGitHooks() != "" {
		logger.Info("  3. Consider setting up git hooks for automatic generation")
	}
}

func parseProviderFlags(cmd *cobra.Command) templates.ProviderConfig {
	if preset != "" {
		return parsePresetFlag()
	}

	if allProviders {
		return parseAllProvidersFlag()
	}

	if popularProviders {
		return parsePopularProvidersFlag()
	}

	return parseIndividualFlags(cmd)
}

func parsePresetFlag() templates.ProviderConfig {
	presetMap := map[string]func(*templates.ProviderConfig){
		"claude":       func(pc *templates.ProviderConfig) { pc.Claude = true },
		"cursor":       func(pc *templates.ProviderConfig) { pc.Cursor = true },
		"windsurf":     func(pc *templates.ProviderConfig) { pc.Windsurf = true },
		"copilot":      func(pc *templates.ProviderConfig) { pc.Copilot = true },
		"gemini":       func(pc *templates.ProviderConfig) { pc.Gemini = true },
		"amp":          func(pc *templates.ProviderConfig) { pc.Amp = true },
		"codex":        func(pc *templates.ProviderConfig) { pc.Codex = true },
		"cline":        func(pc *templates.ProviderConfig) { pc.Cline = true },
		"continue-dev": func(pc *templates.ProviderConfig) { pc.ContinueDev = true },
		"continue":     func(pc *templates.ProviderConfig) { pc.ContinueDev = true },
	}

	pc := templates.ProviderConfig{}
	if setter, exists := presetMap[preset]; exists {
		setter(&pc)
	}
	return pc
}

func parseAllProvidersFlag() templates.ProviderConfig {
	return templates.ProviderConfig{
		Claude:      true,
		Cursor:      true,
		Windsurf:    true,
		Copilot:     true,
		Gemini:      true,
		Amp:         true,
		Codex:       true,
		Cline:       true,
		ContinueDev: true,
	}
}

func parsePopularProvidersFlag() templates.ProviderConfig {
	return templates.ProviderConfig{
		Claude:   true,
		Cursor:   true,
		Windsurf: true,
		Copilot:  true,
	}
}

func parseIndividualFlags(cmd *cobra.Command) templates.ProviderConfig {
	pc := templates.ProviderConfig{
		Claude:      claudeFlag,
		Cursor:      cursorFlag,
		Windsurf:    windsurfFlag,
		Copilot:     copilotFlag,
		Gemini:      geminiFlag,
		Amp:         ampFlag,
		Codex:       codexFlag,
		Cline:       clineFlag,
		ContinueDev: continueDevFlag,
	}

	// If no providers specified, default to Claude
	if !pc.HasAny() {
		pc.Claude = true
	}

	// Auto-enable agents for Claude if not explicitly disabled
	if pc.Claude && !cmd.Flags().Changed("with-agents") {
		withAgents = true
	}

	return pc
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
