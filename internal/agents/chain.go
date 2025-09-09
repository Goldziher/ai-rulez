package agents

import (
	"fmt"
	"os"
	"strconv"
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

// getInitAgentTasks returns the agent tasks using configurable max agents with smart heuristics
func getInitAgentTasks(context *ProjectContext, providerConfig templates.ProviderConfig) []AgentTask {
	return distributeSpecialistTasks(context, providerConfig)
}

// getMaxAgents returns the configured maximum number of concurrent agents
func getMaxAgents() int {
	// Check environment variable first
	if maxStr := os.Getenv("AI_RULEZ_MAX_AGENTS"); maxStr != "" {
		if maxVal, err := strconv.Atoi(maxStr); err == nil && maxVal > 0 && maxVal <= 20 {
			return maxVal
		}
	}
	// Default to 5 agents (optimal balance)
	return 5
}

// distributeSpecialistTasks creates generic tasks using a single specialist agent approach
func distributeSpecialistTasks(context *ProjectContext, providerConfig templates.ProviderConfig) []AgentTask {
	workload := assessProjectWorkload(context)

	// Focus only on core documentation: sections and rules
	baseTasks := []string{"project description", "coding standards", "documentation sections"}

	// Only add agent definitions for providers that support them (Claude and Continue.dev)
	if providerConfig.Claude || providerConfig.ContinueDev {
		baseTasks = append(baseTasks, "agent definitions")
	}

	// Apply intelligent splitting based on workload (without concurrency limits)
	tasks := applySplittingHeuristics(baseTasks, workload, workload.suggestedAgents)

	return createGenericTasks(tasks, context)
}

// ProjectWorkload represents the complexity assessment of a project
type ProjectWorkload struct {
	complexity            int
	documentationHeavy    bool
	complexInfrastructure bool
	multiLanguage         bool
	largeCodebase         bool
	suggestedAgents       int
}

// assessProjectWorkload analyzes project characteristics to determine optimal task distribution
func assessProjectWorkload(context *ProjectContext) ProjectWorkload {
	workload := ProjectWorkload{}

	// Count documentation files
	docCount := len(context.MarkdownFiles)
	workload.documentationHeavy = docCount > 10

	// Analyze codebase complexity
	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		workload.complexInfrastructure = info.HasDocker || len(info.TechStack) > 5
		workload.multiLanguage = len(info.TechStack) > 3
		workload.largeCodebase = len(context.MarkdownFiles) > 15 // proxy for large codebase
	}

	// Calculate complexity score (1-10)
	workload.complexity = 3 // base complexity
	if workload.documentationHeavy {
		workload.complexity += 2
	}
	if workload.complexInfrastructure {
		workload.complexity += 2
	}
	if workload.multiLanguage {
		workload.complexity++
	}
	if workload.largeCodebase {
		workload.complexity += 2
	}

	// Suggest optimal number of agents
	workload.suggestedAgents = min(8, max(3, workload.complexity))

	return workload
}

// applySplittingHeuristics intelligently splits tasks based on project workload
func applySplittingHeuristics(baseTasks []string, workload ProjectWorkload, maxAgents int) []string {
	tasks := make([]string, len(baseTasks))
	copy(tasks, baseTasks)

	// Split documentation if heavy and we have capacity
	if workload.documentationHeavy && maxAgents >= 4 {
		tasks = splitDocumentationTasks(tasks)
	}

	// Split infrastructure if complex and we have capacity
	if workload.complexInfrastructure && maxAgents >= 5 {
		tasks = splitInfrastructureTasks(tasks)
	}

	// Ensure we don't exceed max agents
	if len(tasks) > maxAgents {
		tasks = tasks[:maxAgents]
	}

	return tasks
}

// splitDocumentationTasks splits documentation analysis into multiple tasks
func splitDocumentationTasks(tasks []string) []string {
	for i, task := range tasks {
		if task == "documentation analysis" {
			// Replace single task with two
			tasks[i] = "documentation analysis #1"
			tasks = append(tasks, "documentation analysis #2")
			break
		}
	}
	return tasks
}

// splitInfrastructureTasks splits infrastructure analysis into multiple tasks
func splitInfrastructureTasks(tasks []string) []string {
	for i, task := range tasks {
		if task == "infrastructure analysis" {
			// Replace single task with two
			tasks[i] = "infrastructure analysis #1"
			tasks = append(tasks, "infrastructure analysis #2")
			break
		}
	}
	return tasks
}

// createGenericTasks converts task names into AgentTask structs using specialist prompts
func createGenericTasks(taskNames []string, context *ProjectContext) []AgentTask {
	tasks := make([]AgentTask, len(taskNames))

	for i, name := range taskNames {
		tasks[i] = AgentTask{
			Name:           generateTaskID(name, i),
			Description:    name,
			MaxRetries:     defaultMaxRetries,
			Prompt:         selectPromptForTask(name),
			FallbackPrompt: selectFallbackPromptForTask(name),
		}
	}

	return tasks
}

// generateTaskID creates unique internal IDs for tasks
func generateTaskID(taskName string, index int) string {
	// Convert "documentation analysis #1" -> "doc-analysis-1"
	name := strings.ToLower(taskName)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "#", "")
	return name
}

// selectPromptForTask maps generic task names to specific prompt functions
func selectPromptForTask(taskName string) func(*ProjectContext) string {
	switch {
	case strings.Contains(taskName, "project description"):
		return buildProjectAnalysisPrompt
	case strings.Contains(taskName, "documentation sections"):
		return buildDocumentationPrompt
	case strings.Contains(taskName, "coding standards"):
		return buildStandardsPrompt
	case strings.Contains(taskName, "agent definitions"):
		return buildAgentDefinitionsPrompt
	default:
		return buildProjectAnalysisPrompt
	}
}

// selectFallbackPromptForTask maps generic task names to fallback prompt functions
func selectFallbackPromptForTask(taskName string) func(*ProjectContext) string {
	switch {
	case strings.Contains(taskName, "project description"):
		return buildProjectAnalysisFallbackPrompt
	case strings.Contains(taskName, "documentation sections"):
		return buildDocumentationFallbackPrompt
	case strings.Contains(taskName, "coding standards"):
		return buildStandardsFallbackPrompt
	case strings.Contains(taskName, "agent definitions"):
		return buildAgentDefinitionsFallbackPrompt
	default:
		return buildProjectAnalysisFallbackPrompt
	}
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

	// Get dynamic task list based on project and provider
	initAgentTasks := getInitAgentTasks(context, providerConfig)

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
	fmt.Printf("\n")

	// Display all tasks initially
	display.renderAllTasks()

	// Execute tasks in waves based on maxAgents limit
	maxAgents := getMaxAgents()
	failedTasks, successCount := executeTasksInWaves(initAgentTasks, taskStatuses, agent, context, display, maxAgents)

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
		logger.Info("🔧 Attempting to repair validation errors...")

		if repairErr := repairConfigurationWithRetries(err, context); repairErr != nil {
			logger.Error(fmt.Sprintf("❌ Failed to repair configuration: %v", repairErr))
			logger.Info("The file was created but may need manual review")
		} else {
			logger.Success("✅ Configuration repaired successfully")
		}
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

	// Placeholder arrays - agents will populate these
	sb.WriteString("\nsections: []\n")
	sb.WriteString("rules: []\n")
	sb.WriteString("agents: []\n")
	sb.WriteString("commands: []\n")
	sb.WriteString("mcp_servers:\n")
	sb.WriteString("  - name: \"ai-rulez\"\n")
	sb.WriteString("    command: \"ai-rulez\"\n")
	sb.WriteString("    args: [\"mcp\"]\n")
	sb.WriteString("    description: \"AI-Rulez MCP server for configuration management\"\n")

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

// repairConfigurationWithRetries attempts to repair validation errors with retries
func repairConfigurationWithRetries(validationErr error, context *ProjectContext) error {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Info(fmt.Sprintf("🔧 Repair attempt %d/%d", attempt, maxRetries))

		if err := runRepairAgent(validationErr, context); err != nil {
			logger.Warn(fmt.Sprintf("Repair attempt %d failed: %v", attempt, getErrorSummary(err)))
			if attempt == maxRetries {
				return fmt.Errorf("all repair attempts failed, last error: %w", err)
			}
			continue
		}

		// Re-validate after repair
		if err := validateGeneratedConfig(); err != nil {
			logger.Warn(fmt.Sprintf("Still has validation issues after attempt %d: %v", attempt, getErrorSummary(err)))
			validationErr = err // Update for next retry
			if attempt == maxRetries {
				return fmt.Errorf("configuration still invalid after %d repair attempts: %w", maxRetries, err)
			}
			continue
		}

		logger.Success(fmt.Sprintf("✅ Configuration repaired successfully on attempt %d", attempt))
		return nil
	}

	return fmt.Errorf("unexpected end of repair attempts")
}

// runRepairAgent runs a single repair agent to fix validation errors
func runRepairAgent(validationErr error, context *ProjectContext) error {
	task := AgentTask{
		Name:        "validation-repair",
		Description: "Fixing validation errors",
		MaxRetries:  1, // Single attempt, higher-level retry handles multiple attempts
		Prompt: func(ctx *ProjectContext) string {
			return buildRepairPrompt(validationErr, ctx)
		},
	}

	// Create agent info for repair
	agent := AgentInfo{
		ID:      "claude",
		Command: "claude",
		Display: "Claude (Repair)",
	}

	// Create status tracker
	status := &TaskStatus{
		task:      task,
		status:    statusRunning,
		attempt:   1,
		startTime: time.Now(),
	}

	return executeAgentTaskWithStatus(task, agent, context, status)
}

// executeTasksInWaves executes agent tasks in batches/waves based on maxAgents limit
func executeTasksInWaves(tasks []AgentTask, taskStatuses []*TaskStatus, agent AgentInfo, context *ProjectContext, display *TaskDisplay, maxAgents int) ([]string, int) {
	totalTasks := len(tasks)
	var failedTasks []string
	successCount := 0

	// Calculate number of waves needed
	numWaves := (totalTasks + maxAgents - 1) / maxAgents // Ceiling division

	if numWaves > 1 {
		logger.Info(fmt.Sprintf("📋 Executing %d tasks in %d waves (%d agents per wave)", totalTasks, numWaves, maxAgents))
	}

	// Execute tasks in waves
	for wave := 0; wave < numWaves; wave++ {
		startIdx := wave * maxAgents
		endIdx := min(startIdx+maxAgents, totalTasks)

		if numWaves > 1 {
			logger.Info(fmt.Sprintf("🌊 Wave %d/%d: Running tasks %d-%d", wave+1, numWaves, startIdx+1, endIdx))
		}

		// Execute current wave
		waveFailedTasks, waveSuccessCount := executeWave(tasks[startIdx:endIdx], taskStatuses[startIdx:endIdx], agent, context, display, startIdx)

		failedTasks = append(failedTasks, waveFailedTasks...)
		successCount += waveSuccessCount
	}

	return failedTasks, successCount
}

// executeWave executes a single wave of agent tasks in parallel
func executeWave(waveTasks []AgentTask, waveStatuses []*TaskStatus, agent AgentInfo, context *ProjectContext, display *TaskDisplay, baseIndex int) ([]string, int) {
	// Structure to hold task results
	type taskResult struct {
		taskIndex int
		err       error
	}

	// Channel to collect results
	results := make(chan taskResult, len(waveTasks))

	// WaitGroup for synchronization
	var wg sync.WaitGroup

	// Launch wave tasks in parallel
	for i, task := range waveTasks {
		wg.Add(1)
		go func(localIndex int, t AgentTask) {
			defer wg.Done()

			globalIndex := baseIndex + localIndex
			err := executeAgentTaskWithStatus(t, agent, context, waveStatuses[localIndex])
			results <- taskResult{taskIndex: globalIndex, err: err}
		}(i, task)
	}

	// Start a goroutine to show animated status updates
	statusDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				display.updateDisplay()
			case <-statusDone:
				// Final display update
				display.renderAllTasks()
				return
			}
		}
	}()

	// Wait for all wave tasks to complete
	wg.Wait()
	close(statusDone)
	close(results)

	// Process wave results
	var failedTasks []string
	successCount := 0

	for result := range results {
		localIndex := result.taskIndex - baseIndex
		ts := waveStatuses[localIndex]
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

	return failedTasks, successCount
}
