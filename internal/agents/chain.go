package agents

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
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

// TaskStatus represents the status of a running task
type TaskStatus struct {
	task      AgentTask
	status    string // "⠋", "↻", "✓", "✗"
	attempt   int
	mu        sync.RWMutex
	completed bool
}

func ExecuteInitChain(agent AgentInfo, context *ProjectContext, providerConfig templates.ProviderConfig) (string, error) {
	logger.Info("🔗 Starting parallel agent task execution...")
	logger.Info("")

	// Initialize base configuration
	if err := initializeBaseConfigFile(context, providerConfig); err != nil {
		return "", fmt.Errorf("failed to initialize base config: %w", err)
	}
	logger.Info("✅ Initialized base ai_rulez.yaml")
	fmt.Printf("\n")

	// Initialize task status tracking
	taskStatuses := make([]*TaskStatus, len(initAgentTasks))
	for i, task := range initAgentTasks {
		taskStatuses[i] = &TaskStatus{
			task:    task,
			status:  "⠋",
			attempt: 1,
		}
	}

	// Display initial task list
	fmt.Printf("🚀 Running %d agent tasks:\n", len(initAgentTasks))
	displayTaskStatuses(taskStatuses)

	// Structure to hold task results
	type taskResult struct {
		taskIndex int
		err       error
	}

	// Channel to collect results
	results := make(chan taskResult, len(initAgentTasks))

	// WaitGroup for synchronization
	var wg sync.WaitGroup

	// Launch all agent tasks in parallel
	for i, task := range initAgentTasks {
		wg.Add(1)
		go func(taskIndex int, t AgentTask) {
			defer wg.Done()

			err := executeAgentTaskWithStatus(t, agent, context, taskStatuses[taskIndex])
			results <- taskResult{taskIndex: taskIndex, err: err}
		}(i, task)
	}

	// Start a goroutine to show periodic status updates
	statusDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				showRunningTasksStatus(taskStatuses)
			case <-statusDone:
				return
			}
		}
	}()

	// Wait for all tasks to complete
	wg.Wait()
	close(statusDone)
	close(results)

	// Process results
	var failedTasks []string
	successCount := 0

	for result := range results {
		ts := taskStatuses[result.taskIndex]
		ts.mu.Lock()
		if result.err != nil {
			ts.status = "✗"
			ts.completed = true
			failedTasks = append(failedTasks, ts.task.Name)
		} else {
			ts.status = "✓"
			ts.completed = true
			successCount++
		}
		ts.mu.Unlock()
	}

	// Display final status
	fmt.Printf("\nFinal Status:\n")
	displayTaskStatuses(taskStatuses)

	fmt.Printf("\n")
	if len(failedTasks) > 0 {
		logger.Warn(fmt.Sprintf("Completed %d/%d tasks successfully. Failed: %v",
			successCount, len(initAgentTasks), failedTasks))
	} else {
		logger.Success("✅ All agent tasks completed successfully")
	}

	// Validate the generated configuration
	if err := validateGeneratedConfig(); err != nil {
		logger.Warn(fmt.Sprintf("⚠️  Generated config has validation issues: %v", err))
		logger.Info("The file was created but may need manual review")
	} else {
		logger.Success("✅ Generated configuration is valid")
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

// executeAgentTaskWithStatus executes a single agent task with status updates
func executeAgentTaskWithStatus(task AgentTask, agent AgentInfo, context *ProjectContext, status *TaskStatus) error {
	// Build the prompt for this task
	prompt := task.Prompt(context)

	// Execute with retries, updating status for each attempt
	var lastErr error
	startTime := time.Now()

	for attempt := 1; attempt <= task.MaxRetries; attempt++ {
		// Update status for retry attempts
		if attempt > 1 {
			status.mu.Lock()
			status.status = fmt.Sprintf("↻%d", attempt)
			status.attempt = attempt
			status.mu.Unlock()

			// Log retry reason for better feedback
			logger.Info(fmt.Sprintf("🔄 %s: Retry %d/%d (previous attempt: %v)",
				task.Name, attempt-1, task.MaxRetries-1, getErrorSummary(lastErr)))

			// Brief delay between retries with exponential backoff
			delay := time.Duration(attempt-1) * 2 * time.Second
			time.Sleep(delay)
		}

		// Execute the agent call
		attemptStart := time.Now()
		_, err := invokeAgent(agent, prompt, 120*time.Second)
		attemptDuration := time.Since(attemptStart)

		if err == nil {
			// Success - update status and return
			status.mu.Lock()
			status.status = "✓"
			status.completed = true
			status.mu.Unlock()

			totalDuration := time.Since(startTime)
			logger.Success(fmt.Sprintf("✅ %s completed in %v (attempt %d)",
				task.Name, totalDuration.Round(time.Second), attempt))
			return nil
		}

		lastErr = err
		logger.Warn(fmt.Sprintf("⚠️  %s: Attempt %d failed after %v: %v",
			task.Name, attempt, attemptDuration.Round(time.Second), getErrorSummary(err)))
	}

	// All attempts failed - update status
	status.mu.Lock()
	status.status = "✗"
	status.completed = true
	status.mu.Unlock()

	totalDuration := time.Since(startTime)
	return fmt.Errorf("task '%s' failed after %d attempts in %v: %w",
		task.Name, task.MaxRetries, totalDuration.Round(time.Second), lastErr)
}

// displayTaskStatuses shows all task statuses
func displayTaskStatuses(statuses []*TaskStatus) {
	for _, ts := range statuses {
		ts.mu.RLock()
		fmt.Printf("%s %s", ts.status, ts.task.Description)
		if ts.attempt > 1 {
			fmt.Printf(" (attempt %d)", ts.attempt)
		}
		fmt.Printf("\n")
		ts.mu.RUnlock()
	}
}

// showRunningTasksStatus shows which tasks are still running
func showRunningTasksStatus(statuses []*TaskStatus) {
	var running []string
	var completed int

	for _, ts := range statuses {
		ts.mu.RLock()
		if ts.completed {
			completed++
		} else {
			status := "working"
			if ts.attempt > 1 {
				status = fmt.Sprintf("retry %d", ts.attempt)
			}
			running = append(running, fmt.Sprintf("%s (%s)", ts.task.Name, status))
		}
		ts.mu.RUnlock()
	}

	if len(running) > 0 {
		fmt.Printf("⏳ Progress: %d/%d completed | Still running: %s\n",
			completed, len(statuses), strings.Join(running, ", "))
	}
}

// validateGeneratedConfig validates the generated ai_rulez.yaml file
func validateGeneratedConfig() error {
	// Try to load and validate the configuration
	cfg, err := config.LoadConfig("ai_rulez.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}

// getErrorSummary returns a concise error summary for user feedback
func getErrorSummary(err error) string {
	if err == nil {
		return "no error"
	}

	errStr := err.Error()

	// Common error patterns
	if strings.Contains(errStr, "timed out") || strings.Contains(errStr, "timeout") {
		return "timeout"
	}
	if strings.Contains(errStr, "Usage Policy") || strings.Contains(errStr, "policy") {
		return "policy violation"
	}
	if strings.Contains(errStr, "rate limit") {
		return "rate limited"
	}
	if strings.Contains(errStr, "network") || strings.Contains(errStr, "connection") {
		return "network error"
	}
	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "unauthorized") {
		return "authentication error"
	}

	// Return first 50 chars for other errors
	if len(errStr) > 50 {
		return errStr[:47] + "..."
	}
	return errStr
}
