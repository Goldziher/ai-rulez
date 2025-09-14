package enforcement

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// Reporter handles enforcement result reporting
type Reporter struct {
	format string
	writer *os.File
}

// NewReporter creates a new result reporter
func NewReporter(format string, outputPath string) (*Reporter, error) {
	var writer = os.Stdout

	if outputPath != "" && outputPath != "-" {
		var err error
		writer, err = os.Create(outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file %s: %w", outputPath, err)
		}
	}

	return &Reporter{
		format: format,
		writer: writer,
	}, nil
}

// Report outputs the enforcement results in the specified format
func (r *Reporter) Report(result *EnforcementResult) error {
	switch strings.ToLower(r.format) {
	case "json":
		return r.reportJSON(result)
	case "table", "text", "":
		return r.reportTable(result)
	case "summary":
		return r.reportSummary(result)
	case "csv":
		return r.reportCSV(result)
	default:
		return fmt.Errorf("unsupported output format: %s", r.format)
	}
}

// Close closes the output writer if it's a file
func (r *Reporter) Close() error {
	if r.writer != nil && r.writer != os.Stdout && r.writer != os.Stderr {
		return r.writer.Close()
	}
	return nil
}

func (r *Reporter) reportJSON(result *EnforcementResult) error {
	encoder := json.NewEncoder(r.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func (r *Reporter) reportTable(result *EnforcementResult) error {
	w := tabwriter.NewWriter(r.writer, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }() //nolint:errcheck

	// Header
	_, _ = fmt.Fprintf(w, "AI-Rulez Enforcement Report\n")  //nolint:errcheck
	_, _ = fmt.Fprintf(w, "==========================\n\n") //nolint:errcheck

	// Summary
	_, _ = fmt.Fprintf(w, "Configuration:\t%s\n", result.ConfigPath)                           //nolint:errcheck
	_, _ = fmt.Fprintf(w, "Rules Enforced:\t%d/%d\n", result.EnforcedRules, result.TotalRules) //nolint:errcheck
	_, _ = fmt.Fprintf(w, "Files Scanned:\t%d\n", result.FilesScanned)                         //nolint:errcheck
	_, _ = fmt.Fprintf(w, "Total Violations:\t%d\n", len(result.Violations))                   //nolint:errcheck
	_, _ = fmt.Fprintf(w, "Fixes Applied:\t%d\n", result.FixesApplied)                         //nolint:errcheck
	_, _ = fmt.Fprintf(w, "Duration:\t%s\n", result.Duration.Round(time.Millisecond))          //nolint:errcheck

	if result.AgentUsed != "" {
		_, _ = fmt.Fprintf(w, "Agent Used:\t%s\n", result.AgentUsed) //nolint:errcheck
	}

	_, _ = fmt.Fprintf(w, "Status:\t%s\n", r.getStatusString(result)) //nolint:errcheck

	if result.ErrorMessage != "" {
		_, _ = fmt.Fprintf(w, "Error:\t%s\n", result.ErrorMessage) //nolint:errcheck
	}

	_, _ = fmt.Fprintf(w, "\n") //nolint:errcheck

	if len(result.Violations) == 0 {
		_, _ = fmt.Fprintf(w, "✅ No violations found!\n") //nolint:errcheck
		return nil
	}

	// Group violations by severity
	severityGroups := r.groupViolationsBySeverity(result.Violations)

	// Sort severities
	severityOrder := []EnforcementLevel{EnforcementStrict, EnforcementError, EnforcementWarn}

	for _, severity := range severityOrder {
		violations, exists := severityGroups[severity]
		if !exists || len(violations) == 0 {
			continue
		}

		_, _ = fmt.Fprintf(w, "%s Violations (%d):\n", r.getSeverityEmoji(severity), len(violations)) //nolint:errcheck
		_, _ = fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))                                        //nolint:errcheck

		// Sort by file, then by line number
		sort.Slice(violations, func(i, j int) bool {
			if violations[i].FilePath != violations[j].FilePath {
				return violations[i].FilePath < violations[j].FilePath
			}
			return violations[i].LineNumber < violations[j].LineNumber
		})

		for i := range violations {
			v := &violations[i]
			location := v.FilePath
			if v.LineNumber > 0 {
				location += fmt.Sprintf(":%d", v.LineNumber)
				if v.ColumnNumber > 0 {
					location += fmt.Sprintf(":%d", v.ColumnNumber)
				}
			}

			_, _ = fmt.Fprintf(w, "%s\t[%s] %s\n", location, v.RuleName, v.Message) //nolint:errcheck

			if v.Context != "" {
				_, _ = fmt.Fprintf(w, "\tContext: %s\n", strings.ReplaceAll(v.Context, "\n", "\\n")) //nolint:errcheck
			}

			if v.Suggestion != "" {
				_, _ = fmt.Fprintf(w, "\tSuggestion: %s\n", v.Suggestion) //nolint:errcheck
			}

			if v.Fix != nil && v.Fix.Description != "" {
				_, _ = fmt.Fprintf(w, "\tFix: %s\n", v.Fix.Description) //nolint:errcheck
			}

			_, _ = fmt.Fprintf(w, "\n") //nolint:errcheck
		}

		_, _ = fmt.Fprintf(w, "\n") //nolint:errcheck
	}

	return nil
}

func (r *Reporter) reportSummary(result *EnforcementResult) error {
	_, _ = fmt.Fprintf(r.writer, "Enforcement Summary\n")  //nolint:errcheck
	_, _ = fmt.Fprintf(r.writer, "==================\n\n") //nolint:errcheck

	_, _ = fmt.Fprintf(r.writer, "Files scanned: %d\n", result.FilesScanned)                         //nolint:errcheck
	_, _ = fmt.Fprintf(r.writer, "Rules enforced: %d/%d\n", result.EnforcedRules, result.TotalRules) //nolint:errcheck
	_, _ = fmt.Fprintf(r.writer, "Total violations: %d\n", len(result.Violations))                   //nolint:errcheck

	if len(result.Violations) > 0 {
		severityGroups := r.groupViolationsBySeverity(result.Violations)

		for severity, violations := range severityGroups {
			_, _ = fmt.Fprintf(r.writer, "  %s: %d\n", severity, len(violations)) //nolint:errcheck
		}
	}

	_, _ = fmt.Fprintf(r.writer, "Fixes applied: %d\n", result.FixesApplied)                //nolint:errcheck
	_, _ = fmt.Fprintf(r.writer, "Duration: %s\n", result.Duration.Round(time.Millisecond)) //nolint:errcheck
	_, _ = fmt.Fprintf(r.writer, "Status: %s\n", r.getStatusString(result))                 //nolint:errcheck

	return nil
}

func (r *Reporter) reportCSV(result *EnforcementResult) error {
	_, _ = fmt.Fprintf(r.writer, "file_path,line_number,column_number,rule_name,severity,message,context,suggestion\n") //nolint:errcheck

	for i := range result.Violations {
		v := &result.Violations[i]
		context := strings.ReplaceAll(v.Context, "\"", "\"\"")
		context = strings.ReplaceAll(context, "\n", "\\n")

		suggestion := strings.ReplaceAll(v.Suggestion, "\"", "\"\"")
		suggestion = strings.ReplaceAll(suggestion, "\n", "\\n")

		message := strings.ReplaceAll(v.Message, "\"", "\"\"")
		message = strings.ReplaceAll(message, "\n", "\\n")

		_, _ = fmt.Fprintf(r.writer, "\"%s\",%d,%d,\"%s\",\"%s\",\"%s\",\"%s\",\"%s\"\n", //nolint:errcheck
			v.FilePath,
			v.LineNumber,
			v.ColumnNumber,
			v.RuleName,
			v.Severity,
			message,
			context,
			suggestion)
	}

	return nil
}

func (r *Reporter) groupViolationsBySeverity(violations []RuleViolation) map[EnforcementLevel][]RuleViolation {
	groups := make(map[EnforcementLevel][]RuleViolation)

	for i := range violations {
		groups[violations[i].Severity] = append(groups[violations[i].Severity], violations[i])
	}

	return groups
}

func (r *Reporter) getSeverityEmoji(severity EnforcementLevel) string {
	switch severity {
	case EnforcementStrict:
		return "🚫 STRICT"
	case EnforcementError:
		return "❌ ERROR"
	case EnforcementWarn:
		return "⚠️  WARNING"
	default:
		return "ℹ️  INFO"
	}
}

func (r *Reporter) getStatusString(result *EnforcementResult) string {
	if result.Success {
		return "✅ PASSED"
	}
	return "❌ FAILED"
}

// ReportToConsole provides a quick console output for enforcement results
func ReportToConsole(result *EnforcementResult) {
	if result.Success {
		logger.Info("🎉 Enforcement completed successfully!",
			"violations", len(result.Violations),
			"fixes_applied", result.FixesApplied,
			"duration", result.Duration.Round(time.Millisecond))
	} else {
		logger.Error("💥 Enforcement failed!",
			"violations", len(result.Violations),
			"error", result.ErrorMessage,
			"duration", result.Duration.Round(time.Millisecond))
	}

	if len(result.Violations) > 0 {
		severityGroups := make(map[EnforcementLevel]int)
		for i := range result.Violations {
			severityGroups[result.Violations[i].Severity]++
		}

		for severity, count := range severityGroups {
			switch severity {
			case EnforcementStrict:
				logger.Error(fmt.Sprintf("  🚫 %d strict violations", count))
			case EnforcementError:
				logger.Error(fmt.Sprintf("  ❌ %d error violations", count))
			case EnforcementWarn:
				logger.Warn(fmt.Sprintf("  ⚠️ %d warning violations", count))
			}
		}
	}
}
