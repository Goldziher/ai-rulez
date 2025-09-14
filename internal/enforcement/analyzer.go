package enforcement

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/agents"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

// Analyzer analyzes files for rule violations using AI agents
type Analyzer struct {
	agent   *agents.AgentInfo
	timeout time.Duration
}

// NewAnalyzer creates a new file analyzer
func NewAnalyzer(agent *agents.AgentInfo, timeout time.Duration) *Analyzer {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Analyzer{
		agent:   agent,
		timeout: timeout,
	}
}

// AnalyzeFile analyzes a file for violations of a specific rule
func (a *Analyzer) AnalyzeFile(ctx context.Context, filePath, ruleName, ruleContent string) ([]RuleViolation, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Skip empty files or binary files
	if len(content) == 0 || isBinaryFile(content) {
		return nil, nil
	}

	// Build analysis prompt
	prompt := a.buildAnalysisPrompt(filePath, string(content), ruleName, ruleContent)

	// Invoke agent with timeout context
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	result, err := a.invokeAgent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("agent analysis failed: %w", err)
	}

	// Parse agent response
	violations, err := a.parseAgentResponse(result, filePath, ruleName, ruleContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}

	return violations, nil
}

func (a *Analyzer) buildAnalysisPrompt(filePath, fileContent, ruleName, ruleContent string) string {
	var prompt strings.Builder

	prompt.WriteString("TASK: Analyze the provided file for violations of a specific rule.\n\n")

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the rule carefully and understand what it requires/prohibits\n")
	prompt.WriteString("2. Analyze the file content line by line\n")
	prompt.WriteString("3. Identify any violations of the rule\n")
	prompt.WriteString("4. For each violation, provide:\n")
	prompt.WriteString("   - Exact line number (1-based)\n")
	prompt.WriteString("   - Column number if applicable\n")
	prompt.WriteString("   - Clear description of the violation\n")
	prompt.WriteString("   - Context (the problematic code snippet)\n")
	prompt.WriteString("   - Suggested fix if possible\n")
	prompt.WriteString("5. Output your findings as JSON only\n\n")

	prompt.WriteString("OUTPUT FORMAT:\n")
	prompt.WriteString("Return a JSON array of violations. Each violation should have:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"line_number\": <number>,\n")
	prompt.WriteString("  \"column_number\": <number or null>,\n")
	prompt.WriteString("  \"severity\": \"warn\" | \"error\",\n")
	prompt.WriteString("  \"message\": \"<description of violation>\",\n")
	prompt.WriteString("  \"context\": \"<problematic code snippet>\",\n")
	prompt.WriteString("  \"suggestion\": \"<suggested fix>\",\n")
	prompt.WriteString("  \"fix\": {\n")
	prompt.WriteString("    \"description\": \"<fix description>\",\n")
	prompt.WriteString("    \"old_text\": \"<text to replace>\",\n")
	prompt.WriteString("    \"new_text\": \"<replacement text>\",\n")
	prompt.WriteString("    \"start_line\": <number>,\n")
	prompt.WriteString("    \"end_line\": <number>\n")
	prompt.WriteString("  }\n")
	prompt.WriteString("}\n\n")

	prompt.WriteString("If no violations are found, return an empty array: []\n")
	prompt.WriteString("Return ONLY the JSON, no explanations or markdown.\n\n")

	prompt.WriteString(fmt.Sprintf("RULE TO ENFORCE:\nName: %s\nContent: %s\n\n", ruleName, ruleContent))

	prompt.WriteString(fmt.Sprintf("FILE TO ANALYZE:\nPath: %s\nContent:\n", filePath))

	// Add line numbers for easier reference
	lines := strings.Split(fileContent, "\n")
	for i, line := range lines {
		prompt.WriteString(fmt.Sprintf("%4d: %s\n", i+1, line))
	}

	return prompt.String()
}

func (a *Analyzer) invokeAgent(ctx context.Context, prompt string) (string, error) {
	var cmd *exec.Cmd

	switch a.agent.ID {
	case "claude":
		cmd = exec.CommandContext(ctx, a.agent.Command, "--print", "--permission-mode", "bypassPermissions", prompt) //nolint:gosec
	case "amp":
		cmd = exec.CommandContext(ctx, a.agent.Command, "--execute", prompt) //nolint:gosec
	case "continue-dev":
		cmd = exec.CommandContext(ctx, a.agent.Command, "--print", prompt) //nolint:gosec
	case "gemini":
		cmd = exec.CommandContext(ctx, a.agent.Command, "--prompt", prompt) //nolint:gosec
	default:
		cmd = exec.CommandContext(ctx, a.agent.Command, prompt) //nolint:gosec
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start agent: %w", err)
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("failed to read stdout: %w", err)
	}

	stderrOutput, err := io.ReadAll(stderr)
	if err != nil {
		logger.Warn("Failed to read stderr", "error", err.Error())
	} else if len(stderrOutput) > 0 {
		logger.Warn("Agent stderr", "output", string(stderrOutput))
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("agent timed out after %v", a.timeout)
		}
		return "", fmt.Errorf("agent failed: %w", err)
	}

	return string(output), nil
}

func (a *Analyzer) parseAgentResponse(response, filePath, ruleName, ruleContent string) ([]RuleViolation, error) {
	// Clean response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json\n")
	response = strings.TrimPrefix(response, "```\n")
	response = strings.TrimSuffix(response, "\n```")
	response = strings.TrimSuffix(response, "```")

	// Try to find JSON in the response
	startIdx := strings.Index(response, "[")
	endIdx := strings.LastIndex(response, "]")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		// No JSON array found, treat as no violations
		return nil, nil
	}

	jsonContent := response[startIdx : endIdx+1]

	// Parse JSON
	type ViolationResponse struct {
		LineNumber   int    `json:"line_number"`
		ColumnNumber *int   `json:"column_number"`
		Severity     string `json:"severity"`
		Message      string `json:"message"`
		Context      string `json:"context"`
		Suggestion   string `json:"suggestion"`
		Fix          *struct {
			Description string `json:"description"`
			OldText     string `json:"old_text"`
			NewText     string `json:"new_text"`
			StartLine   int    `json:"start_line"`
			EndLine     int    `json:"end_line"`
		} `json:"fix"`
	}

	var rawViolations []ViolationResponse
	if err := json.Unmarshal([]byte(jsonContent), &rawViolations); err != nil {
		logger.Debug("Failed to parse agent response as JSON", "response", jsonContent, "error", err)
		return nil, nil // No violations rather than error
	}

	// Convert to RuleViolation structs
	var violations []RuleViolation
	for _, raw := range rawViolations {
		violation := RuleViolation{
			RuleName:    ruleName,
			RuleContent: ruleContent,
			FilePath:    filePath,
			LineNumber:  raw.LineNumber,
			Message:     raw.Message,
			Context:     raw.Context,
			Suggestion:  raw.Suggestion,
			Metadata:    make(map[string]string),
		}

		if raw.ColumnNumber != nil {
			violation.ColumnNumber = *raw.ColumnNumber
		}

		// Parse severity
		switch strings.ToLower(raw.Severity) {
		case "warn", "warning":
			violation.Severity = EnforcementWarn
		case "error":
			violation.Severity = EnforcementError
		default:
			violation.Severity = EnforcementWarn
		}

		// Add fix if provided
		if raw.Fix != nil {
			violation.Fix = &ViolationFix{
				Description: raw.Fix.Description,
				OldText:     raw.Fix.OldText,
				NewText:     raw.Fix.NewText,
				StartLine:   raw.Fix.StartLine,
				EndLine:     raw.Fix.EndLine,
			}
		}

		// Add metadata
		violation.Metadata["file_ext"] = filepath.Ext(filePath)
		violation.Metadata["agent"] = a.agent.ID

		violations = append(violations, violation)
	}

	return violations, nil
}

// isBinaryFile checks if content appears to be binary
func isBinaryFile(content []byte) bool {
	if len(content) == 0 {
		return false
	}

	// Check for null bytes (common indicator of binary content)
	for i := 0; i < len(content) && i < 512; i++ {
		if content[i] == 0 {
			return true
		}
	}

	return false
}
