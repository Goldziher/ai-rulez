package enforcement

import (
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// EnforcementLevel defines the severity level of rule enforcement
type EnforcementLevel string

const (
	EnforcementOff    EnforcementLevel = "off"
	EnforcementWarn   EnforcementLevel = "warn"
	EnforcementError  EnforcementLevel = "error"
	EnforcementFix    EnforcementLevel = "fix"
	EnforcementStrict EnforcementLevel = "strict"
)

// ReviewConfig defines review and feedback loop settings
type ReviewConfig struct {
	Enabled            bool          `yaml:"enabled,omitempty"`
	MaxIterations      int           `yaml:"max_iterations,omitempty"`
	ReviewAgent        string        `yaml:"review_agent,omitempty"`
	FeedbackThreshold  float64       `yaml:"feedback_threshold,omitempty"`
	ReviewTimeout      time.Duration `yaml:"review_timeout,omitempty"`
	AutoApprove        bool          `yaml:"auto_approve,omitempty"`
	RequireImprovement bool          `yaml:"require_improvement,omitempty"`
}

// EnforcementConfig defines enforcement settings
type EnforcementConfig struct {
	Level         EnforcementLevel `yaml:"level,omitempty"`
	AutoFix       bool             `yaml:"auto_fix,omitempty"`
	AgentID       string           `yaml:"agent_id,omitempty"`
	Timeout       time.Duration    `yaml:"timeout,omitempty"`
	ExcludeFiles  []string         `yaml:"exclude_files,omitempty"`
	IncludeFiles  []string         `yaml:"include_files,omitempty"`
	MaxViolations int              `yaml:"max_violations,omitempty"`
	Review        ReviewConfig     `yaml:"review,omitempty"`
}

// RuleViolation represents a detected rule violation
type RuleViolation struct {
	RuleName     string            `json:"rule_name"`
	RuleContent  string            `json:"rule_content"`
	FilePath     string            `json:"file_path"`
	LineNumber   int               `json:"line_number,omitempty"`
	ColumnNumber int               `json:"column_number,omitempty"`
	Severity     EnforcementLevel  `json:"severity"`
	Message      string            `json:"message"`
	Context      string            `json:"context,omitempty"`
	Suggestion   string            `json:"suggestion,omitempty"`
	Fix          *ViolationFix     `json:"fix,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ViolationFix represents a suggested fix for a violation
type ViolationFix struct {
	Description string `json:"description"`
	OldText     string `json:"old_text,omitempty"`
	NewText     string `json:"new_text,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	EndLine     int    `json:"end_line,omitempty"`
	StartColumn int    `json:"start_column,omitempty"`
	EndColumn   int    `json:"end_column,omitempty"`
}

// EnforcementResult contains the results of rule enforcement
type EnforcementResult struct {
	ConfigPath    string                 `json:"config_path"`
	TotalRules    int                    `json:"total_rules"`
	EnforcedRules int                    `json:"enforced_rules"`
	FilesScanned  int                    `json:"files_scanned"`
	Violations    []RuleViolation        `json:"violations"`
	FixesApplied  int                    `json:"fixes_applied"`
	Duration      time.Duration          `json:"duration"`
	AgentUsed     string                 `json:"agent_used,omitempty"`
	Success       bool                   `json:"success"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// EnforcementOptions configures enforcement execution
type EnforcementOptions struct {
	ConfigFile   string            `yaml:"config_file,omitempty"`
	WorkingDir   string            `yaml:"working_dir,omitempty"`
	Enforcement  EnforcementConfig `yaml:"enforcement"`
	DryRun       bool              `yaml:"dry_run,omitempty"`
	Verbose      bool              `yaml:"verbose,omitempty"`
	OnlyRules    []string          `yaml:"only_rules,omitempty"`
	ExcludeRules []string          `yaml:"exclude_rules,omitempty"`
	OutputFormat string            `yaml:"output_format,omitempty"`
}

// EnforceableRule extends config.Rule with enforcement metadata
type EnforceableRule struct {
	config.Rule
	Enforcement EnforcementConfig `yaml:"enforcement,omitempty"`
}

// EnforceableSection extends config.Section with enforcement metadata
type EnforceableSection struct {
	config.Section
	Enforcement EnforcementConfig `yaml:"enforcement,omitempty"`
}
