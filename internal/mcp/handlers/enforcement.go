package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/enforcement"
	"github.com/mark3labs/mcp-go/mcp"
)

// EnforceRulesHandler handles rule enforcement requests via MCP
func EnforceRulesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract arguments from request
	arguments := request.Params.Arguments

	// Type assert arguments to the expected type
	argsMap, ok := arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments format"), nil
	}

	// Parse arguments
	opts, err := parseEnforcementOptions(argsMap)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	// Load configuration
	cfg, configPath, err := loadConfigForEnforcement(opts.ConfigFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to load config: %v", err)), nil
	}

	opts.ConfigFile = configPath

	// Create enforcer
	enforcer, err := enforcement.NewEnforcer(cfg, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create enforcer: %v", err)), nil
	}

	// Run enforcement
	result, err := enforcer.Enforce(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Enforcement failed: %v", err)), nil
	}

	// Run review if enabled and there are violations
	if opts.Enforcement.Review.Enabled && len(result.Violations) > 0 {
		sessions, err := enforcer.ReviewViolations(ctx, result.Violations)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Review failed: %v", err)), nil
		}

		// Add review sessions to result metadata
		if result.Metadata == nil {
			result.Metadata = make(map[string]interface{})
		}
		result.Metadata["review_sessions"] = sessions
		result.Metadata["review_summary"] = enforcement.SummarizeReviewSessions(sessions)
	}

	// Format output
	switch strings.ToLower(opts.OutputFormat) {
	case "json":
		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resultJSON)), nil

	default:
		// Return structured result for other formats
		return mcp.NewToolResultText(formatEnforcementSummary(result)), nil
	}
}

// CheckViolationsHandler handles violation checking (dry-run) requests via MCP
func CheckViolationsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract arguments from request
	arguments := request.Params.Arguments

	// Type assert arguments to the expected type
	argsMap, ok := arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments format"), nil
	}

	// Parse arguments (similar to enforce but force read-only)
	opts, err := parseEnforcementOptions(argsMap)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	// Force read-only mode (no fixes)
	opts.DryRun = true
	opts.Enforcement.AutoFix = false
	opts.Enforcement.Level = enforcement.EnforcementWarn // Safe level for checking

	// Load configuration
	cfg, configPath, err := loadConfigForEnforcement(opts.ConfigFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to load config: %v", err)), nil
	}

	opts.ConfigFile = configPath

	// Create enforcer
	enforcer, err := enforcement.NewEnforcer(cfg, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create enforcer: %v", err)), nil
	}

	// Run enforcement (read-only)
	result, err := enforcer.Enforce(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Violation check failed: %v", err)), nil
	}

	// Format output
	switch strings.ToLower(opts.OutputFormat) {
	case "json":
		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resultJSON)), nil

	default:
		return mcp.NewToolResultText(formatViolationSummary(result)), nil
	}
}

func parseEnforcementOptions(arguments map[string]interface{}) (*enforcement.EnforcementOptions, error) { //nolint:gocyclo
	opts := &enforcement.EnforcementOptions{
		Enforcement: enforcement.EnforcementConfig{
			Level:         enforcement.EnforcementWarn,
			Timeout:       30 * time.Second,
			MaxViolations: -1,
		},
		OutputFormat: "json",
	}

	// Parse config file
	if configFile, ok := arguments["config_file"].(string); ok && configFile != "" {
		opts.ConfigFile = configFile
	}

	// Parse agent
	if agent, ok := arguments["agent"].(string); ok && agent != "" {
		opts.Enforcement.AgentID = agent
	}

	// Parse enforcement level
	if level, ok := arguments["level"].(string); ok && level != "" {
		switch strings.ToLower(level) {
		case "off":
			opts.Enforcement.Level = enforcement.EnforcementOff
		case "warn", "warning":
			opts.Enforcement.Level = enforcement.EnforcementWarn
		case "error":
			opts.Enforcement.Level = enforcement.EnforcementError
		case "fix":
			opts.Enforcement.Level = enforcement.EnforcementFix
		case "strict":
			opts.Enforcement.Level = enforcement.EnforcementStrict
		}
	}

	// Parse boolean flags
	if autoFix, ok := arguments["auto_fix"].(bool); ok {
		opts.Enforcement.AutoFix = autoFix
	}
	// Set DryRun based on AutoFix - read-only by default
	if !opts.Enforcement.AutoFix {
		opts.DryRun = true // Read-only when no AutoFix
	} else {
		opts.DryRun = false // Allow fixes when AutoFix is true
	}

	// Parse timeout
	if timeout, ok := arguments["timeout"].(float64); ok && timeout > 0 {
		opts.Enforcement.Timeout = time.Duration(timeout) * time.Second
	}

	// Parse max violations
	if maxViolations, ok := arguments["max_violations"].(float64); ok {
		opts.Enforcement.MaxViolations = int(maxViolations)
	}

	// Parse arrays
	if onlyRules, ok := arguments["only_rules"].([]interface{}); ok {
		opts.OnlyRules = interfaceSliceToStringSlice(onlyRules)
	}
	if excludeRules, ok := arguments["exclude_rules"].([]interface{}); ok {
		opts.ExcludeRules = interfaceSliceToStringSlice(excludeRules)
	}
	if includeFiles, ok := arguments["include_files"].([]interface{}); ok {
		opts.Enforcement.IncludeFiles = interfaceSliceToStringSlice(includeFiles)
	}
	if excludeFiles, ok := arguments["exclude_files"].([]interface{}); ok {
		opts.Enforcement.ExcludeFiles = interfaceSliceToStringSlice(excludeFiles)
	}

	// Parse output format
	if format, ok := arguments["output_format"].(string); ok && format != "" {
		opts.OutputFormat = format
	}

	// Parse review options
	if enableReview, ok := arguments["enable_review"].(bool); ok {
		opts.Enforcement.Review.Enabled = enableReview
	}
	if reviewAgent, ok := arguments["review_agent"].(string); ok && reviewAgent != "" {
		opts.Enforcement.Review.ReviewAgent = reviewAgent
	}
	if reviewIterations, ok := arguments["review_iterations"].(float64); ok && reviewIterations > 0 {
		opts.Enforcement.Review.MaxIterations = int(reviewIterations)
	}
	if reviewThreshold, ok := arguments["review_threshold"].(float64); ok && reviewThreshold >= 0 && reviewThreshold <= 100 {
		opts.Enforcement.Review.FeedbackThreshold = reviewThreshold
	}
	if reviewTimeout, ok := arguments["review_timeout"].(float64); ok && reviewTimeout > 0 {
		opts.Enforcement.Review.ReviewTimeout = time.Duration(reviewTimeout) * time.Second
	}
	if reviewAutoApprove, ok := arguments["review_auto_approve"].(bool); ok {
		opts.Enforcement.Review.AutoApprove = reviewAutoApprove
	}
	if requireImprovement, ok := arguments["require_improvement"].(bool); ok {
		opts.Enforcement.Review.RequireImprovement = requireImprovement
	}

	return opts, nil
}

func loadConfigForEnforcement(configFile string) (*config.Config, string, error) {
	var configPath string
	if configFile != "" {
		configPath = configFile
	} else {
		// Default config file
		configPath = "ai-rulez.yaml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, "", err
	}

	return cfg, configPath, nil
}

func interfaceSliceToStringSlice(slice []interface{}) []string {
	var result []string
	for _, item := range slice {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

func formatEnforcementSummary(result *enforcement.EnforcementResult) string {
	var summary strings.Builder

	summary.WriteString("🔍 Enforcement Summary\n")
	summary.WriteString("======================\n\n")

	summary.WriteString(fmt.Sprintf("Configuration: %s\n", result.ConfigPath))
	summary.WriteString(fmt.Sprintf("Rules Enforced: %d/%d\n", result.EnforcedRules, result.TotalRules))
	summary.WriteString(fmt.Sprintf("Files Scanned: %d\n", result.FilesScanned))
	summary.WriteString(fmt.Sprintf("Total Violations: %d\n", len(result.Violations)))
	summary.WriteString(fmt.Sprintf("Fixes Applied: %d\n", result.FixesApplied))
	summary.WriteString(fmt.Sprintf("Duration: %s\n", result.Duration.Round(time.Millisecond)))

	if result.AgentUsed != "" {
		summary.WriteString(fmt.Sprintf("Agent Used: %s\n", result.AgentUsed))
	}

	statusIcon := "✅"
	if !result.Success {
		statusIcon = "❌"
	}
	summary.WriteString(fmt.Sprintf("Status: %s %s\n", statusIcon, map[bool]string{true: "PASSED", false: "FAILED"}[result.Success]))

	if result.ErrorMessage != "" {
		summary.WriteString(fmt.Sprintf("Error: %s\n", result.ErrorMessage))
	}

	if len(result.Violations) > 0 {
		summary.WriteString("\n📋 Violations by Severity:\n")
		severityCount := make(map[enforcement.EnforcementLevel]int)
		for i := range result.Violations {
			severityCount[result.Violations[i].Severity]++
		}

		for severity, count := range severityCount {
			emoji := map[enforcement.EnforcementLevel]string{
				enforcement.EnforcementStrict: "🚫",
				enforcement.EnforcementError:  "❌",
				enforcement.EnforcementWarn:   "⚠️",
			}[severity]
			summary.WriteString(fmt.Sprintf("  %s %s: %d\n", emoji, severity, count))
		}
	}

	// Add review summary if available
	if result.Metadata != nil {
		if reviewSummary, ok := result.Metadata["review_summary"].(*enforcement.ReviewSummary); ok {
			summary.WriteString("\n🔄 Review Summary:\n")
			summary.WriteString(fmt.Sprintf("  Sessions: %d\n", reviewSummary.TotalSessions))
			summary.WriteString(fmt.Sprintf("  Approved: %d\n", reviewSummary.ApprovedSessions))
			summary.WriteString(fmt.Sprintf("  Average Score: %.1f%%\n", reviewSummary.AverageScore))
			summary.WriteString(fmt.Sprintf("  Average Iterations: %.1f\n", reviewSummary.AverageIterations))
		}
	}

	return summary.String()
}

func formatViolationSummary(result *enforcement.EnforcementResult) string {
	var summary strings.Builder

	summary.WriteString("🔍 Violation Check Summary\n")
	summary.WriteString("==========================\n\n")

	summary.WriteString(fmt.Sprintf("Files Scanned: %d\n", result.FilesScanned))
	summary.WriteString(fmt.Sprintf("Rules Checked: %d\n", result.EnforcedRules))
	summary.WriteString(fmt.Sprintf("Total Violations: %d\n", len(result.Violations)))
	summary.WriteString(fmt.Sprintf("Duration: %s\n", result.Duration.Round(time.Millisecond)))

	if len(result.Violations) == 0 {
		summary.WriteString("\n✅ No violations found!\n")
	} else {
		summary.WriteString("\n📋 Violations by File:\n")
		fileViolations := make(map[string][]enforcement.RuleViolation)
		for i := range result.Violations {
			fileViolations[result.Violations[i].FilePath] = append(fileViolations[result.Violations[i].FilePath], result.Violations[i])
		}

		for filePath, violations := range fileViolations {
			summary.WriteString(fmt.Sprintf("\n  📄 %s (%d violations):\n", filePath, len(violations)))
			for j := range violations {
				v := &violations[j]
				location := ""
				if v.LineNumber > 0 {
					location = fmt.Sprintf(":%d", v.LineNumber)
				}
				summary.WriteString(fmt.Sprintf("    - Line%s [%s]: %s\n", location, v.RuleName, v.Message))
			}
		}
	}

	return summary.String()
}

// Add EnforcementResult metadata field if it doesn't exist
type EnforcementResultWithMetadata struct {
	*enforcement.EnforcementResult
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
