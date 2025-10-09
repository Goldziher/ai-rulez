package agents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"gopkg.in/yaml.v3"
)

type ErrorSeverity int

const (
	ErrorSeverityMinor ErrorSeverity = iota
	ErrorSeverityMajor
	ErrorSeverityCritical
)

type AgentError struct {
	Err       error
	TaskName  string
	Severity  ErrorSeverity
	Retryable bool
}

func (ae *AgentError) Error() string {
	return fmt.Sprintf("task %s failed: %v", ae.TaskName, ae.Err)
}

func (ae *AgentError) Unwrap() error {
	return ae.Err
}

func classifyError(err error, taskName string) *AgentError {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "failed to write config") ||
		strings.Contains(err.Error(), "failed to create temporary file") ||
		strings.Contains(err.Error(), "failed to marshal config") {
		return &AgentError{
			Err:       err,
			TaskName:  taskName,
			Severity:  ErrorSeverityCritical,
			Retryable: true,
		}
	}

	if strings.Contains(err.Error(), "failed to invoke agent") ||
		strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "connection refused") {
		return &AgentError{
			Err:       err,
			TaskName:  taskName,
			Severity:  ErrorSeverityMajor,
			Retryable: true,
		}
	}

	if strings.Contains(err.Error(), "failed to parse agent output") ||
		strings.Contains(err.Error(), "invalid JSON") {
		return &AgentError{
			Err:       err,
			TaskName:  taskName,
			Severity:  ErrorSeverityMajor,
			Retryable: false,
		}
	}

	return &AgentError{
		Err:       err,
		TaskName:  taskName,
		Severity:  ErrorSeverityMinor,
		Retryable: true,
	}
}

type AgentTask struct {
	Name           string
	Description    string
	MaxRetries     int
	Prompt         func(context *ProjectContext) string
	FallbackPrompt func(context *ProjectContext) string
}

const (
	defaultMaxRetries = 3

	statusPending  = "pending"
	statusRunning  = "running"
	statusRetrying = "retrying"
	statusSuccess  = "success"
	statusFailed   = "failed"
)

func getInitAgentTasks(context *ProjectContext, providerConfig templates.ProviderConfig) []AgentTask {
	return distributeSpecialistTasks(context, providerConfig)
}

func getMaxAgents() int {
	if maxStr := os.Getenv("AI_RULEZ_MAX_AGENTS"); maxStr != "" {
		if maxVal, err := strconv.Atoi(maxStr); err == nil && maxVal > 0 && maxVal <= 10 {
			return maxVal
		}
	}
	return 5
}

func distributeSpecialistTasks(context *ProjectContext, providerConfig templates.ProviderConfig) []AgentTask {
	workload := assessProjectWorkload(context)

	baseTasks := []string{"project description", "coding standards", "documentation sections"}

	if providerConfig.Claude || providerConfig.ContinueDev {
		baseTasks = append(baseTasks, "agent definitions")
	}

	tasks := applySplittingHeuristics(baseTasks, workload, workload.suggestedAgents)

	return createGenericTasks(tasks, context)
}

type ProjectWorkload struct {
	complexity            int
	documentationHeavy    bool
	complexInfrastructure bool
	multiLanguage         bool
	largeCodebase         bool
	suggestedAgents       int
}

func assessProjectWorkload(context *ProjectContext) ProjectWorkload {
	workload := ProjectWorkload{}

	docCount := len(context.MarkdownFiles)
	workload.documentationHeavy = docCount > 10

	totalFiles := 0
	if context.DirectoryStructure != nil {
		for _, files := range context.DirectoryStructure {
			totalFiles += len(files)
		}
	}

	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		workload.complexInfrastructure = info.HasDocker || len(info.TechStack) > 5
		workload.multiLanguage = len(info.TechStack) > 3
		workload.largeCodebase = totalFiles > 100 || docCount > 15
	}

	isMonorepo := context.RepoType == repoTypeMonorepo || len(context.PackageLocations) > 1

	workload.complexity = 3
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

func applySplittingHeuristics(baseTasks []string, workload ProjectWorkload, maxAgents int) []string {
	var expandedTasks []string

	for _, task := range baseTasks {
		expanded := expandTask(task, workload, maxAgents)
		expandedTasks = append(expandedTasks, expanded...)
	}

	if len(expandedTasks) > maxAgents {
		return prioritizeTasks(expandedTasks, maxAgents)
	}

	return expandedTasks
}

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

func expandAgentDefinitions(workload ProjectWorkload, maxAgents int) []string {
	if workload.complexity < 6 || maxAgents < 4 {
		return []string{"agent definitions"}
	}

	return []string{
		"core agents",
		"specialized agents",
	}
}

func prioritizeTasks(tasks []string, maxAgents int) []string {
	prioritized := []string{"project description"}
	remaining := maxAgents - 1

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

func generateTaskID(taskName string, index int) string {
	name := strings.ToLower(taskName)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "#", "")
	return name
}

func selectPromptForTask(taskName string) func(*ProjectContext) string {
	switch {
	case strings.Contains(taskName, "project description"):
		return buildProjectAnalysisPrompt
	case strings.Contains(taskName, "documentation"):
		return buildDocumentationPrompt
	case strings.Contains(taskName, "standards"):
		return buildStandardsPrompt
	case strings.Contains(taskName, "agents"):
		return buildAgentDefinitionsPrompt
	default:
		return buildProjectAnalysisPrompt
	}
}

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

type TaskStatus struct {
	task       AgentTask
	status     string
	attempt    int
	mu         sync.RWMutex
	completed  bool
	startTime  time.Time
	duration   time.Duration
	lastError  error
	lineNumber int
}

type TaskDisplay struct {
	tasks        []*TaskStatus
	spinnerIndex int
	mu           sync.Mutex
	startTime    time.Time
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func ExecuteInitChain(agent AgentInfo, context *ProjectContext, providerConfig templates.ProviderConfig, presets []string) (string, error) {
	fmt.Printf("🔗 Starting parallel agent task execution...\n")

	if err := initializeBaseConfigFile(context, providerConfig, presets); err != nil {
		return "", fmt.Errorf("failed to initialize base config: %w", err)
	}
	fmt.Printf("✅ Initialized base ai-rulez.yaml\n")

	initAgentTasks := getInitAgentTasks(context, providerConfig)

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

	display := &TaskDisplay{
		tasks:        taskStatuses,
		spinnerIndex: 0,
		startTime:    startTime,
	}

	maxAgents := getMaxAgents()
	failedTasks, successCount, criticalErrors := executeTasksInWaves(initAgentTasks, taskStatuses, agent, context, display, maxAgents)

	if len(criticalErrors) > 0 {
		fmt.Printf("\n")
		fmt.Printf("❌ Critical failures occurred during agent execution:\n")
		for _, critErr := range criticalErrors {
			fmt.Printf("   • %s: %v\n", critErr.TaskName, critErr.Err)
		}
		return "", fmt.Errorf("critical failures prevent successful configuration generation: %d critical errors", len(criticalErrors))
	}

	fmt.Printf("\n")
	if len(failedTasks) > 0 {
		fmt.Printf("⚠️  Completed %d/%d tasks successfully. Failed: %v\n",
			successCount, len(initAgentTasks), failedTasks)
		fmt.Printf("💡 Partial success: The configuration has been created with available content\n")
		fmt.Printf("   You can manually add missing sections or re-run with better connectivity\n")
	} else {
		fmt.Println("✅ All agent tasks completed successfully")
	}

	fmt.Println("✅ Configuration generated successfully")

	data, err := os.ReadFile("ai-rulez.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to read configuration file: %w", err)
	}

	return string(data), nil
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	tmpFile, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(filename)+"-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	tmpPath := tmpFile.Name()

	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()    //nolint:errcheck // Cleanup operation, error not critical
			_ = os.Remove(tmpPath) //nolint:errcheck // Cleanup operation, error not critical
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}
	tmpFile = nil

	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // Error cleanup, original error more important
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, filename); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // Error cleanup, original error more important
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func initializeBaseConfigFile(context *ProjectContext, providerConfig templates.ProviderConfig, presets []string) error {
	content := buildInitialConfigTemplate(context, providerConfig, presets)
	if err := atomicWriteFile("ai-rulez.yaml", []byte(content), 0o644); err != nil {
		return err
	}

	// Format the generated YAML with yamlfmt if available
	formatConfigFile("ai-rulez.yaml")
	return nil
}

func buildInitialConfigTemplate(context *ProjectContext, providerConfig templates.ProviderConfig, presets []string) string {
	var sb strings.Builder

	sb.WriteString("$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json\n\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s\n", context.ProjectName)
	sb.WriteString("  version: \"1.0.0\"\n")
	sb.WriteString("  description: \"\" # Will be updated by project-agent\n\n")

	// Use presets if available, otherwise use individual provider outputs
	if len(presets) > 0 {
		sb.WriteString("presets:\n")
		for _, preset := range presets {
			fmt.Fprintf(&sb, "  - \"%s\"\n", preset)
		}
	} else {
		sb.WriteString("outputs:\n")

		// Ensure at least one output exists for schema compliance
		hasOutputs := false

		if providerConfig.Claude {
			sb.WriteString("  - path: CLAUDE.md\n")
			sb.WriteString("  - path: .claude/agents/\n")
			sb.WriteString("    type: agent\n")
			sb.WriteString("    naming_scheme: '{name}.md'\n")
			hasOutputs = true
		}
		if providerConfig.Cursor {
			sb.WriteString("  - path: .cursor/rules/\n")
			sb.WriteString("    type: rule\n")
			sb.WriteString("    naming_scheme: '{name}.md'\n")
			hasOutputs = true
		}
		if providerConfig.Windsurf {
			sb.WriteString("  - path: .windsurfrules\n")
			hasOutputs = true
		}
		if providerConfig.Copilot {
			sb.WriteString("  - path: .github/copilot-instructions.md\n")
			hasOutputs = true
		}
		if providerConfig.Gemini {
			sb.WriteString("  - path: GEMINI.md\n")
			hasOutputs = true
		}
		if providerConfig.Amp || providerConfig.Codex {
			sb.WriteString("  - path: AGENTS.md\n")
			hasOutputs = true
		}
		if providerConfig.Cline {
			sb.WriteString("  - path: .clinerules/\n")
			sb.WriteString("    type: rule\n")
			sb.WriteString("    naming_scheme: '{name}.md'\n")
			hasOutputs = true
		}
		if providerConfig.ContinueDev {
			sb.WriteString("  - path: .continue/rules/\n")
			sb.WriteString("    type: rule\n")
			sb.WriteString("    naming_scheme: '{name}.md'\n")
			sb.WriteString("  - path: .continue/prompts/ai_rulez_prompts.yaml\n")
			sb.WriteString("    template:\n")
			sb.WriteString("      type: builtin\n")
			sb.WriteString("      value: continue-prompts\n")
			hasOutputs = true
		}

		// Fallback to ensure at least one output for schema compliance
		if !hasOutputs {
			sb.WriteString("  - path: CLAUDE.md\n")
		}
	}

	sb.WriteString("\nsections: []\n")
	sb.WriteString("rules: []\n")
	sb.WriteString("agents: []\n")
	sb.WriteString("commands: []\n")
	sb.WriteString("mcp_servers:\n")
	sb.WriteString("  - name: \"ai-rulez\"\n")
	sb.WriteString("    command: \"npx\"\n")
	sb.WriteString("    args: [\"-y\", \"ai-rulez@latest\", \"mcp\"]\n")
	sb.WriteString("    description: \"AI-Rulez MCP server for configuration management\"\n")

	return sb.String()
}

func executeAgentTaskWithStatus(task AgentTask, agent AgentInfo, context *ProjectContext, status *TaskStatus) error {
	_ = context.UpdateContextFromConfig() //nolint:errcheck // Continue anyway - this is not critical

	var lastErr error
	startTime := time.Now()

	for attempt := 1; attempt <= task.MaxRetries; attempt++ {
		var prompt string
		if attempt > 1 && task.FallbackPrompt != nil {
			prompt = task.FallbackPrompt(context)
		} else {
			prompt = task.Prompt(context)
		}
		if attempt > 1 {
			status.mu.Lock()
			status.status = statusRetrying
			status.attempt = attempt
			status.mu.Unlock()

			delay := time.Duration(attempt-1) * 2 * time.Second
			time.Sleep(delay)
		}

		timeout := 120 * time.Second

		logger.Debug("Invoking agent", "task", task.Name, "attempt", attempt)
		output, err := invokeAgent(agent, prompt, timeout)

		if err == nil {
			logger.Debug("Agent output received", "task", task.Name, "outputLength", len(output))
			if output != "" {
				preview := output
				if len(preview) > 500 {
					preview = preview[:500] + "..."
				}
				logger.Debug("Agent output preview", "task", task.Name, "preview", preview)
			}
			if err := executeAgentCommands(output, task.Name); err != nil {
				lastErr = fmt.Errorf("failed to process agent response: %w", err)
				logger.Debug("Response processing failed", "task", task.Name, "error", err.Error())
				continue
			}

			status.mu.Lock()
			status.status = statusSuccess
			status.completed = true
			status.duration = time.Since(startTime)
			status.mu.Unlock()

			context.AddCompletedTask(task.Name)

			return nil
		}

		lastErr = err
	}

	status.mu.Lock()
	status.status = statusFailed
	status.completed = true
	status.lastError = lastErr
	status.duration = time.Since(startTime)
	status.mu.Unlock()

	return lastErr
}

func (d *TaskDisplay) renderAllTasks() {
	d.mu.Lock()
	defer d.mu.Unlock()

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

	for _, ts := range completedTasks {
		d.renderTaskLine(ts)
	}
	for _, ts := range runningTasks {
		d.renderTaskLine(ts)
	}
	for _, ts := range pendingTasks {
		d.renderTaskLine(ts)
	}
}

func (d *TaskDisplay) updateDisplay() {
	d.mu.Lock()
	d.spinnerIndex = (d.spinnerIndex + 1) % len(spinnerFrames)
	spinnerFrame := spinnerFrames[d.spinnerIndex]
	d.mu.Unlock()

	fmt.Printf("\033[s")

	for i := 0; i < len(d.tasks); i++ {
		fmt.Printf("\033[A")
	}

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

	allTasks := make([]*TaskStatus, 0, len(completedTasks)+len(runningTasks)+len(pendingTasks))
	allTasks = append(allTasks, completedTasks...)
	allTasks = append(allTasks, runningTasks...)
	allTasks = append(allTasks, pendingTasks...)

	for _, ts := range allTasks {
		ts.mu.RLock()
		fmt.Printf("\r\033[K")

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

	fmt.Printf("\033[u")
}

func (d *TaskDisplay) renderTaskLine(ts *TaskStatus) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	fmt.Printf("\r\033[K")

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

func getErrorSummary(err error) string {
	if err == nil {
		return "no error"
	}

	errStr := err.Error()

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

	if len(errStr) > 50 {
		return errStr[:47] + "..."
	}
	return errStr
}

func executeAgentCommands(output string, taskName string) error {
	if output == "" {
		return nil
	}

	responseType := getResponseTypeForTask(taskName)
	if responseType == "" {
		return fmt.Errorf("unknown task type: %s", taskName)
	}

	response, err := ParseAgentOutput(output, responseType)
	if err != nil {
		logger.Warn("Failed to parse agent output", "task", taskName, "error", err.Error())
		return fmt.Errorf("failed to parse agent output: %w", err)
	}

	if err := applyResponseToConfig(response, responseType); err != nil {
		logger.Warn("Failed to apply response", "task", taskName, "error", err.Error())
		return fmt.Errorf("failed to apply response: %w", err)
	}

	logger.Debug("Successfully processed response", "task", taskName)

	return nil
}

const (
	responseTypeMetadata = "metadata"
	responseTypeSections = "sections"
	responseTypeRules    = "rules"
	responseTypeAgents   = "agents"
)

func getResponseTypeForTask(taskName string) string {
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

func applyResponseToConfig(response interface{}, responseType string) error {
	configPath := "ai-rulez.yaml"

	var config map[string]interface{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		config = make(map[string]interface{})
	} else {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
		if config == nil {
			config = make(map[string]interface{})
		}
	}

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

	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := atomicWriteFile(configPath, output, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

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

func updateExistingRule(rules []interface{}, existingRules []RuleResponse, similarRule *RuleResponse, merged RuleResponse) {
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

		for j := range existingRules {
			if existingRules[j].Name == similarRule.Name {
				existingRules[j] = merged
				break
			}
		}

		break
	}
}

func applyRulesResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*RulesResponse)
	if !ok {
		return fmt.Errorf("invalid rules response type")
	}

	rules, existingRules, existingNames, err := extractExistingRules(config)
	if err != nil {
		return err
	}

	similarity := NewContentSimilarity(0.75)

	for _, newRule := range resp.Rules {
		if existingNames[newRule.Name] {
			continue
		}

		similarRule, _ := similarity.FindSimilarRule(newRule, existingRules)
		if similarRule != nil {
			merged := similarity.MergeRules(*similarRule, newRule)
			updateExistingRule(rules, existingRules, similarRule, merged)
		} else {
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

func updateExistingSection(sections []interface{}, existingSections []SectionResponse, similarSection *SectionResponse, newSection SectionResponse) {
	for i, s := range sections {
		sectionMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := sectionMap["name"].(string)
		if !ok || name != similarSection.Name {
			continue
		}

		mergedContent := similarSection.Content
		if len(newSection.Content) > len(similarSection.Content) {
			mergedContent = newSection.Content
		}

		sections[i] = map[string]interface{}{
			"name":     similarSection.Name,
			"priority": newSection.Priority,
			"content":  mergedContent,
		}

		for j := range existingSections {
			if existingSections[j].Name == similarSection.Name {
				existingSections[j].Content = mergedContent
				existingSections[j].Priority = newSection.Priority
				break
			}
		}

		break
	}
}

func applySectionsResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*SectionsResponse)
	if !ok {
		return fmt.Errorf("invalid sections response type")
	}

	sections, existingSections, existingNames, err := extractExistingSections(config)
	if err != nil {
		return err
	}

	similarity := NewContentSimilarity(0.65)

	for _, newSection := range resp.Sections {
		if existingNames[newSection.Name] {
			continue
		}

		similarSection, _ := similarity.FindSimilarSection(newSection, existingSections)
		if similarSection != nil {
			updateExistingSection(sections, existingSections, similarSection, newSection)
		} else {
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

func applyAgentsResponse(config map[string]interface{}, response interface{}) error {
	resp, ok := response.(*AgentsResponse)
	if !ok {
		return fmt.Errorf("invalid agents response type")
	}
	agents := []interface{}{}
	existingNames := make(map[string]bool)

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

	for _, agent := range resp.Agents {
		if !existingNames[agent.Name] {
			agents = append(agents, map[string]interface{}{
				"name":        agent.Name,
				"description": agent.Description,
			})
		}
	}
	config["agents"] = agents
	return nil
}

func executeTasksInWaves(tasks []AgentTask, taskStatuses []*TaskStatus, agent AgentInfo, context *ProjectContext, display *TaskDisplay, maxAgents int) (failedTasks []string, successCount int, criticalErrors []*AgentError) {
	totalTasks := len(tasks)

	numWaves := (totalTasks + maxAgents - 1) / maxAgents
	const maxWaves = 3
	if numWaves > maxWaves {
		numWaves = maxWaves
		maxAgents = (totalTasks + numWaves - 1) / numWaves
	}

	for wave := 0; wave < numWaves; wave++ {
		startIdx := wave * maxAgents
		endIdx := min(startIdx+maxAgents, totalTasks)

		if totalTasks > 1 {
			fmt.Printf("\n🌊 Wave %d/%d: Running tasks %d-%d (out of %d)\n",
				wave+1, numWaves, startIdx+1, endIdx, totalTasks)
		}

		waveDisplay := &TaskDisplay{
			tasks: taskStatuses[startIdx:endIdx],
		}

		fmt.Println()
		waveDisplay.renderAllTasks()

		waveFailedTasks, waveSuccessCount, waveCriticalErrors := executeWave(
			tasks[startIdx:endIdx],
			taskStatuses[startIdx:endIdx],
			agent,
			context,
			waveDisplay,
			startIdx,
		)

		failedTasks = append(failedTasks, waveFailedTasks...)
		successCount += waveSuccessCount
		criticalErrors = append(criticalErrors, waveCriticalErrors...)
	}

	return failedTasks, successCount, criticalErrors
}

func executeWave(waveTasks []AgentTask, waveStatuses []*TaskStatus, agent AgentInfo, context *ProjectContext, display *TaskDisplay, baseIndex int) (failedTasks []string, successCount int, criticalErrors []*AgentError) {
	type taskResult struct {
		taskIndex int
		err       error
	}

	results := make(chan taskResult, len(waveTasks))

	var wg sync.WaitGroup

	for i, task := range waveTasks {
		wg.Add(1)
		go func(localIndex int, t AgentTask) {
			defer wg.Done()

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

	statusDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				display.updateDisplay()
			case <-statusDone:
				display.renderAllTasks()
				return
			}
		}
	}()

	wg.Wait()
	close(statusDone)
	close(results)

	for result := range results {
		localIndex := result.taskIndex - baseIndex
		ts := waveStatuses[localIndex]
		ts.mu.Lock()
		if result.err != nil {
			agentErr := classifyError(result.err, ts.task.Name)

			ts.status = statusFailed
			ts.completed = true
			ts.lastError = result.err
			failedTasks = append(failedTasks, ts.task.Name)

			if agentErr.Severity == ErrorSeverityCritical {
				criticalErrors = append(criticalErrors, agentErr)
			}
		} else {
			ts.status = statusSuccess
			ts.completed = true
			ts.duration = time.Since(ts.startTime)
			successCount++
		}
		ts.mu.Unlock()
	}

	return failedTasks, successCount, criticalErrors
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
