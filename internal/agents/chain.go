package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// AgentTask represents a specific task that an agent will perform
type AgentTask struct {
	Name           string
	Description    string
	MaxRetries     int
	Prompt         func(context *ProjectContext) string
	FallbackPrompt func(context *ProjectContext) string // Simplified prompt for retries
}

const (
	// Default number of retries for agent tasks
	defaultMaxRetries = 3

	// Task status constants
	statusRunning  = "running"
	statusRetrying = "retrying"
	statusSuccess  = "success"
	statusFailed   = "failed"
)

// getInitAgentTasks returns the agent tasks, dynamically adjusting for project size
func getInitAgentTasks(context *ProjectContext) []AgentTask {
	tasks := []AgentTask{
		{
			Name:           "project-agent",
			Description:    "Analyzing project structure and conventions",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildProjectAnalysisPrompt,
			FallbackPrompt: buildProjectAnalysisFallbackPrompt,
		},
		{
			Name:           "infra-agent",
			Description:    "Analyzing CI/CD and infrastructure",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildInfrastructurePrompt,
			FallbackPrompt: buildInfrastructureFallbackPrompt,
		},
		{
			Name:           "standards-agent",
			Description:    "Defining coding standards",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildStandardsPrompt,
			FallbackPrompt: buildStandardsFallbackPrompt,
		},
		{
			Name:           "ai-agents",
			Description:    "Creating specialized AI agent configurations",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildSpecialistPrompt,
			FallbackPrompt: buildSpecialistFallbackPrompt,
		},
		{
			Name:           "tooling-agent",
			Description:    "Adding commands and MCP server configuration",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildToolingPrompt,
			FallbackPrompt: buildToolingFallbackPrompt,
		},
	}

	// Add documentation tasks based on project size
	docTasks := createDocumentationTasks(context)
	tasks = append(tasks, docTasks...)

	return tasks
}

// createDocumentationTasks creates appropriate documentation tasks based on project size
func createDocumentationTasks(context *ProjectContext) []AgentTask {
	// Count markdown files
	docCount := len(context.MarkdownFiles)

	if docCount <= 10 {
		// Small project - single task
		return []AgentTask{
			{
				Name:           "documentation-agent",
				Description:    "Creating documentation sections",
				MaxRetries:     defaultMaxRetries,
				Prompt:         buildDocumentationPrompt,
				FallbackPrompt: buildDocumentationFallbackPrompt,
			},
		}
	}

	// Large project - split into specialized tasks
	tasks := []AgentTask{}

	// Core documentation (README, CONTRIBUTING, etc.)
	if hasCoreDocs(context) {
		tasks = append(tasks, AgentTask{
			Name:           "docs-core",
			Description:    "Creating core documentation sections",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildCoreDocumentationPrompt,
			FallbackPrompt: buildDocumentationFallbackPrompt,
		})
	}

	// API documentation
	if hasAPIDocs(context) {
		tasks = append(tasks, AgentTask{
			Name:           "docs-api",
			Description:    "Creating API documentation sections",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildAPIDocumentationPrompt,
			FallbackPrompt: buildDocumentationFallbackPrompt,
		})
	}

	// Architecture documentation
	if hasArchDocs(context) {
		tasks = append(tasks, AgentTask{
			Name:           "docs-architecture",
			Description:    "Creating architecture documentation sections",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildArchDocumentationPrompt,
			FallbackPrompt: buildDocumentationFallbackPrompt,
		})
	}

	// If no specific categories, fall back to single task
	if len(tasks) == 0 {
		tasks = append(tasks, AgentTask{
			Name:           "documentation-agent",
			Description:    "Creating documentation sections",
			MaxRetries:     defaultMaxRetries,
			Prompt:         buildDocumentationPrompt,
			FallbackPrompt: buildDocumentationFallbackPrompt,
		})
	}

	return tasks
}

const (
	readmeFile       = "readme.md"
	contributingFile = "contributing.md"
	licenseFile      = "license.md"
)

// Helper functions to categorize documentation
func hasCoreDocs(context *ProjectContext) bool {
	for _, doc := range context.MarkdownFiles {
		name := strings.ToLower(filepath.Base(doc.Path))
		if name == readmeFile || name == contributingFile || name == licenseFile {
			return true
		}
	}
	return false
}

func hasAPIDocs(context *ProjectContext) bool {
	for _, doc := range context.MarkdownFiles {
		name := strings.ToLower(filepath.Base(doc.Path))
		if strings.Contains(name, "api") || strings.Contains(name, "reference") || strings.Contains(name, "endpoint") {
			return true
		}
	}
	return false
}

func hasArchDocs(context *ProjectContext) bool {
	for _, doc := range context.MarkdownFiles {
		name := strings.ToLower(filepath.Base(doc.Path))
		if strings.Contains(name, "architect") || strings.Contains(name, "design") || strings.Contains(name, "structure") {
			return true
		}
	}
	return false
}

// countDocTasks counts how many documentation tasks are in the list
func countDocTasks(tasks []AgentTask) int {
	count := 0
	for _, task := range tasks {
		if strings.HasPrefix(task.Name, "doc") {
			count++
		}
	}
	return count
}

// TaskStatus represents the status of a running task
type TaskStatus struct {
	task       AgentTask
	status     string // "running", "retrying", "success", "failed"
	attempt    int
	mu         sync.RWMutex
	completed  bool
	startTime  time.Time
	duration   time.Duration
	lastError  error
	lineNumber int // Which line this task is displayed on
}

// TaskDisplay manages the visual display of all tasks
type TaskDisplay struct {
	tasks        []*TaskStatus
	spinnerIndex int
	mu           sync.Mutex
	startTime    time.Time
}

// Spinner frames for animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var bulletFrames = []string{"◉", "◎", "◉", "○"}

func ExecuteInitChain(agent AgentInfo, context *ProjectContext, providerConfig templates.ProviderConfig) (string, error) {
	logger.Info("🔗 Starting parallel agent task execution...")
	logger.Info("")

	// Initialize base configuration
	if err := initializeBaseConfigFile(context, providerConfig); err != nil {
		return "", fmt.Errorf("failed to initialize base config: %w", err)
	}
	logger.Info("✅ Initialized base ai-rulez.yaml")
	fmt.Printf("\n")

	// Get dynamic task list based on project
	initAgentTasks := getInitAgentTasks(context)

	// Initialize task status tracking
	startTime := time.Now()
	taskStatuses := make([]*TaskStatus, len(initAgentTasks))
	for i, task := range initAgentTasks {
		taskStatuses[i] = &TaskStatus{
			task:       task,
			status:     statusRunning,
			attempt:    1,
			startTime:  time.Now(),
			lineNumber: i,
		}
	}

	// Create task display manager
	display := &TaskDisplay{
		tasks:        taskStatuses,
		spinnerIndex: 0,
		startTime:    startTime,
	}

	// Display initial task list
	fmt.Printf("🚀 Running %d agent tasks in parallel...\n", len(initAgentTasks))
	if len(context.MarkdownFiles) > 10 {
		fmt.Printf("   📚 Large project detected: Documentation split into %d specialized tasks\n",
			countDocTasks(initAgentTasks))
	}
	fmt.Printf("\n")

	// Display all tasks initially
	display.renderAllTasks()

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

	// Start a goroutine to show animated status updates
	statusDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond) // Slower, smoother animation
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				display.updateDisplay()
			case <-statusDone:
				// Final display update
				display.renderAllTasks()
				fmt.Printf("\n")
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
			ts.status = statusFailed
			ts.completed = true
			ts.lastError = result.err
			failedTasks = append(failedTasks, ts.task.Name)
		} else {
			ts.status = statusSuccess
			ts.completed = true
			ts.duration = time.Since(ts.startTime)
			successCount++
		}
		ts.mu.Unlock()
	}

	// Show summary
	fmt.Printf("\n")
	if len(failedTasks) > 0 {
		logger.Warn(fmt.Sprintf("Completed %d/%d tasks successfully. Failed: %v",
			successCount, len(initAgentTasks), failedTasks))
		logger.Info("💡 Partial success: The configuration has been created with available content")
		logger.Info("   You can manually add missing sections or re-run with better connectivity")
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

	// Always return the current file content, even if some tasks failed
	data, err := os.ReadFile("ai-rulez.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Return success with current content - partial success is still valuable
	return string(data), nil
}

// initializeBaseConfigFile creates the initial ai-rulez.yaml with metadata and outputs
func initializeBaseConfigFile(context *ProjectContext, providerConfig templates.ProviderConfig) error {
	content := buildInitialConfigTemplate(context, providerConfig)
	return os.WriteFile("ai-rulez.yaml", []byte(content), 0o644)
}

// buildInitialConfigTemplate creates the initial YAML content with comments and guidance
func buildInitialConfigTemplate(context *ProjectContext, providerConfig templates.ProviderConfig) string {
	var sb strings.Builder

	// Schema and metadata
	sb.WriteString("$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json\n\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s\n", context.ProjectName)
	sb.WriteString("  version: \"1.0.0\"\n")
	sb.WriteString("  description: \"\" # Will be updated by project-agent\n\n")

	// Outputs section
	sb.WriteString("outputs:\n")
	if providerConfig.Claude {
		sb.WriteString("  - path: CLAUDE.md\n")
		sb.WriteString("  - path: .claude/agents/\n")
		sb.WriteString("    type: agent\n")
		sb.WriteString("    naming_scheme: '{name}.md'\n")
	}
	if providerConfig.Cursor {
		sb.WriteString("  - path: .cursor/rules/\n")
		sb.WriteString("    type: section\n")
		sb.WriteString("    naming_scheme: '{name}.md'\n")
	}
	if providerConfig.Windsurf {
		sb.WriteString("  - path: .windsurfrules\n")
	}
	if providerConfig.Copilot {
		sb.WriteString("  - path: .github/copilot-instructions.md\n")
	}
	if providerConfig.Gemini {
		sb.WriteString("  - path: GEMINI.md\n")
	}
	if providerConfig.Amp || providerConfig.Codex {
		sb.WriteString("  - path: AGENTS.md\n")
	}
	if providerConfig.Cline {
		sb.WriteString("  - path: .clinerules/\n")
		sb.WriteString("    type: section\n")
		sb.WriteString("    naming_scheme: '{name}.md'\n")
	}
	if providerConfig.ContinueDev {
		sb.WriteString("  - path: .continue/rules/\n")
		sb.WriteString("    type: section\n")
		sb.WriteString("    naming_scheme: '{name}.md'\n")
		sb.WriteString("  - path: .continue/prompts/ai_rulez_prompts.yaml\n")
		sb.WriteString("    template:\n")
		sb.WriteString("      type: builtin\n")
		sb.WriteString("      value: continue-prompts\n")
	}

	// Placeholder sections - agents will add to or replace these
	sb.WriteString("\n# Sections will be populated by infra-agent and documentation-agent\n")
	sb.WriteString("sections: []\n")

	sb.WriteString("\n# Rules will be populated by standards-agent\n")
	sb.WriteString("rules: []\n")

	sb.WriteString("\n# Agents will be populated by ai-agents\n")
	sb.WriteString("agents: []\n")

	sb.WriteString("\n# Commands will be populated by tooling-agent\n")
	sb.WriteString("commands: []\n")

	sb.WriteString("\n# MCP servers will be populated by tooling-agent\n")
	sb.WriteString("mcp_servers: []\n")

	return sb.String()
}

// executeAgentTaskWithStatus executes a single agent task with status updates
func executeAgentTaskWithStatus(task AgentTask, agent AgentInfo, context *ProjectContext, status *TaskStatus) error {
	// Execute with retries, updating status for each attempt
	var lastErr error
	startTime := time.Now()

	for attempt := 1; attempt <= task.MaxRetries; attempt++ {
		// Choose prompt: use fallback for retries if available
		var prompt string
		if attempt > 1 && task.FallbackPrompt != nil {
			prompt = task.FallbackPrompt(context)
		} else {
			prompt = task.Prompt(context)
		}
		// Update status for retry attempts
		if attempt > 1 {
			status.mu.Lock()
			status.status = statusRetrying
			status.attempt = attempt
			status.mu.Unlock()

			// Brief delay between retries with exponential backoff
			delay := time.Duration(attempt-1) * 2 * time.Second
			time.Sleep(delay)
		}

		// Use shorter timeout for first attempt, longer for retries with fallback
		timeout := 60 * time.Second // 1 minute for first attempt
		if attempt > 1 {
			timeout = 120 * time.Second // 2 minutes for fallback retries
		}

		// Execute the agent call
		_, err := invokeAgent(agent, prompt, timeout)

		if err == nil {
			// Success - update status and return
			status.mu.Lock()
			status.status = statusSuccess
			status.completed = true
			status.duration = time.Since(startTime)
			status.mu.Unlock()

			return nil
		}

		lastErr = err
		// Don't log individual attempts, the display shows them
	}

	// All attempts failed - update status
	status.mu.Lock()
	status.status = statusFailed
	status.completed = true
	status.lastError = lastErr
	status.duration = time.Since(startTime)
	status.mu.Unlock()

	return lastErr
}

// renderAllTasks displays all tasks with their current status
func (d *TaskDisplay) renderAllTasks() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Initial render - just print all tasks
	for _, ts := range d.tasks {
		d.renderTaskLine(ts)
	}
}

// updateDisplay updates the animated display
func (d *TaskDisplay) updateDisplay() {
	d.mu.Lock()
	d.spinnerIndex = (d.spinnerIndex + 1) % len(spinnerFrames)
	spinnerFrame := spinnerFrames[d.spinnerIndex]
	// Slow down bullet animation by using integer division
	bulletFrame := bulletFrames[(d.spinnerIndex/4)%len(bulletFrames)]
	d.mu.Unlock()

	// Save cursor position
	fmt.Printf("\033[s")

	// Move to start of task display
	for i := 0; i < len(d.tasks); i++ {
		fmt.Printf("\033[A")
	}

	// Update each task line
	for _, ts := range d.tasks {
		ts.mu.RLock()
		fmt.Printf("\r\033[K") // Clear line

		// Choose bullet based on status
		bullet := bulletFrame
		if ts.completed {
			switch ts.status {
			case statusSuccess:
				bullet = "✓"
			case statusFailed:
				bullet = "✗"
			}
		} else if ts.status == statusRetrying {
			bullet = "↻"
		}

		if !ts.completed {
			elapsed := time.Since(ts.startTime).Round(time.Second)
			if ts.attempt > 1 {
				fmt.Printf("%s %s [%v] (retry %d)", spinnerFrame, ts.task.Name, elapsed, ts.attempt-1)
			} else {
				fmt.Printf("%s %s [%v]", spinnerFrame, ts.task.Name, elapsed)
			}
		} else {
			fmt.Printf("%s %s", bullet, ts.task.Name)
			if ts.status == statusFailed && ts.lastError != nil {
				fmt.Printf(" ✗ %s", getErrorSummary(ts.lastError))
			}
		}

		ts.mu.RUnlock()
		fmt.Printf("\n")
	}

	// Restore cursor position
	fmt.Printf("\033[u")
}

// renderTaskLine renders a single task line
func (d *TaskDisplay) renderTaskLine(ts *TaskStatus) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	fmt.Printf("\r\033[K") // Clear line

	// Choose symbol based on status
	symbol := "◉"
	switch ts.status {
	case statusSuccess:
		symbol = "✓"
	case statusFailed:
		symbol = "✗"
	case statusRetrying:
		symbol = "↻"
	}

	if !ts.completed {
		elapsed := time.Since(ts.startTime).Round(time.Second)
		spinner := spinnerFrames[d.spinnerIndex]
		if ts.attempt > 1 {
			fmt.Printf("%s %s [%v] (retry %d)", spinner, ts.task.Name, elapsed, ts.attempt-1)
		} else {
			fmt.Printf("%s %s [%v]", spinner, ts.task.Name, elapsed)
		}
	} else {
		fmt.Printf("%s %s", symbol, ts.task.Name)
		if ts.status == statusFailed && ts.lastError != nil {
			fmt.Printf(" ✗ %s", getErrorSummary(ts.lastError))
		}
	}

	fmt.Printf("\n")
}

// validateGeneratedConfig validates the generated ai-rulez.yaml file
func validateGeneratedConfig() error {
	// Try to load and validate the configuration
	cfg, err := config.LoadConfig("ai-rulez.yaml")
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
