package agents

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
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
	statusPending  = "pending"
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
		if maxVal, err := strconv.Atoi(maxStr); err == nil && maxVal > 0 && maxVal <= 10 {
			return maxVal
		}
	}
	// Default to 5 agents (optimal balance for most systems)
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

	// Count total files for better size assessment
	totalFiles := 0
	if context.DirectoryStructure != nil {
		for _, files := range context.DirectoryStructure {
			totalFiles += len(files)
		}
	}

	// Analyze codebase complexity
	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		workload.complexInfrastructure = info.HasDocker || len(info.TechStack) > 5
		workload.multiLanguage = len(info.TechStack) > 3
		workload.largeCodebase = totalFiles > 100 || docCount > 15
	}

	// Check for monorepo characteristics
	isMonorepo := context.RepoType == "monorepo" || len(context.PackageLocations) > 1

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
	if isMonorepo {
		workload.complexity += 2
	}

	// Suggest optimal number of agents based on complexity
	// For large codebases, suggest more agents (up to 10)
	switch {
	case workload.complexity >= 8:
		workload.suggestedAgents = 10
	case workload.complexity >= 6:
		workload.suggestedAgents = 8
	case workload.complexity >= 4:
		workload.suggestedAgents = 5
	default:
		workload.suggestedAgents = 3
	}

	return workload
}

// applySplittingHeuristics intelligently splits tasks based on project workload
func applySplittingHeuristics(baseTasks []string, workload ProjectWorkload, maxAgents int) []string {
	var expandedTasks []string

	for _, task := range baseTasks {
		expanded := expandTask(task, workload, maxAgents)
		expandedTasks = append(expandedTasks, expanded...)
	}

	// Ensure we don't exceed max agents but use at least what we have
	if len(expandedTasks) > maxAgents {
		return prioritizeTasks(expandedTasks, maxAgents)
	}

	return expandedTasks
}

// expandTask expands a single task based on workload and available agents
func expandTask(task string, workload ProjectWorkload, maxAgents int) []string {
	switch task {
	case "coding standards":
		return expandCodingStandards(workload, maxAgents)
	case "documentation sections":
		return expandDocumentation(workload, maxAgents)
	case "agent definitions":
		return expandAgentDefinitions(workload, maxAgents)
	default:
		return []string{task}
	}
}

// expandCodingStandards expands coding standards task based on project characteristics
func expandCodingStandards(workload ProjectWorkload, maxAgents int) []string {
	if !workload.largeCodebase || maxAgents < 6 {
		return []string{"coding standards"}
	}

	tasks := []string{
		"error handling standards",
		"code style standards",
		"testing standards",
	}

	if workload.multiLanguage {
		tasks = append(tasks, "language-specific standards")
	}

	return tasks
}

// expandDocumentation expands documentation task based on project characteristics
func expandDocumentation(workload ProjectWorkload, maxAgents int) []string {
	if !workload.documentationHeavy || maxAgents < 5 {
		return []string{"documentation sections"}
	}

	tasks := []string{
		"setup documentation",
		"architecture documentation",
	}

	if workload.complexInfrastructure {
		tasks = append(tasks, "deployment documentation")
	}

	return tasks
}

// expandAgentDefinitions expands agent definitions task based on project complexity
func expandAgentDefinitions(workload ProjectWorkload, maxAgents int) []string {
	if workload.complexity < 6 || maxAgents < 4 {
		return []string{"agent definitions"}
	}

	return []string{
		"core agents",
		"specialized agents",
	}
}

// prioritizeTasks prioritizes tasks when there are more tasks than available agents
func prioritizeTasks(tasks []string, maxAgents int) []string {
	// Always include project description
	prioritized := []string{"project description"}
	remaining := maxAgents - 1

	// Add other tasks up to limit
	for _, task := range tasks[1:] {
		if remaining > 0 {
			prioritized = append(prioritized, task)
			remaining--
		} else {
			break
		}
	}

	return prioritized
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
	fmt.Printf("[DEBUG] Selecting prompt for task: '%s'\n", taskName)

	switch {
	case strings.Contains(taskName, "project description"):
		fmt.Printf("[DEBUG] Task '%s' matched project description -> buildProjectAnalysisPrompt\n", taskName)
		return buildProjectAnalysisPrompt
	case strings.Contains(taskName, "documentation"):
		fmt.Printf("[DEBUG] Task '%s' matched documentation -> buildDocumentationPrompt\n", taskName)
		return buildDocumentationPrompt
	case strings.Contains(taskName, "standards"):
		fmt.Printf("[DEBUG] Task '%s' matched standards -> buildStandardsPrompt\n", taskName)
		return buildStandardsPrompt
	case strings.Contains(taskName, "agents"):
		fmt.Printf("[DEBUG] Task '%s' matched agents -> buildAgentDefinitionsPrompt\n", taskName)
		return buildAgentDefinitionsPrompt
	default:
		fmt.Printf("[DEBUG] Task '%s' didn't match any pattern -> DEFAULT buildProjectAnalysisPrompt\n", taskName)
		return buildProjectAnalysisPrompt
	}
}

// selectFallbackPromptForTask maps generic task names to fallback prompt functions
func selectFallbackPromptForTask(taskName string) func(*ProjectContext) string {
	switch {
	case strings.Contains(taskName, "project description"):
		return buildProjectAnalysisFallbackPrompt
	case strings.Contains(taskName, "documentation") || strings.Contains(taskName, "setup documentation") || strings.Contains(taskName, "architecture documentation"):
		return buildDocumentationFallbackPrompt
	case strings.Contains(taskName, "standards") || strings.Contains(taskName, "error handling standards") || strings.Contains(taskName, "code style standards") || strings.Contains(taskName, "testing standards"):
		return buildStandardsFallbackPrompt
	case strings.Contains(taskName, "agents") || strings.Contains(taskName, "core agents") || strings.Contains(taskName, "specialized agents"):
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

func ExecuteInitChain(agent AgentInfo, context *ProjectContext, providerConfig templates.ProviderConfig) (string, error) {
	fmt.Printf("🔗 Starting parallel agent task execution...\n")

	// Initialize base configuration
	if err := initializeBaseConfigFile(context, providerConfig); err != nil {
		return "", fmt.Errorf("failed to initialize base config: %w", err)
	}
	fmt.Printf("✅ Initialized base ai-rulez.yaml\n")

	// Get dynamic task list based on project and provider
	initAgentTasks := getInitAgentTasks(context, providerConfig)

	// Initialize task status tracking
	startTime := time.Now()
	taskStatuses := make([]*TaskStatus, len(initAgentTasks))
	for i, task := range initAgentTasks {
		taskStatuses[i] = &TaskStatus{
			task:       task,
			status:     statusPending,
			attempt:    0,
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

	// Execute tasks in waves based on maxAgents limit
	maxAgents := getMaxAgents()
	failedTasks, successCount := executeTasksInWaves(initAgentTasks, taskStatuses, agent, context, display, maxAgents)

	// Show summary
	fmt.Printf("\n")
	if len(failedTasks) > 0 {
		fmt.Printf("⚠️  Completed %d/%d tasks successfully. Failed: %v\n",
			successCount, len(initAgentTasks), failedTasks)
		fmt.Printf("💡 Partial success: The configuration has been created with available content\n")
		fmt.Printf("   You can manually add missing sections or re-run with better connectivity\n")
	} else {
		fmt.Println("✅ All agent tasks completed successfully")
	}

	// CLI commands ensure valid configuration - no validation needed
	fmt.Println("✅ Configuration generated successfully")

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
	// Update context with existing configuration before each task
	if err := context.UpdateContextFromConfig(); err != nil {
		fmt.Printf("[DEBUG] Warning: Failed to update context from config: %v\n", err)
		// Continue anyway - this is not critical
	}

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

		// Use reasonable timeout for agent tasks
		timeout := 90 * time.Second // 90 seconds for all attempts

		// Execute the agent call and capture output
		fmt.Printf("[DEBUG] Invoking agent for task: %s (attempt %d)\n", task.Name, attempt)
		logger.Debug("Invoking agent", "task", task.Name, "attempt", attempt)
		output, err := invokeAgent(agent, prompt, timeout)

		if err == nil {
			fmt.Printf("[DEBUG] Agent output received for %s: %d bytes\n", task.Name, len(output))
			logger.Debug("Agent output received", "task", task.Name, "outputLength", len(output))
			if output != "" {
				// Log first 500 chars of output for debugging
				preview := output
				if len(preview) > 500 {
					preview = preview[:500] + "..."
				}
				fmt.Printf("[DEBUG] Output preview for %s:\n%s\n", task.Name, preview)
				logger.Debug("Agent output preview", "task", task.Name, "preview", preview)
			}
			// Parse JSON from agent output and apply to config
			if err := executeAgentCommands(output, task.Name); err != nil {
				lastErr = fmt.Errorf("failed to process agent response: %w", err)
				fmt.Printf("[DEBUG] Response processing failed for %s: %v\n", task.Name, err)
				logger.Debug("Response processing failed", "task", task.Name, "error", err.Error())
				continue
			}

			// Success - update status and mark task as completed
			status.mu.Lock()
			status.status = statusSuccess
			status.completed = true
			status.duration = time.Since(startTime)
			status.mu.Unlock()

			// Mark task as completed in context for future agents
			context.AddCompletedTask(task.Name)
			fmt.Printf("[DEBUG] Task '%s' completed successfully and added to context\n", task.Name)

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

	// Sort tasks: completed first, then running, then pending
	var completedTasks, runningTasks, pendingTasks []*TaskStatus
	for _, ts := range d.tasks {
		switch {
		case ts.completed:
			completedTasks = append(completedTasks, ts)
		case ts.status == statusPending && ts.attempt == 0:
			pendingTasks = append(pendingTasks, ts)
		default:
			runningTasks = append(runningTasks, ts)
		}
	}

	// Render completed tasks first
	for _, ts := range completedTasks {
		d.renderTaskLine(ts)
	}
	// Then running tasks
	for _, ts := range runningTasks {
		d.renderTaskLine(ts)
	}
	// Then pending tasks
	for _, ts := range pendingTasks {
		d.renderTaskLine(ts)
	}
}

// updateDisplay updates the animated display
func (d *TaskDisplay) updateDisplay() {
	d.mu.Lock()
	d.spinnerIndex = (d.spinnerIndex + 1) % len(spinnerFrames)
	spinnerFrame := spinnerFrames[d.spinnerIndex]
	d.mu.Unlock()

	// Save cursor position
	fmt.Printf("\033[s")

	// Move to start of task display
	for i := 0; i < len(d.tasks); i++ {
		fmt.Printf("\033[A")
	}

	// Sort tasks: completed first, then running, then pending
	var completedTasks, runningTasks, pendingTasks []*TaskStatus
	for _, ts := range d.tasks {
		switch {
		case ts.completed:
			completedTasks = append(completedTasks, ts)
		case ts.status == statusPending && ts.attempt == 0:
			pendingTasks = append(pendingTasks, ts)
		default:
			runningTasks = append(runningTasks, ts)
		}
	}

	// Render all tasks in order
	allTasks := make([]*TaskStatus, 0, len(completedTasks)+len(runningTasks)+len(pendingTasks))
	allTasks = append(allTasks, completedTasks...)
	allTasks = append(allTasks, runningTasks...)
	allTasks = append(allTasks, pendingTasks...)

	for _, ts := range allTasks {
		ts.mu.RLock()
		fmt.Printf("\r\033[K") // Clear line

		// Choose symbol based on status
		switch {
		case ts.completed && ts.status == statusSuccess:
			fmt.Printf("✓ %s", ts.task.Name)
		case ts.completed && ts.status == statusFailed:
			fmt.Printf("✗ %s", ts.task.Name)
			if ts.lastError != nil {
				fmt.Printf(" (%s)", getErrorSummary(ts.lastError))
			}
		case ts.status == statusPending && ts.attempt == 0:
			fmt.Printf("○ %s (pending)", ts.task.Name)
		default:
			elapsed := time.Since(ts.startTime).Round(time.Second)
			if ts.attempt > 1 {
				fmt.Printf("%s %s [%v] (retry %d)", spinnerFrame, ts.task.Name, elapsed, ts.attempt-1)
			} else {
				fmt.Printf("%s %s [%v]", spinnerFrame, ts.task.Name, elapsed)
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
	case statusPending:
		symbol = "○"
	case statusSuccess:
		symbol = "✓"
	case statusFailed:
		symbol = "✗"
	case statusRetrying:
		symbol = "↻"
	}

	if !ts.completed {
		if ts.status == statusPending && ts.attempt == 0 {
			fmt.Printf("%s %s (pending)", symbol, ts.task.Name)
		} else {
			elapsed := time.Since(ts.startTime).Round(time.Second)
			spinner := spinnerFrames[d.spinnerIndex]
			if ts.attempt > 1 {
				fmt.Printf("%s %s [%v] (retry %d)", spinner, ts.task.Name, elapsed, ts.attempt-1)
			} else {
				fmt.Printf("%s %s [%v]", spinner, ts.task.Name, elapsed)
			}
		}
	} else {
		fmt.Printf("%s %s", symbol, ts.task.Name)
		if ts.status == statusFailed && ts.lastError != nil {
			fmt.Printf(" ✗ %s", getErrorSummary(ts.lastError))
		}
	}

	fmt.Printf("\n")
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

// executeAgentCommands parses JSON from agent output and updates the config
func executeAgentCommands(output string, taskName string) error {
	if output == "" {
		return nil
	}

	// Determine the response type based on task name
	responseType := getResponseTypeForTask(taskName)
	if responseType == "" {
		fmt.Printf("[DEBUG] Unknown task type: %s\n", taskName)
		return fmt.Errorf("unknown task type: %s", taskName)
	}

	// Parse the JSON response
	response, err := ParseAgentOutput(output, responseType)
	if err != nil {
		fmt.Printf("[DEBUG] Failed to parse agent output for %s: %v\n", taskName, err)
		logger.Warn("Failed to parse agent output", "task", taskName, "error", err.Error())
		return fmt.Errorf("failed to parse agent output: %w", err)
	}

	// Apply the response to the config
	if err := applyResponseToConfig(response, responseType); err != nil {
		fmt.Printf("[DEBUG] Failed to apply response for %s: %v\n", taskName, err)
		logger.Warn("Failed to apply response", "task", taskName, "error", err.Error())
		return fmt.Errorf("failed to apply response: %w", err)
	}

	fmt.Printf("[DEBUG] Successfully processed response for %s\n", taskName)
	logger.Debug("Successfully processed response", "task", taskName)

	return nil
}

// Response type constants
const (
	responseTypeMetadata = "metadata"
	responseTypeSections = "sections"
	responseTypeRules    = "rules"
	responseTypeAgents   = "agents"
)

// getResponseTypeForTask maps task names to response types
func getResponseTypeForTask(taskName string) string {
	// Normalize task name by replacing hyphens with spaces
	normalizedName := strings.ReplaceAll(taskName, "-", " ")

	switch {
	case strings.Contains(normalizedName, "project description"):
		return responseTypeMetadata
	case strings.Contains(normalizedName, "documentation"):
		return responseTypeSections
	case strings.Contains(normalizedName, "standards"):
		return responseTypeRules
	case strings.Contains(normalizedName, "agent") || strings.Contains(normalizedName, "agents"):
		return responseTypeAgents
	default:
		return ""
	}
}

// applyResponseToConfig applies the parsed response to the configuration file
func applyResponseToConfig(response interface{}, responseType string) error {
	// Load the current configuration
	configPath := "ai_rulez.yaml"

	// Read the current config file if it exists
	var config map[string]interface{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		// File doesn't exist, create a new config
		config = make(map[string]interface{})
	} else {
		// Parse existing YAML
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
		if config == nil {
			config = make(map[string]interface{})
		}
	}

	// Apply the response based on type
	switch responseType {
	case responseTypeMetadata:
		err = applyMetadataResponse(config, response)
	case responseTypeRules:
		err = applyRulesResponse(config, response)
	case responseTypeSections:
		err = applySectionsResponse(config, response)
	case responseTypeAgents:
		err = applyAgentsResponse(config, response)
	default:
		return fmt.Errorf("unknown response type: %s", responseType)
	}

	if err != nil {
		return err
	}

	// Write the updated config back
	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// applyMetadataResponse applies metadata response to config
func applyMetadataResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*MetadataResponse)
	if !ok {
		return fmt.Errorf("invalid metadata response type")
	}
	if config["metadata"] == nil {
		config["metadata"] = make(map[string]interface{})
	}
	metadata, ok := config["metadata"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("metadata is not a map")
	}
	metadata["description"] = resp.Description
	return nil
}

// extractExistingRules extracts existing rules from config into structured form
func extractExistingRules(config map[string]interface{}) (rules []interface{}, existingRules []RuleResponse, existingNames map[string]bool, err error) {
	existingRules = []RuleResponse{}
	existingNames = make(map[string]bool)
	rules = []interface{}{}

	if config["rules"] == nil {
		return rules, existingRules, existingNames, nil
	}

	var ok bool
	rules, ok = config["rules"].([]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("rules is not a slice")
	}

	for _, r := range rules {
		ruleMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		name, nameOk := ruleMap["name"].(string)
		if !nameOk {
			continue
		}

		existingNames[name] = true

		// Convert to RuleResponse for similarity checking
		priority, pOk := ruleMap["priority"].(string)
		content, cOk := ruleMap["content"].(string)
		if pOk && cOk {
			existingRules = append(existingRules, RuleResponse{
				Name:     name,
				Priority: priority,
				Content:  content,
			})
		}
	}

	return rules, existingRules, existingNames, nil
}

// updateExistingRule updates an existing rule in place
func updateExistingRule(rules []interface{}, existingRules []RuleResponse, similarRule *RuleResponse, merged RuleResponse) {
	// Update the existing rule in place
	for i, r := range rules {
		ruleMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := ruleMap["name"].(string)
		if !ok || name != similarRule.Name {
			continue
		}

		rules[i] = map[string]interface{}{
			"name":     merged.Name,
			"priority": merged.Priority,
			"content":  merged.Content,
		}

		// Update tracking arrays
		for j := range existingRules {
			if existingRules[j].Name == similarRule.Name {
				existingRules[j] = merged
				break
			}
		}

		fmt.Printf("[DEBUG] Merged similar rules into '%s'\n", merged.Name)
		break
	}
}

// applyRulesResponse applies rules response to config with semantic deduplication
func applyRulesResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*RulesResponse)
	if !ok {
		return fmt.Errorf("invalid rules response type")
	}

	rules, existingRules, existingNames, err := extractExistingRules(config)
	if err != nil {
		return err
	}

	// Initialize semantic similarity checker
	similarity := NewContentSimilarity(0.75) // 75% similarity threshold

	// Process each new rule with semantic deduplication
	for _, newRule := range resp.Rules {
		// Skip if exact name match exists
		if existingNames[newRule.Name] {
			fmt.Printf("[DEBUG] Skipping rule '%s' - exact name match exists\n", newRule.Name)
			continue
		}

		// Check for semantic similarity with existing rules
		similarRule, simScore := similarity.FindSimilarRule(newRule, existingRules)
		if similarRule != nil {
			fmt.Printf("[DEBUG] Found similar rule for '%s' -> '%s' (%.2f similarity)\n",
				newRule.Name, similarRule.Name, simScore)

			// Merge the rules, keeping the more detailed version
			merged := similarity.MergeRules(*similarRule, newRule)
			updateExistingRule(rules, existingRules, similarRule, merged)
		} else {
			// No similar rule found, add as new rule
			fmt.Printf("[DEBUG] Adding new unique rule: '%s'\n", newRule.Name)
			rules = append(rules, map[string]interface{}{
				"name":     newRule.Name,
				"priority": newRule.Priority,
				"content":  newRule.Content,
			})
			existingNames[newRule.Name] = true
			existingRules = append(existingRules, newRule)
		}
	}

	config["rules"] = rules
	return nil
}

// extractExistingSections extracts existing sections from config into structured form
func extractExistingSections(config map[string]interface{}) (sections []interface{}, existingSections []SectionResponse, existingNames map[string]bool, err error) {
	existingSections = []SectionResponse{}
	existingNames = make(map[string]bool)
	sections = []interface{}{}

	if config["sections"] == nil {
		return sections, existingSections, existingNames, nil
	}

	var ok bool
	sections, ok = config["sections"].([]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("sections is not a slice")
	}

	for _, s := range sections {
		sectionMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		name, nameOk := sectionMap["name"].(string)
		if !nameOk {
			continue
		}

		existingNames[name] = true

		// Convert to SectionResponse for similarity checking
		priority, pOk := sectionMap["priority"].(string)
		content, cOk := sectionMap["content"].(string)
		if pOk && cOk {
			existingSections = append(existingSections, SectionResponse{
				Name:     name,
				Priority: priority,
				Content:  content,
			})
		}
	}

	return sections, existingSections, existingNames, nil
}

// updateExistingSection updates an existing section in place
func updateExistingSection(sections []interface{}, existingSections []SectionResponse, similarSection *SectionResponse, newSection SectionResponse) {
	// For sections, prefer to merge content rather than replace
	for i, s := range sections {
		sectionMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := sectionMap["name"].(string)
		if !ok || name != similarSection.Name {
			continue
		}

		// Choose the more comprehensive content
		mergedContent := similarSection.Content
		if len(newSection.Content) > len(similarSection.Content) {
			mergedContent = newSection.Content
		}

		sections[i] = map[string]interface{}{
			"name":     similarSection.Name, // Keep original name
			"priority": newSection.Priority, // Use new priority if different
			"content":  mergedContent,
		}

		// Update tracking array
		for j := range existingSections {
			if existingSections[j].Name == similarSection.Name {
				existingSections[j].Content = mergedContent
				existingSections[j].Priority = newSection.Priority
				break
			}
		}

		fmt.Printf("[DEBUG] Merged similar sections into '%s'\n", similarSection.Name)
		break
	}
}

// applySectionsResponse applies sections response to config with semantic deduplication
func applySectionsResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*SectionsResponse)
	if !ok {
		return fmt.Errorf("invalid sections response type")
	}

	sections, existingSections, existingNames, err := extractExistingSections(config)
	if err != nil {
		return err
	}

	// Initialize semantic similarity checker
	similarity := NewContentSimilarity(0.65) // Lower threshold for sections (more content variety expected)

	// Process each new section with semantic deduplication
	for _, newSection := range resp.Sections {
		// Skip if exact name match exists
		if existingNames[newSection.Name] {
			fmt.Printf("[DEBUG] Skipping section '%s' - exact name match exists\n", newSection.Name)
			continue
		}

		// Check for semantic similarity with existing sections
		similarSection, simScore := similarity.FindSimilarSection(newSection, existingSections)
		if similarSection != nil {
			fmt.Printf("[DEBUG] Found similar section for '%s' -> '%s' (%.2f similarity)\n",
				newSection.Name, similarSection.Name, simScore)

			updateExistingSection(sections, existingSections, similarSection, newSection)
		} else {
			// No similar section found, add as new section
			fmt.Printf("[DEBUG] Adding new unique section: '%s'\n", newSection.Name)
			sections = append(sections, map[string]interface{}{
				"name":     newSection.Name,
				"priority": newSection.Priority,
				"content":  newSection.Content,
			})
			existingNames[newSection.Name] = true
			existingSections = append(existingSections, newSection)
		}
	}

	config["sections"] = sections
	return nil
}

// applyAgentsResponse applies agents response to config
func applyAgentsResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*AgentsResponse)
	if !ok {
		return fmt.Errorf("invalid agents response type")
	}
	agents := []interface{}{}
	existingNames := make(map[string]bool)

	// Keep existing agents and track their names
	if config["agents"] != nil {
		var ok bool
		agents, ok = config["agents"].([]interface{})
		if !ok {
			return fmt.Errorf("agents is not a slice")
		}
		for _, a := range agents {
			if agentMap, ok := a.(map[string]interface{}); ok {
				if name, ok := agentMap["name"].(string); ok {
					existingNames[name] = true
				}
			}
		}
	}

	// Add new agents only if they don't already exist
	for _, agent := range resp.Agents {
		if !existingNames[agent.Name] {
			agents = append(agents, map[string]interface{}{
				"name":      agent.Name,
				"role":      agent.Role,
				"expertise": agent.Expertise,
			})
		}
	}
	config["agents"] = agents
	return nil
}

// executeTasksInWaves executes agent tasks in batches/waves based on maxAgents limit
func executeTasksInWaves(tasks []AgentTask, taskStatuses []*TaskStatus, agent AgentInfo, context *ProjectContext, display *TaskDisplay, maxAgents int) (failedTasks []string, successCount int) {
	totalTasks := len(tasks)

	// Calculate number of waves needed (maximum 3 waves)
	numWaves := (totalTasks + maxAgents - 1) / maxAgents // Ceiling division
	const maxWaves = 3
	if numWaves > maxWaves {
		// Redistribute tasks to fit within 3 waves
		numWaves = maxWaves
		maxAgents = (totalTasks + numWaves - 1) / numWaves // Recalculate agents per wave
	}

	// Execute tasks in waves
	for wave := 0; wave < numWaves; wave++ {
		startIdx := wave * maxAgents
		endIdx := min(startIdx+maxAgents, totalTasks)

		// Display wave notice with proper formatting
		if totalTasks > 1 {
			fmt.Printf("\n🌊 Wave %d/%d: Running tasks %d-%d (out of %d)\n",
				wave+1, numWaves, startIdx+1, endIdx, totalTasks)
		}

		// Create a new TaskDisplay for this wave only
		waveDisplay := &TaskDisplay{
			tasks: taskStatuses[startIdx:endIdx],
		}

		// Initial display of wave tasks
		fmt.Println() // Add spacing
		waveDisplay.renderAllTasks()

		// Execute current wave with its own display
		waveFailedTasks, waveSuccessCount := executeWave(
			tasks[startIdx:endIdx],
			taskStatuses[startIdx:endIdx],
			agent,
			context,
			waveDisplay, // Pass wave-specific display
			startIdx,
		)

		failedTasks = append(failedTasks, waveFailedTasks...)
		successCount += waveSuccessCount
	}

	return failedTasks, successCount
}

// executeWave executes a single wave of agent tasks in parallel
func executeWave(waveTasks []AgentTask, waveStatuses []*TaskStatus, agent AgentInfo, context *ProjectContext, display *TaskDisplay, baseIndex int) (failedTasks []string, successCount int) {
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

			// Mark task as running when it starts
			waveStatuses[localIndex].mu.Lock()
			waveStatuses[localIndex].status = statusRunning
			waveStatuses[localIndex].attempt = 1
			waveStatuses[localIndex].startTime = time.Now()
			waveStatuses[localIndex].mu.Unlock()

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
