package enforcement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/agents"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

// ReviewResult represents the outcome of a code review
type ReviewResult struct {
	Iteration     int                `json:"iteration"`
	ReviewerAgent string             `json:"reviewer_agent"`
	Score         float64            `json:"score"` // 0-100 quality score
	Approved      bool               `json:"approved"`
	Issues        []ReviewIssue      `json:"issues"`
	Suggestions   []ReviewSuggestion `json:"suggestions"`
	Summary       string             `json:"summary"`
	Feedback      string             `json:"feedback"`
	Improvements  []string           `json:"improvements"`
	NextActions   []string           `json:"next_actions,omitempty"`
	Duration      time.Duration      `json:"duration"`
	Success       bool               `json:"success"`
	ErrorMessage  string             `json:"error_message,omitempty"`
}

// ReviewIssue represents a specific issue found during review
type ReviewIssue struct {
	Type        string `json:"type"`     // "style", "logic", "security", "performance", etc.
	Severity    string `json:"severity"` // "critical", "major", "minor", "suggestion"
	LineNumber  int    `json:"line_number,omitempty"`
	Description string `json:"description"`
	Context     string `json:"context,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// ReviewSuggestion represents an improvement suggestion
type ReviewSuggestion struct {
	Type        string `json:"type"`     // "refactor", "optimize", "enhance", etc.
	Priority    string `json:"priority"` // "high", "medium", "low"
	Description string `json:"description"`
	Benefits    string `json:"benefits,omitempty"`
	Effort      string `json:"effort,omitempty"` // "low", "medium", "high"
}

// ReviewSession manages a complete review with feedback loops
type ReviewSession struct {
	FilePath      string         `json:"file_path"`
	RuleName      string         `json:"rule_name"`
	OriginalCode  string         `json:"original_code"`
	CurrentCode   string         `json:"current_code"`
	Iterations    []ReviewResult `json:"iterations"`
	FinalScore    float64        `json:"final_score"`
	TotalDuration time.Duration  `json:"total_duration"`
	Approved      bool           `json:"approved"`
	Config        ReviewConfig   `json:"config"`
}

// Reviewer handles code review with feedback loops
type Reviewer struct {
	agent  *agents.AgentInfo
	config ReviewConfig
}

// NewReviewer creates a new code reviewer
func NewReviewer(agent *agents.AgentInfo, config ReviewConfig) *Reviewer {
	// Set defaults
	if config.MaxIterations == 0 {
		config.MaxIterations = 3
	}
	if config.FeedbackThreshold == 0 {
		config.FeedbackThreshold = 75.0 // 75% score threshold
	}
	if config.ReviewTimeout == 0 {
		config.ReviewTimeout = 45 * time.Second
	}

	return &Reviewer{
		agent:  agent,
		config: config,
	}
}

// ReviewWithFeedback performs iterative review with feedback loops
func (r *Reviewer) ReviewWithFeedback(ctx context.Context, filePath, ruleContent string, initialViolations []RuleViolation) (*ReviewSession, error) {
	if !r.config.Enabled {
		return nil, fmt.Errorf("reviewer is disabled")
	}

	startTime := time.Now()

	// Read initial code
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	session := &ReviewSession{
		FilePath:     filePath,
		RuleName:     ruleContent,
		OriginalCode: string(content),
		CurrentCode:  string(content),
		Iterations:   []ReviewResult{},
		Config:       r.config,
	}

	logger.Info("Starting review session",
		"file", filePath,
		"max_iterations", r.config.MaxIterations,
		"threshold", r.config.FeedbackThreshold)

	// Iterative review loop
	for iteration := 1; iteration <= r.config.MaxIterations; iteration++ {
		iterationStart := time.Now()

		logger.Debug("Review iteration", "iteration", iteration, "file", filePath)

		// Perform review
		result, err := r.performReview(ctx, session, iteration, initialViolations)
		if err != nil {
			result = &ReviewResult{
				Iteration:     iteration,
				ReviewerAgent: r.agent.Display,
				Success:       false,
				ErrorMessage:  err.Error(),
				Duration:      time.Since(iterationStart),
			}
		}

		session.Iterations = append(session.Iterations, *result)

		// Check if we should continue
		if r.shouldStopReview(session, result) {
			break
		}

		// Apply improvements if suggested
		if len(result.NextActions) > 0 {
			improved, err := r.applyImprovements(session, result)
			if err != nil {
				logger.Warn("Failed to apply improvements", "error", err, "iteration", iteration)
				break
			}
			if improved != nil {
				if improvedCode, ok := improved.(string); ok && improvedCode != session.CurrentCode {
					session.CurrentCode = improvedCode
				}
			}
		}

		// Brief pause between iterations
		time.Sleep(time.Second)
	}

	session.TotalDuration = time.Since(startTime)
	session.FinalScore = r.calculateFinalScore(session)
	session.Approved = session.FinalScore >= r.config.FeedbackThreshold || r.config.AutoApprove

	logger.Info("Review session completed",
		"file", filePath,
		"iterations", len(session.Iterations),
		"final_score", session.FinalScore,
		"approved", session.Approved,
		"duration", session.TotalDuration.Round(time.Millisecond))

	return session, nil
}

func (r *Reviewer) performReview(ctx context.Context, session *ReviewSession, iteration int, violations []RuleViolation) (*ReviewResult, error) {
	prompt := r.buildReviewPrompt(session, iteration, violations)

	// Set timeout for this iteration
	ctx, cancel := context.WithTimeout(ctx, r.config.ReviewTimeout)
	defer cancel()

	response, err := r.invokeReviewAgent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("review agent failed: %w", err)
	}

	return r.parseReviewResponse(response, iteration)
}

func (r *Reviewer) buildReviewPrompt(session *ReviewSession, iteration int, violations []RuleViolation) string {
	var prompt strings.Builder

	prompt.WriteString("TASK: Comprehensive code review with iterative feedback\n\n")

	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- File: %s\n", session.FilePath))
	prompt.WriteString(fmt.Sprintf("- Review iteration: %d/%d\n", iteration, r.config.MaxIterations))
	prompt.WriteString(fmt.Sprintf("- Quality threshold: %.1f%%\n", r.config.FeedbackThreshold))

	if len(violations) > 0 {
		prompt.WriteString(fmt.Sprintf("- Initial violations found: %d\n", len(violations)))
		prompt.WriteString("- Violations to address:\n")
		for i := range violations {
			if i >= 5 { // Limit to first 5 for brevity
				prompt.WriteString(fmt.Sprintf("  ... and %d more\n", len(violations)-5))
				break
			}
			v := &violations[i]
			prompt.WriteString(fmt.Sprintf("  • Line %d: %s\n", v.LineNumber, v.Message))
		}
	}

	if iteration > 1 {
		prompt.WriteString("\nPREVIOUS ITERATION FEEDBACK:\n")
		if len(session.Iterations) > 0 {
			lastResult := session.Iterations[len(session.Iterations)-1]
			prompt.WriteString(fmt.Sprintf("- Previous score: %.1f%%\n", lastResult.Score))
			prompt.WriteString(fmt.Sprintf("- Previous feedback: %s\n", lastResult.Feedback))
		}
	}

	prompt.WriteString("\nRULE TO ENFORCE:\n")
	prompt.WriteString(session.RuleName)
	prompt.WriteString("\n\n")

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Review the code thoroughly for:\n")
	prompt.WriteString("   - Rule compliance\n")
	prompt.WriteString("   - Code quality and style\n")
	prompt.WriteString("   - Security issues\n")
	prompt.WriteString("   - Performance concerns\n")
	prompt.WriteString("   - Best practices adherence\n")
	prompt.WriteString("2. Assign a quality score (0-100)\n")
	prompt.WriteString("3. Identify specific issues and provide actionable suggestions\n")
	prompt.WriteString("4. If score < threshold, provide concrete next actions for improvement\n")
	prompt.WriteString("5. Return results as JSON only\n\n")

	prompt.WriteString("REQUIRED JSON FORMAT:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"score\": <number 0-100>,\n")
	prompt.WriteString("  \"approved\": <boolean>,\n")
	prompt.WriteString("  \"summary\": \"<overall assessment>\",\n")
	prompt.WriteString("  \"feedback\": \"<detailed feedback>\",\n")
	prompt.WriteString("  \"issues\": [\n")
	prompt.WriteString("    {\n")
	prompt.WriteString("      \"type\": \"style|logic|security|performance|other\",\n")
	prompt.WriteString("      \"severity\": \"critical|major|minor|suggestion\",\n")
	prompt.WriteString("      \"line_number\": <number or null>,\n")
	prompt.WriteString("      \"description\": \"<issue description>\",\n")
	prompt.WriteString("      \"suggestion\": \"<how to fix>\"\n")
	prompt.WriteString("    }\n")
	prompt.WriteString("  ],\n")
	prompt.WriteString("  \"suggestions\": [\n")
	prompt.WriteString("    {\n")
	prompt.WriteString("      \"type\": \"refactor|optimize|enhance|other\",\n")
	prompt.WriteString("      \"priority\": \"high|medium|low\",\n")
	prompt.WriteString("      \"description\": \"<suggestion>\",\n")
	prompt.WriteString("      \"benefits\": \"<why this helps>\",\n")
	prompt.WriteString("      \"effort\": \"low|medium|high\"\n")
	prompt.WriteString("    }\n")
	prompt.WriteString("  ],\n")
	prompt.WriteString("  \"improvements\": [\"<list of improvements made since last iteration>\"],\n")
	prompt.WriteString("  \"next_actions\": [\"<specific actions for next iteration if score < threshold>\"]\n")
	prompt.WriteString("}\n\n")

	prompt.WriteString("CODE TO REVIEW:\n")
	lines := strings.Split(session.CurrentCode, "\n")
	for i, line := range lines {
		prompt.WriteString(fmt.Sprintf("%4d: %s\n", i+1, line))
	}

	return prompt.String()
}

func (r *Reviewer) invokeReviewAgent(ctx context.Context, prompt string) (string, error) {
	analyzer := NewAnalyzer(r.agent, r.config.ReviewTimeout)
	return analyzer.invokeAgent(ctx, prompt)
}

func (r *Reviewer) parseReviewResponse(response string, iteration int) (*ReviewResult, error) {
	// Clean response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json\n")
	response = strings.TrimPrefix(response, "```\n")
	response = strings.TrimSuffix(response, "\n```")
	response = strings.TrimSuffix(response, "```")

	// Find JSON content
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	jsonContent := response[startIdx : endIdx+1]

	// Parse JSON
	type ReviewResponse struct {
		Score        float64            `json:"score"`
		Approved     bool               `json:"approved"`
		Summary      string             `json:"summary"`
		Feedback     string             `json:"feedback"`
		Issues       []ReviewIssue      `json:"issues"`
		Suggestions  []ReviewSuggestion `json:"suggestions"`
		Improvements []string           `json:"improvements"`
		NextActions  []string           `json:"next_actions"`
	}

	var parsed ReviewResponse
	if err := json.Unmarshal([]byte(jsonContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse review response: %w", err)
	}

	// Convert to ReviewResult
	result := &ReviewResult{
		Iteration:     iteration,
		ReviewerAgent: r.agent.Display,
		Score:         parsed.Score,
		Approved:      parsed.Approved,
		Issues:        parsed.Issues,
		Suggestions:   parsed.Suggestions,
		Summary:       parsed.Summary,
		Feedback:      parsed.Feedback,
		Improvements:  parsed.Improvements,
		NextActions:   parsed.NextActions,
		Success:       true,
	}

	// Validate score range
	if result.Score < 0 {
		result.Score = 0
	} else if result.Score > 100 {
		result.Score = 100
	}

	return result, nil
}

func (r *Reviewer) shouldStopReview(session *ReviewSession, result *ReviewResult) bool {
	// Stop if error occurred
	if !result.Success {
		return true
	}

	// Stop if approved
	if result.Approved {
		return true
	}

	// Stop if score meets threshold
	if result.Score >= r.config.FeedbackThreshold {
		return true
	}

	// Stop if no next actions provided
	if len(result.NextActions) == 0 {
		return true
	}

	// Stop if require improvement is disabled
	if !r.config.RequireImprovement {
		return true
	}

	// Continue if we haven't reached max iterations
	return len(session.Iterations) >= r.config.MaxIterations
}

func (r *Reviewer) applyImprovements(session *ReviewSession, result *ReviewResult) (interface{}, error) {
	// For now, this is a placeholder - in a full implementation,
	// this would apply the suggested improvements automatically
	// or prompt for manual intervention

	logger.Debug("Suggested improvements for next iteration",
		"file", session.FilePath,
		"actions", result.NextActions)

	// Return current code unchanged for now
	// In practice, this could:
	// 1. Apply automatic fixes
	// 2. Call another agent to make improvements
	// 3. Prompt user for manual changes
	// 4. Use additional tools (formatters, linters, etc.)

	return session.CurrentCode, nil
}

func (r *Reviewer) calculateFinalScore(session *ReviewSession) float64 {
	if len(session.Iterations) == 0 {
		return 0.0
	}

	// Use the best score achieved across all iterations
	bestScore := 0.0
	for i := range session.Iterations {
		if session.Iterations[i].Score > bestScore {
			bestScore = session.Iterations[i].Score
		}
	}

	return bestScore
}

// ReviewSummary provides a summary of all review sessions
type ReviewSummary struct {
	TotalSessions     int            `json:"total_sessions"`
	ApprovedSessions  int            `json:"approved_sessions"`
	AverageScore      float64        `json:"average_score"`
	AverageIterations float64        `json:"average_iterations"`
	TotalDuration     time.Duration  `json:"total_duration"`
	TopIssues         map[string]int `json:"top_issues"`      // Issue type -> count
	TopSuggestions    map[string]int `json:"top_suggestions"` // Suggestion type -> count
}

// SummarizeReviewSessions creates a summary of multiple review sessions
func SummarizeReviewSessions(sessions []*ReviewSession) *ReviewSummary {
	if len(sessions) == 0 {
		return &ReviewSummary{}
	}

	summary := &ReviewSummary{
		TotalSessions:  len(sessions),
		TopIssues:      make(map[string]int),
		TopSuggestions: make(map[string]int),
	}

	totalScore := 0.0
	totalIterations := 0

	for _, session := range sessions {
		if session.Approved {
			summary.ApprovedSessions++
		}

		totalScore += session.FinalScore
		totalIterations += len(session.Iterations)
		summary.TotalDuration += session.TotalDuration

		// Count issue types
		for i := range session.Iterations {
			for _, issue := range session.Iterations[i].Issues {
				summary.TopIssues[issue.Type]++
			}
			for _, suggestion := range session.Iterations[i].Suggestions {
				summary.TopSuggestions[suggestion.Type]++
			}
		}
	}

	summary.AverageScore = totalScore / float64(len(sessions))
	summary.AverageIterations = float64(totalIterations) / float64(len(sessions))

	return summary
}
