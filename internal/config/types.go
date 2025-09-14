package config

import (
	"strings"
	"time"
)

type Config struct {
	Metadata    Metadata                 `yaml:"metadata"`
	Extends     string                   `yaml:"extends,omitempty"`
	Includes    []string                 `yaml:"includes,omitempty"`
	Gitignore   *bool                    `yaml:"gitignore,omitempty"`
	Outputs     []Output                 `yaml:"outputs,omitempty"`
	Presets     []string                 `yaml:"presets,omitempty"`
	Rules       []Rule                   `yaml:"rules,omitempty"`
	Sections    []Section                `yaml:"sections,omitempty"`
	Agents      []Agent                  `yaml:"agents,omitempty"`
	MCPServers  []MCPServer              `yaml:"mcp_servers,omitempty"`
	Commands    []Command                `yaml:"commands,omitempty"`
	Enforcement *EnforcementGlobalConfig `yaml:"enforcement,omitempty"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Output struct {
	Path         string         `yaml:"path"`
	Type         string         `yaml:"type,omitempty"`
	Template     TemplateConfig `yaml:"template,omitempty"`
	NamingScheme string         `yaml:"naming_scheme,omitempty"`
}

func (o *Output) GetTemplate() (*Template, error) {
	return ParseTemplate(o.Template)
}

func (o *Output) IsDirectory() bool {
	return strings.HasSuffix(o.Path, "/")
}

func (o *Output) GetOutputType() string {
	if o.Type == "" {
		return "rule"
	}
	return o.Type
}

func (o *Output) GetNamingScheme() string {
	if o.NamingScheme != "" {
		return o.NamingScheme
	}
	if o.GetOutputType() == "agent" {
		return "{name}.md"
	}
	return "{type}.md"
}

type Rule struct {
	ID          string                 `yaml:"id,omitempty"`
	Name        string                 `yaml:"name"`
	Priority    Priority               `yaml:"priority,omitempty"`
	Content     string                 `yaml:"content"`
	Targets     []string               `yaml:"targets,omitempty"`
	Enforcement *RuleEnforcementConfig `yaml:"enforcement,omitempty"`
}

type Section struct {
	ID          string                 `yaml:"id,omitempty"`
	Name        string                 `yaml:"name"`
	Priority    Priority               `yaml:"priority,omitempty"`
	Content     string                 `yaml:"content"`
	Targets     []string               `yaml:"targets,omitempty"`
	Enforcement *RuleEnforcementConfig `yaml:"enforcement,omitempty"`
}

type Agent struct {
	ID           string         `yaml:"id,omitempty"`
	Name         string         `yaml:"name"`
	Description  string         `yaml:"description"`
	Priority     Priority       `yaml:"priority,omitempty"`
	Tools        []string       `yaml:"tools,omitempty"`
	Model        string         `yaml:"model,omitempty"`
	Template     TemplateConfig `yaml:"template,omitempty"`
	SystemPrompt string         `yaml:"system_prompt,omitempty"`
	Targets      []string       `yaml:"targets,omitempty"`
}

func (r *Rule) GetName() string       { return r.Name }
func (r *Rule) GetPriority() Priority { return r.Priority }

func (s *Section) GetName() string       { return s.Name }
func (s *Section) GetPriority() Priority { return s.Priority }

func (a *Agent) GetName() string       { return a.Name }
func (a *Agent) GetPriority() Priority { return a.Priority }

func (a *Agent) GetTemplate() (*Template, error) {
	return ParseTemplate(a.Template)
}

type MCPServer struct {
	ID          string            `yaml:"id,omitempty"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Command     string            `yaml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Transport   string            `yaml:"transport,omitempty"`
	URL         string            `yaml:"url,omitempty"`
	Enabled     *bool             `yaml:"enabled,omitempty"`
	Targets     []string          `yaml:"targets,omitempty"`
}

func (m *MCPServer) IsEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

func (m *MCPServer) GetTransport() string {
	if m.Transport == "" {
		return "stdio"
	}
	return m.Transport
}

type Command struct {
	ID           string   `yaml:"id,omitempty"`
	Name         string   `yaml:"name"`
	Aliases      []string `yaml:"aliases,omitempty"`
	Description  string   `yaml:"description"`
	Usage        string   `yaml:"usage,omitempty"`
	SystemPrompt string   `yaml:"system_prompt,omitempty"`
	Shortcut     string   `yaml:"shortcut,omitempty"`
	Enabled      *bool    `yaml:"enabled,omitempty"`
	Targets      []string `yaml:"targets,omitempty"`
}

func (c *Command) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func (c *Config) ShouldUpdateGitignore() bool {
	if c.Gitignore == nil {
		return true
	}
	return *c.Gitignore
}

// EnforcementGlobalConfig defines global enforcement settings in configuration
type EnforcementGlobalConfig struct {
	Enabled       *bool                    `yaml:"enabled,omitempty"`
	Level         string                   `yaml:"level,omitempty"`
	Agent         string                   `yaml:"agent,omitempty"`
	AutoFix       *bool                    `yaml:"auto_fix,omitempty"`
	Timeout       *time.Duration           `yaml:"timeout,omitempty"`
	IncludeFiles  []string                 `yaml:"include_files,omitempty"`
	ExcludeFiles  []string                 `yaml:"exclude_files,omitempty"`
	MaxViolations *int                     `yaml:"max_violations,omitempty"`
	Review        *EnforcementReviewConfig `yaml:"review,omitempty"`
}

// EnforcementReviewConfig defines review settings in configuration
type EnforcementReviewConfig struct {
	Enabled            *bool          `yaml:"enabled,omitempty"`
	MaxIterations      *int           `yaml:"max_iterations,omitempty"`
	Agent              string         `yaml:"agent,omitempty"`
	FeedbackThreshold  *float64       `yaml:"feedback_threshold,omitempty"`
	Timeout            *time.Duration `yaml:"timeout,omitempty"`
	AutoApprove        *bool          `yaml:"auto_approve,omitempty"`
	RequireImprovement *bool          `yaml:"require_improvement,omitempty"`
}

// IsEnforcementEnabled returns whether enforcement is enabled in the global config
func (c *Config) IsEnforcementEnabled() bool {
	if c.Enforcement == nil || c.Enforcement.Enabled == nil {
		return false
	}
	return *c.Enforcement.Enabled
}

// GetEnforcementLevel returns the enforcement level from global config
func (c *Config) GetEnforcementLevel() string {
	if c.Enforcement == nil || c.Enforcement.Level == "" {
		return "warn"
	}
	return c.Enforcement.Level
}

// GetEnforcementAgent returns the enforcement agent from global config
func (c *Config) GetEnforcementAgent() string {
	if c.Enforcement == nil {
		return ""
	}
	return c.Enforcement.Agent
}

// RuleEnforcementConfig defines per-rule enforcement settings
type RuleEnforcementConfig struct {
	Enabled *bool  `yaml:"enabled,omitempty"`
	Level   string `yaml:"level,omitempty"`
	Agent   string `yaml:"agent,omitempty"`
	AutoFix *bool  `yaml:"auto_fix,omitempty"`
}

// IsRuleEnforcementEnabled returns whether enforcement is enabled for a specific rule
func (r *Rule) IsEnforcementEnabled() bool {
	if r.Enforcement == nil || r.Enforcement.Enabled == nil {
		return true // Default to enabled
	}
	return *r.Enforcement.Enabled
}

// GetRuleEnforcementLevel returns the enforcement level for a specific rule
func (r *Rule) GetEnforcementLevel() string {
	if r.Enforcement == nil || r.Enforcement.Level == "" {
		return "" // Use global level
	}
	return r.Enforcement.Level
}

// IsRuleEnforcementEnabled returns whether enforcement is enabled for a specific section
func (s *Section) IsEnforcementEnabled() bool {
	if s.Enforcement == nil || s.Enforcement.Enabled == nil {
		return true // Default to enabled
	}
	return *s.Enforcement.Enabled
}

// GetRuleEnforcementLevel returns the enforcement level for a specific section
func (s *Section) GetEnforcementLevel() string {
	if s.Enforcement == nil || s.Enforcement.Level == "" {
		return "" // Use global level
	}
	return s.Enforcement.Level
}
