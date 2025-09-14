package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/enforcement"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var EnforceCmd = &cobra.Command{
	Use:   "enforce",
	Short: "Enforce rules using AI agents",
	Long: `Enforce rules defined in your ai-rulez configuration using AI agents.

The enforce command analyzes your codebase against the rules and sections defined
in your configuration, detecting violations and optionally applying fixes.

Examples:
  # Check for violations (default behavior)
  ai-rulez enforce

  # Apply fixes automatically
  ai-rulez enforce --fix

  # Enforce only specific rules
  ai-rulez enforce --only-rules "coding-standards,security"

  # Use specific agent
  ai-rulez enforce --agent claude

  # Output results as JSON
  ai-rulez enforce --format json --output results.json`,

	RunE: enforceRun,
}

var (
	enforceAgent         string
	enforceLevel         string
	enforceAutoFix       bool
	enforceTimeout       time.Duration
	enforceOnlyRules     []string
	enforceExcludeRules  []string
	enforceIncludeFiles  []string
	enforceExcludeFiles  []string
	enforceMaxViolations int
	enforceOutputFormat  string
	enforceOutputFile    string
	enforceVerbose       bool
	// Review options
	enforceReview             bool
	enforceReviewAgent        string
	enforceReviewIterations   int
	enforceReviewThreshold    float64
	enforceReviewTimeout      time.Duration
	enforceReviewAutoApprove  bool
	enforceRequireImprovement bool
)

func init() {
	EnforceCmd.Flags().StringVar(&enforceAgent, "agent", "", "AI agent to use for enforcement (claude, gemini, amp, continue-dev)")
	EnforceCmd.Flags().StringVar(&enforceLevel, "level", "warn", "Enforcement level (off, warn, error, fix, strict)")
	EnforceCmd.Flags().BoolVar(&enforceAutoFix, "fix", false, "Automatically apply suggested fixes")
	EnforceCmd.Flags().DurationVar(&enforceTimeout, "timeout", 30*time.Second, "Timeout per file analysis")
	EnforceCmd.Flags().StringSliceVar(&enforceOnlyRules, "only-rules", nil, "Enforce only these rules (comma-separated)")
	EnforceCmd.Flags().StringSliceVar(&enforceExcludeRules, "exclude-rules", nil, "Exclude these rules from enforcement (comma-separated)")
	EnforceCmd.Flags().StringSliceVar(&enforceIncludeFiles, "include-files", nil, "Include only these file patterns (comma-separated)")
	EnforceCmd.Flags().StringSliceVar(&enforceExcludeFiles, "exclude-files", nil, "Exclude these file patterns (comma-separated)")
	EnforceCmd.Flags().IntVar(&enforceMaxViolations, "max-violations", -1, "Maximum allowed violations (-1 for unlimited)")
	EnforceCmd.Flags().StringVar(&enforceOutputFormat, "format", "table", "Output format (table, json, summary, csv)")
	EnforceCmd.Flags().StringVarP(&enforceOutputFile, "output", "o", "", "Output file (default: stdout)")
	EnforceCmd.Flags().BoolVar(&enforceVerbose, "verbose", false, "Enable verbose output")

	// Review flags
	EnforceCmd.Flags().BoolVar(&enforceReview, "review", false, "Enable iterative review with feedback loops")
	EnforceCmd.Flags().StringVar(&enforceReviewAgent, "review-agent", "", "Agent for code review (defaults to enforcement agent)")
	EnforceCmd.Flags().IntVar(&enforceReviewIterations, "review-iterations", 3, "Maximum review iterations")
	EnforceCmd.Flags().Float64Var(&enforceReviewThreshold, "review-threshold", 75.0, "Quality threshold for approval (0-100)")
	EnforceCmd.Flags().DurationVar(&enforceReviewTimeout, "review-timeout", 45*time.Second, "Timeout per review iteration")
	EnforceCmd.Flags().BoolVar(&enforceReviewAutoApprove, "review-auto-approve", false, "Auto-approve regardless of score")
	EnforceCmd.Flags().BoolVar(&enforceRequireImprovement, "require-improvement", false, "Require iterative improvement in review")

	EnforceCmd.MarkFlagsMutuallyExclusive("only-rules", "exclude-rules")
}

func enforceRun(cmd *cobra.Command, args []string) error { //nolint:gocyclo
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}

	// Load configuration
	cfg, configPath, err := loadEnforcementConfig(workingDir)
	if err != nil {
		return err
	}

	logger.Info("Starting rule enforcement", "config", configPath, "agent", enforceAgent)

	// Parse enforcement level
	var level enforcement.EnforcementLevel
	switch strings.ToLower(enforceLevel) {
	case "off":
		level = enforcement.EnforcementOff
	case "warn", "warning":
		level = enforcement.EnforcementWarn
	case "error":
		level = enforcement.EnforcementError
	case "fix":
		level = enforcement.EnforcementFix
	case "strict":
		level = enforcement.EnforcementStrict
	default:
		level = enforcement.EnforcementWarn
		logger.Warn("Unknown enforcement level, using 'warn'", "level", enforceLevel)
	}

	// If level is 'off', exit early
	if level == enforcement.EnforcementOff {
		logger.Info("Enforcement level is 'off', skipping enforcement")
		return nil
	}

	// Build enforcement options
	options := enforcement.EnforcementOptions{
		ConfigFile: configPath,
		WorkingDir: workingDir,
		Enforcement: enforcement.EnforcementConfig{
			Level:         level,
			AutoFix:       enforceAutoFix,
			AgentID:       enforceAgent,
			Timeout:       enforceTimeout,
			IncludeFiles:  enforceIncludeFiles,
			ExcludeFiles:  enforceExcludeFiles,
			MaxViolations: enforceMaxViolations,
			Review: enforcement.ReviewConfig{
				Enabled:            enforceReview,
				MaxIterations:      enforceReviewIterations,
				ReviewAgent:        enforceReviewAgent,
				FeedbackThreshold:  enforceReviewThreshold,
				ReviewTimeout:      enforceReviewTimeout,
				AutoApprove:        enforceReviewAutoApprove,
				RequireImprovement: enforceRequireImprovement,
			},
		},
		DryRun:       !enforceAutoFix, // Read-only by default, write only with --fix
		Verbose:      enforceVerbose,
		OnlyRules:    enforceOnlyRules,
		ExcludeRules: enforceExcludeRules,
		OutputFormat: enforceOutputFormat,
	}

	// Log mode based on --fix flag
	if enforceAutoFix {
		logger.Info("Fix mode enabled, violations will be automatically corrected")
	} else {
		logger.Info("Read-only mode, violations will be reported but not fixed")
	}

	// Create enforcer
	enforcer, err := enforcement.NewEnforcer(cfg, &options)
	if err != nil {
		return err
	}

	// Run enforcement
	ctx := context.Background()
	result, err := enforcer.Enforce(ctx)
	if err != nil {
		logger.LogError("Enforcement failed", err)
		return err
	}

	// Report results
	reporter, err := enforcement.NewReporter(enforceOutputFormat, enforceOutputFile)
	if err != nil {
		logger.LogError("Failed to create reporter", err)
		return err
	}
	defer reporter.Close()

	if err := reporter.Report(result); err != nil {
		logger.LogError("Failed to report results", err)
		return err
	}

	// Also output to console for visibility
	if enforceOutputFile != "" && enforceOutputFile != "-" {
		enforcement.ReportToConsole(result)
	}

	// Exit with error code if enforcement failed
	if !result.Success {
		//nolint:errcheck
		_ = reporter.Close() // Ensure cleanup before exit
		os.Exit(1)           //nolint:gocritic
	}

	return nil
}

func loadEnforcementConfig(workingDir string) (*config.Config, string, error) {
	// Find config file
	configPath, err := config.FindConfigFile(workingDir)
	if err != nil {
		return nil, "", err
	}

	// Load config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, "", err
	}

	return cfg, configPath, nil
}

// Add --enforce flag to the generate command
func addEnforceToGenerateCmd() {
	GenerateCmd.Flags().BoolVar(&enforceAfterGenerate, "enforce", false, "Run rule enforcement after generation")
	GenerateCmd.Flags().StringVar(&enforceAgentForGenerate, "enforce-agent", "", "AI agent to use for post-generation enforcement")
	GenerateCmd.Flags().StringVar(&enforceLevelForGenerate, "enforce-level", "warn", "Enforcement level for post-generation check")
}

var (
	enforceAfterGenerate    bool
	enforceAgentForGenerate string
	enforceLevelForGenerate string
)

// Function to run enforcement after generation
func runPostGenerateEnforcement(configPath string) error {
	if !enforceAfterGenerate {
		return nil
	}

	logger.Info("Running post-generation rule enforcement...")

	workingDir := filepath.Dir(configPath)

	// Load configuration
	cfg, _, err := loadEnforcementConfig(workingDir)
	if err != nil {
		logger.Warn("Failed to load config for enforcement", "error", err)
		return nil // Don't fail generation due to enforcement issues
	}

	// Parse enforcement level
	var level enforcement.EnforcementLevel
	switch strings.ToLower(enforceLevelForGenerate) {
	case "off":
		return nil // Skip enforcement
	case "warn", "warning":
		level = enforcement.EnforcementWarn
	case "error":
		level = enforcement.EnforcementError
	case "fix":
		level = enforcement.EnforcementFix
	case "strict":
		level = enforcement.EnforcementStrict
	default:
		level = enforcement.EnforcementWarn
	}

	// Build enforcement options (conservative defaults for post-generation)
	options := enforcement.EnforcementOptions{
		ConfigFile: configPath,
		WorkingDir: workingDir,
		Enforcement: enforcement.EnforcementConfig{
			Level:         level,
			AutoFix:       false, // Never auto-fix after generation
			AgentID:       enforceAgentForGenerate,
			Timeout:       15 * time.Second, // Shorter timeout for post-generation
			MaxViolations: -1,
		},
		DryRun:       true, // Always read-only for post-generation
		Verbose:      false,
		OutputFormat: "summary",
	}

	// Create enforcer
	enforcer, err := enforcement.NewEnforcer(cfg, &options)
	if err != nil {
		logger.Warn("Failed to create enforcer", "error", err)
		return nil
	}

	// Run enforcement
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := enforcer.Enforce(ctx)
	if err != nil {
		logger.Warn("Post-generation enforcement failed", "error", err)
		return nil
	}

	// Report results to console
	enforcement.ReportToConsole(result)

	return nil
}
