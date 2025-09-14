package enforcement

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/agents"
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

// Enforcer handles rule enforcement using AI agents
type Enforcer struct {
	config   *config.Config
	options  *EnforcementOptions
	agent    *agents.AgentInfo
	reviewer *Reviewer
}

// NewEnforcer creates a new rule enforcer
func NewEnforcer(cfg *config.Config, options *EnforcementOptions) (*Enforcer, error) {
	enforcer := &Enforcer{
		config:  cfg,
		options: options,
	}

	// Initialize agent if specified
	if options.Enforcement.AgentID != "" {
		agent, err := getAgentByID(options.Enforcement.AgentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get agent %s: %w", options.Enforcement.AgentID, err)
		}
		enforcer.agent = agent

		// Initialize reviewer if review is enabled
		if options.Enforcement.Review.Enabled {
			reviewAgent := agent
			if options.Enforcement.Review.ReviewAgent != "" {
				reviewAgent, err = getAgentByID(options.Enforcement.Review.ReviewAgent)
				if err != nil {
					return nil, fmt.Errorf("failed to get review agent %s: %w", options.Enforcement.Review.ReviewAgent, err)
				}
			}
			enforcer.reviewer = NewReviewer(reviewAgent, options.Enforcement.Review)
		}
	}

	return enforcer, nil
}

// Enforce runs rule enforcement on the configured project
func (e *Enforcer) Enforce(ctx context.Context) (*EnforcementResult, error) { //nolint:gocyclo
	startTime := time.Now()

	result := &EnforcementResult{
		ConfigPath:   e.options.ConfigFile,
		TotalRules:   len(e.config.Rules) + len(e.config.Sections),
		FilesScanned: 0,
		Violations:   []RuleViolation{},
		FixesApplied: 0,
		Duration:     0,
		Success:      true,
	}

	if e.agent != nil {
		result.AgentUsed = e.agent.Display
	}

	logger.Info("Starting rule enforcement", "total_rules", result.TotalRules)

	// Get files to scan
	files, err := e.getFilesToScan()
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to get files to scan: %v", err)
		return result, err
	}

	result.FilesScanned = len(files)
	logger.Info("Files to scan", "count", result.FilesScanned)

	// Enforce rules
	rulesEnforced := 0
	for _, rule := range e.config.Rules {
		if e.shouldEnforceRule(rule.Name) {
			violations, err := e.enforceRule(ctx, &rule, files)
			if err != nil {
				logger.Warn("Failed to enforce rule", "rule", rule.Name, "error", err)
				continue
			}
			result.Violations = append(result.Violations, violations...)
			rulesEnforced++
		}
	}

	// Enforce sections
	for _, section := range e.config.Sections {
		if e.shouldEnforceSection(section.Name) {
			violations, err := e.enforceSection(ctx, &section, files)
			if err != nil {
				logger.Warn("Failed to enforce section", "section", section.Name, "error", err)
				continue
			}
			result.Violations = append(result.Violations, violations...)
			rulesEnforced++
		}
	}

	result.EnforcedRules = rulesEnforced

	// Apply fixes if enabled and not in dry-run mode
	if e.options.Enforcement.AutoFix && !e.options.DryRun {
		fixesApplied, err := e.applyFixes(result.Violations)
		if err != nil {
			logger.Warn("Some fixes failed to apply", "error", err)
		}
		result.FixesApplied = fixesApplied
	}

	result.Duration = time.Since(startTime)

	// Check if enforcement passed
	errorViolations := 0
	for i := range result.Violations {
		if result.Violations[i].Severity == EnforcementError || result.Violations[i].Severity == EnforcementStrict {
			errorViolations++
		}
	}

	if errorViolations > 0 && e.options.Enforcement.MaxViolations >= 0 && errorViolations > e.options.Enforcement.MaxViolations {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Too many violations: %d (max allowed: %d)", errorViolations, e.options.Enforcement.MaxViolations)
	}

	logger.Info("Rule enforcement completed",
		"violations", len(result.Violations),
		"fixes_applied", result.FixesApplied,
		"duration", result.Duration,
		"success", result.Success)

	return result, nil
}

func (e *Enforcer) getFilesToScan() ([]string, error) {
	var files []string
	workingDir := e.options.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	// Default includes if none specified
	includes := e.options.Enforcement.IncludeFiles
	if len(includes) == 0 {
		includes = []string{"**/*.go", "**/*.py", "**/*.js", "**/*.ts", "**/*.jsx", "**/*.tsx", "**/*.java", "**/*.c", "**/*.cpp", "**/*.h", "**/*.hpp", "**/*.cs", "**/*.php", "**/*.rb", "**/*.rs", "**/*.kt", "**/*.swift", "**/*.scala", "**/*.sh", "**/*.yaml", "**/*.yml", "**/*.json", "**/*.xml", "**/*.md", "**/*.txt"}
	}

	excludes := e.options.Enforcement.ExcludeFiles
	// Add common excludes
	excludes = append(excludes, "**/node_modules/**", "**/vendor/**", "**/.git/**", "**/build/**", "**/dist/**", "**/target/**", "**/*.min.js", "**/*.min.css")

	for _, pattern := range includes {
		matches, err := filepath.Glob(filepath.Join(workingDir, pattern))
		if err != nil {
			continue
		}

		for _, match := range matches {
			if !e.shouldExcludeFile(match, excludes) {
				files = append(files, match)
			}
		}
	}

	return files, nil
}

func (e *Enforcer) shouldExcludeFile(filePath string, excludes []string) bool {
	for _, exclude := range excludes {
		if matched, _ := filepath.Match(exclude, filePath); matched {
			return true
		}
		if strings.Contains(filePath, exclude) {
			return true
		}
	}
	return false
}

func (e *Enforcer) shouldEnforceRule(ruleName string) bool {
	// Check if rule is in only_rules filter
	if len(e.options.OnlyRules) > 0 {
		for _, r := range e.options.OnlyRules {
			if r == ruleName {
				return true
			}
		}
		return false
	}

	// Check if rule is in exclude_rules filter
	for _, r := range e.options.ExcludeRules {
		if r == ruleName {
			return false
		}
	}

	return true
}

func (e *Enforcer) shouldEnforceSection(sectionName string) bool {
	// Check if section is in only_rules filter (sections count as rules)
	if len(e.options.OnlyRules) > 0 {
		for _, r := range e.options.OnlyRules {
			if r == sectionName {
				return true
			}
		}
		return false
	}

	// Check if section is in exclude_rules filter
	for _, r := range e.options.ExcludeRules {
		if r == sectionName {
			return false
		}
	}

	return true
}

func (e *Enforcer) enforceRule(ctx context.Context, rule *config.Rule, files []string) ([]RuleViolation, error) {
	if e.agent == nil {
		return nil, fmt.Errorf("no agent configured for enforcement")
	}

	var allViolations []RuleViolation

	analyzer := NewAnalyzer(e.agent, e.options.Enforcement.Timeout)

	for _, filePath := range files {
		violations, err := analyzer.AnalyzeFile(ctx, filePath, rule.Name, rule.Content)
		if err != nil {
			if e.options.Verbose {
				logger.Warn("Failed to analyze file", "file", filePath, "rule", rule.Name, "error", err)
			}
			continue
		}

		allViolations = append(allViolations, violations...)
	}

	return allViolations, nil
}

func (e *Enforcer) enforceSection(ctx context.Context, section *config.Section, files []string) ([]RuleViolation, error) {
	if e.agent == nil {
		return nil, fmt.Errorf("no agent configured for enforcement")
	}

	var allViolations []RuleViolation

	analyzer := NewAnalyzer(e.agent, e.options.Enforcement.Timeout)

	for _, filePath := range files {
		violations, err := analyzer.AnalyzeFile(ctx, filePath, section.Name, section.Content)
		if err != nil {
			if e.options.Verbose {
				logger.Warn("Failed to analyze file", "file", filePath, "section", section.Name, "error", err)
			}
			continue
		}

		allViolations = append(allViolations, violations...)
	}

	return allViolations, nil
}

func (e *Enforcer) applyFixes(violations []RuleViolation) (int, error) {
	fixesApplied := 0
	fixer := NewFixer()

	for i := range violations {
		if violations[i].Fix == nil {
			continue
		}

		err := fixer.ApplyFix(&violations[i])
		if err != nil {
			logger.Warn("Failed to apply fix", "file", violations[i].FilePath, "rule", violations[i].RuleName, "error", err)
			continue
		}

		fixesApplied++
		logger.Info("Applied fix", "file", violations[i].FilePath, "rule", violations[i].RuleName)
	}

	return fixesApplied, nil
}

// ReviewViolations performs iterative review with feedback loops for files with violations
func (e *Enforcer) ReviewViolations(ctx context.Context, violations []RuleViolation) ([]*ReviewSession, error) {
	if e.reviewer == nil {
		return nil, fmt.Errorf("no reviewer configured")
	}

	// Group violations by file
	fileViolations := make(map[string][]RuleViolation)
	for i := range violations {
		fileViolations[violations[i].FilePath] = append(fileViolations[violations[i].FilePath], violations[i])
	}

	var sessions []*ReviewSession
	for filePath, fileViols := range fileViolations {
		// Find the primary rule for this file (use the first rule mentioned)
		if len(fileViols) == 0 {
			continue
		}

		primaryRule := fileViols[0].RuleContent
		logger.Info("Starting review session", "file", filePath, "violations", len(fileViols))

		session, err := e.reviewer.ReviewWithFeedback(ctx, filePath, primaryRule, fileViols)
		if err != nil {
			logger.Warn("Review session failed", "file", filePath, "error", err)
			continue
		}

		sessions = append(sessions, session)

		// Brief pause between file reviews
		time.Sleep(2 * time.Second)
	}

	return sessions, nil
}

// Helper function to get agent by ID (reuse from agents package)
func getAgentByID(id string) (*agents.AgentInfo, error) {
	supportedAgents := []agents.AgentInfo{
		{ID: "amp", Command: "amp", Display: "AMP (Sourcegraph)"},
		{ID: "claude", Command: "claude", Display: "Claude (Anthropic)"},
		{ID: "codex", Command: "codex", Display: "Codex"},
		{ID: "continue-dev", Command: "cn", Display: "Continue.dev"},
		{ID: "cursor", Command: "cursor-agent", Display: "Cursor"},
		{ID: "gemini", Command: "gemini", Display: "Gemini (Google)"},
		{ID: "github-copilot", Command: "github-copilot", Display: "GitHub Copilot"},
		{ID: "windsurf", Command: "windsurf", Display: "Windsurf"},
	}

	id = strings.ToLower(strings.TrimSpace(id))
	for _, agent := range supportedAgents {
		if agent.ID == id {
			return &agent, nil
		}
	}
	return nil, fmt.Errorf("unknown agent: %s", id)
}
