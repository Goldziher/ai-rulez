package agents

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// AgentTask represents a specific task that an agent will perform
type AgentTask struct {
	Name        string
	Description string
	MaxRetries  int
	Prompt      func(context *ProjectContext) string
}

const (
	// Default number of retries for agent tasks
	defaultMaxRetries = 3
)

// All agent tasks that run in parallel
var initAgentTasks = []AgentTask{
	{
		Name:        "project-analyzer",
		Description: "Analyzing project structure and writing description",
		MaxRetries:  defaultMaxRetries,
		Prompt:      buildProjectAnalysisPrompt,
	},
	{
		Name:        "documentation-agent",
		Description: "Creating documentation sections",
		MaxRetries:  defaultMaxRetries,
		Prompt:      buildDocumentationPrompt,
	},
	{
		Name:        "standards-agent",
		Description: "Defining coding standards",
		MaxRetries:  defaultMaxRetries,
		Prompt:      buildStandardsPrompt,
	},
	{
		Name:        "specialist-agent",
		Description: "Creating specialized AI agents",
		MaxRetries:  defaultMaxRetries,
		Prompt:      buildSpecialistPrompt,
	},
}

func ExecuteInitChain(agent AgentInfo, context *ProjectContext, providerConfig templates.ProviderConfig) (string, error) {
	logger.Info("🔗 Starting parallel agent task execution...")
	logger.Info("")

	// Initialize base configuration
	if err := initializeBaseConfigFile(context, providerConfig); err != nil {
		return "", fmt.Errorf("failed to initialize base config: %w", err)
	}
	logger.Info("✅ Initialized base ai_rulez.yaml")

	// Show all tasks that will run in parallel
	fmt.Printf("🚀 Running %d agent tasks in parallel...\n", len(initAgentTasks))
	for _, task := range initAgentTasks {
		fmt.Printf("  ∥ %s: %s\n", task.Name, task.Description)
	}
	fmt.Printf("\n")

	// Structure to hold task results
	type taskResult struct {
		task AgentTask
		err  error
	}

	// Channel to collect results
	results := make(chan taskResult, len(initAgentTasks))

	// WaitGroup for synchronization
	var wg sync.WaitGroup

	// Launch all agent tasks in parallel
	for _, task := range initAgentTasks {
		wg.Add(1)
		go func(t AgentTask) {
			defer wg.Done()

			logger.Info(fmt.Sprintf("%s: %s", t.Name, t.Description))
			err := executeAgentTask(t, agent, context)
			results <- taskResult{task: t, err: err}
		}(task)
	}

	// Wait for all tasks to complete
	wg.Wait()
	close(results)

	// Process results
	var failedTasks []string
	successCount := 0

	for result := range results {
		if result.err != nil {
			logger.Warn(fmt.Sprintf("Task %s failed: %v", result.task.Name, result.err))
			failedTasks = append(failedTasks, result.task.Name)
		} else {
			logger.Success(fmt.Sprintf("✓ %s completed", result.task.Name))
			successCount++
		}
	}

	logger.Info("")
	if len(failedTasks) > 0 {
		logger.Warn(fmt.Sprintf("Completed %d/%d tasks successfully. Failed: %v",
			successCount, len(initAgentTasks), failedTasks))
	} else {
		logger.Success("✅ All agent tasks completed successfully")
	}

	// Return final configuration content
	if data, err := os.ReadFile("ai_rulez.yaml"); err == nil {
		return string(data), nil
	}

	return "", fmt.Errorf("failed to read final configuration")
}

// initializeBaseConfigFile creates the initial ai_rulez.yaml with metadata and outputs
func initializeBaseConfigFile(context *ProjectContext, providerConfig templates.ProviderConfig) error {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:    context.ProjectName,
			Version: "1.0.0",
		},
		Outputs: buildOutputsConfig(providerConfig),
	}
	return config.SaveConfig(cfg, "ai_rulez.yaml")
}

// buildOutputsConfig creates outputs configuration based on provider config
func buildOutputsConfig(providerConfig templates.ProviderConfig) []config.Output {
	var outputs []config.Output

	if providerConfig.Claude {
		outputs = append(outputs,
			config.Output{Path: "CLAUDE.md"},
			config.Output{
				Path:         ".claude/agents/",
				Type:         "agent",
				NamingScheme: "{name}.md",
			})
	}
	if providerConfig.Cursor {
		outputs = append(outputs, config.Output{
			Path:         ".cursor/rules/",
			Type:         "section",
			NamingScheme: "{name}.md",
		})
	}
	if providerConfig.Windsurf {
		outputs = append(outputs, config.Output{Path: ".windsurfrules"})
	}
	if providerConfig.Copilot {
		outputs = append(outputs, config.Output{Path: ".github/copilot-instructions.md"})
	}
	if providerConfig.Gemini {
		outputs = append(outputs, config.Output{Path: "GEMINI.md"})
	}
	if providerConfig.Amp || providerConfig.Codex {
		outputs = append(outputs, config.Output{Path: "AGENTS.md"})
	}
	if providerConfig.Cline {
		outputs = append(outputs, config.Output{
			Path:         ".clinerules/",
			Type:         "section",
			NamingScheme: "{name}.md",
		})
	}
	if providerConfig.ContinueDev {
		outputs = append(outputs,
			config.Output{
				Path:         ".continue/rules/",
				Type:         "section",
				NamingScheme: "{name}.md",
			},
			config.Output{
				Path: ".continue/prompts/ai_rulez_prompts.yaml",
				Template: &config.Template{
					Type:  "builtin",
					Value: "continue-prompts",
				},
			})
	}

	return outputs
}

// executeAgentTask executes a single agent task (retry logic is handled by executeAgentWithRetries)
func executeAgentTask(task AgentTask, agent AgentInfo, context *ProjectContext) error {
	return runSingleAgentTask(task, agent, context)
}

// runSingleAgentTask executes a single attempt at an agent task
func runSingleAgentTask(task AgentTask, agent AgentInfo, context *ProjectContext) error {
	// Build the prompt for this task
	prompt := task.Prompt(context)

	// Create spinner for visual feedback
	spinner := progress.NewSpinner(fmt.Sprintf("%s...", task.Description))

	// Execute the agent with the prompt (executeAgentWithRetries handles its own spinner management)
	_, err := executeAgentWithRetries(agent, prompt, 120*time.Second, spinner, task.MaxRetries, 1)

	// Stop spinner
	if err := spinner.Finish(); err != nil {
		logger.Warn("Failed to finish spinner", "error", err)
	}

	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	return nil
}
