package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/agents"
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/hooks"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	allProviders     bool
	popularProviders bool
	withAgents       bool
	withSections     bool
	preset           string
	useAgent         string
	noAgent          bool
	listAgents       bool
	setupHooks       bool
	autoYes          bool

	claudeFlag      bool
	cursorFlag      bool
	windsurfFlag    bool
	copilotFlag     bool
	geminiFlag      bool
	ampFlag         bool
	codexFlag       bool
	opencodeFlag    bool
	clineFlag       bool
	continueDevFlag bool
	junieFlag       bool
)

var InitCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new AI rules configuration",
	Long: `Initialize a new AI rules configuration for your project.
This creates an ai-rulez.yaml file with sensible defaults and 
configures outputs for your selected AI assistants.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runInit,
}

func init() {
	InitCmd.Flags().BoolVar(&allProviders, "all", false, "Enable all supported providers")
	InitCmd.Flags().BoolVar(&popularProviders, "popular", false, "Enable popular providers (Claude, Cursor, Windsurf, Copilot)")
	InitCmd.Flags().StringVar(&preset, "preset", "", "Use a preset configuration (claude, cursor, windsurf, copilot, etc.)")

	InitCmd.Flags().BoolVar(&claudeFlag, "claude", false, "Include Claude configuration")
	InitCmd.Flags().BoolVar(&cursorFlag, "cursor", false, "Include Cursor configuration")
	InitCmd.Flags().BoolVar(&windsurfFlag, "windsurf", false, "Include Windsurf configuration")
	InitCmd.Flags().BoolVar(&copilotFlag, "copilot", false, "Include GitHub Copilot configuration")
	InitCmd.Flags().BoolVar(&geminiFlag, "gemini", false, "Include Gemini configuration")
	InitCmd.Flags().BoolVar(&ampFlag, "amp", false, "Include AMP configuration")
	InitCmd.Flags().BoolVar(&codexFlag, "codex", false, "Include Codex configuration")
	InitCmd.Flags().BoolVar(&opencodeFlag, "opencode", false, "Include OpenCode configuration")
	InitCmd.Flags().BoolVar(&clineFlag, "cline", false, "Include Cline configuration")
	InitCmd.Flags().BoolVar(&continueDevFlag, "continue-dev", false, "Include Continue.dev configuration")
	InitCmd.Flags().BoolVar(&junieFlag, "junie", false, "Include Junie configuration")

	InitCmd.Flags().BoolVar(&withAgents, "with-agents", false, "Include agent configurations (for Claude)")
	InitCmd.Flags().BoolVar(&withSections, "with-sections", true, "Include documentation sections")

	InitCmd.Flags().StringVar(&useAgent, "use-agent", "", "Use an AI agent to generate configuration (claude, gemini, etc.)")
	InitCmd.Flags().BoolVar(&noAgent, "no-agent", false, "Skip AI agent detection and usage")
	InitCmd.Flags().BoolVar(&listAgents, "list-agents", false, "List available AI agents and exit")
	InitCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically answer yes to prompts")

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
	handleContinueDevSetup(providerConfig)
	displaySuccessMessage(projectName, providerConfig)

	if setupHooks {
		handleHooksSetup()
	}
}

func handleContinueDevSetup(providerConfig templates.ProviderConfig) {
	if !providerConfig.ContinueDev {
		return
	}

	logger.Info("  - Continue.dev now uses YAML configuration")
	logger.Info("  - Custom prompts will be generated in .continue/prompts/")
	logger.Info("  - For more info, see: https://docs.continue.dev/reference")
}

func handleListAgents() bool {
	if listAgents {
		agents.ListAvailableAgents()
		return true
	}
	return false
}

func checkExistingConfig() {
	configNames := []string{
		".ai-rulez.yaml", ".ai-rulez.yml",
		"ai-rulez.yaml", "ai-rulez.yml",
		".ai_rulez.yaml", ".ai_rulez.yml",
		"ai_rulez.yaml", "ai_rulez.yml",
	}

	for _, name := range configNames {
		if _, err := os.Stat(name); err != nil {
			continue
		}
		logger.Info("Configuration file already exists", "file", name)

		if !shouldOverwriteConfig(name) {
			logger.Info("Operation canceled. Remove or rename the existing file to initialize a new configuration")
			os.Exit(1)
		}

		if err := os.Remove(name); err != nil {
			logger.Error("Failed to remove existing configuration file", "file", name, "error", err)
			os.Exit(1)
		}
		logger.Info("Existing configuration file removed", "file", name)
		break
	}
}

func shouldOverwriteConfig(filename string) bool {
	if autoYes || os.Getenv("CI") != "" || os.Getenv("NO_INTERACTIVE") != "" {
		logger.Info("Auto-overwriting existing configuration file (--yes or CI environment)")
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

	fmt.Printf("⚠️  Overwrite existing file '%s'? (y/N): ", filename)

	var response string
	_, err = fmt.Scanln(&response)
	if err != nil && err.Error() != "unexpected newline" {
		logger.Info("Failed to read input, canceling operation")
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
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

func tryAgentGeneration(cmd *cobra.Command, projectName string, providerConfig templates.ProviderConfig) bool {
	if !noAgent && (useAgent != "" || agents.ShouldPromptForAgent()) {
		// Pass preset information to agent generation
		var presets []string
		if shouldUsePresets(providerConfig) {
			presets = getPresetsFromProviderConfig(providerConfig)
		}
		_, handled := agents.HandleAgentGenerationWithChain(cmd, projectName, providerConfig, presets, useAgent, autoYes)
		if handled {
			fmt.Println("✅ Configuration generated with AI assistance")
			return true
		}
	}
	return false
}

func generateAndWriteConfig(projectName string, providerConfig templates.ProviderConfig) {
	var configContent string

	if shouldUsePresets(providerConfig) {
		presets := getPresetsFromProviderConfig(providerConfig)
		configContent = templates.GeneratePresetConfigTemplate(projectName, presets)
	} else {
		configContent = templates.GenerateConfigTemplate(projectName, providerConfig)
	}

	writeConfigFile(configContent)
}

func shouldUsePresets(providerConfig templates.ProviderConfig) bool {
	return preset != "" || popularProviders || allProviders
}

func getPresetsFromProviderConfig(providerConfig templates.ProviderConfig) []string {
	if preset != "" {
		return []string{preset}
	}

	if popularProviders {
		return []string{"popular"}
	}

	if allProviders {
		return config.IndividualPresetNames()
	}

	return []string{}
}

func writeConfigFile(content string) {
	if err := os.WriteFile("ai-rulez.yaml", []byte(content), 0o644); err != nil {
		logger.LogError("Failed to write configuration file", oops.
			With("path", "ai-rulez.yaml").
			Wrapf(err, "write file"))
		os.Exit(1)
	}

	// Format the generated YAML with yamlfmt if available
	formatConfigFile("ai-rulez.yaml")
}

func displaySuccessMessage(projectName string, providerConfig templates.ProviderConfig) {
	logger.Info("✅ Created ai-rulez.yaml", "project", projectName)
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
	if providerConfig.Amp || providerConfig.Codex || providerConfig.Opencode {
		logger.Info("  - AMP/Codex/Opencode (AGENTS.md)")
	}
	if providerConfig.Cline {
		logger.Info("  - Cline (.clinerules/)")
	}
	if providerConfig.ContinueDev {
		logger.Info("  - Continue.dev rules (.continue/rules/)")
		logger.Info("  - Continue.dev prompts (.continue/prompts/ai_rulez_prompts.yaml)")
	}
	if providerConfig.Junie {
		logger.Info("  - Junie (.junie/guidelines.md)")
	}
}

func displayNextSteps() {
	logger.Info("\nNext steps:")
	logger.Info("  1. Edit ai-rulez.yaml to customize your rules and sections")
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
		"opencode":     func(pc *templates.ProviderConfig) { pc.Opencode = true },
		"cline":        func(pc *templates.ProviderConfig) { pc.Cline = true },
		"continue-dev": func(pc *templates.ProviderConfig) { pc.ContinueDev = true },
		"junie":        func(pc *templates.ProviderConfig) { pc.Junie = true },
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
		Opencode:    true,
		Cline:       true,
		ContinueDev: true,
		Junie:       true,
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
		Opencode:    opencodeFlag,
		Cline:       clineFlag,
		ContinueDev: continueDevFlag,
		Junie:       junieFlag,
	}

	if !pc.HasAny() {
		pc.Claude = true
	}

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

// formatConfigFile formats the YAML file using yamlfmt if available
func formatConfigFile(filename string) {
	// Check if yamlfmt is available
	if _, err := exec.LookPath("yamlfmt"); err != nil {
		logger.Debug("yamlfmt not found, skipping YAML formatting", "file", filename)
		return
	}

	// Run yamlfmt on the file
	cmd := exec.Command("yamlfmt", "-w", filename)
	if err := cmd.Run(); err != nil {
		logger.Warn("Failed to format YAML with yamlfmt", "file", filename, "error", err)
		return
	}

	logger.Debug("Formatted YAML file with yamlfmt", "file", filename)
}
