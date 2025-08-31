package config

import "strings"

// Config represents the main configuration structure
type Config struct {
	Metadata  Metadata            `yaml:"metadata"`
	Includes  []string            `yaml:"includes,omitempty"`
	Targets   map[string][]string `yaml:"targets,omitempty"`
	Outputs   []Output            `yaml:"outputs"`
	Rules     []Rule              `yaml:"rules,omitempty"`
	Sections  []Section           `yaml:"sections,omitempty"`
	Agents    []Agent             `yaml:"agents,omitempty"`
	UserRulez *UserRulez          `yaml:"user_rulez,omitempty"`
}

// UserRulez contains user-specific rules configuration
type UserRulez struct {
	Rules    []Rule    `yaml:"rules,omitempty"`
	Sections []Section `yaml:"sections,omitempty"`
	Agents   []Agent   `yaml:"agents,omitempty"`
}

// Metadata contains project metadata
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// Output represents an output file configuration
type Output struct {
	File         string `yaml:"file,omitempty"`
	Path         string `yaml:"path,omitempty"`
	Type         string `yaml:"type,omitempty"`
	Template     string `yaml:"template,omitempty"`
	NamingScheme string `yaml:"naming_scheme,omitempty"`
}

// GetPath returns the output path
func (o *Output) GetPath() string {
	if o.Path != "" {
		return o.Path
	}
	return o.File
}

// IsDirectory checks if the output is a directory
func (o *Output) IsDirectory() bool {
	path := o.GetPath()
	return strings.HasSuffix(path, "/")
}

// GetOutputType returns the output type with default
func (o *Output) GetOutputType() string {
	if o.Type == "" {
		return "rule"
	}
	return o.Type
}

// GetNamingScheme returns the naming scheme for directory outputs
func (o *Output) GetNamingScheme() string {
	if o.NamingScheme != "" {
		return o.NamingScheme
	}
	if o.GetOutputType() == "agent" {
		return "{name}.md"
	}
	return "{type}.md"
}

// Rule represents a single rule configuration
type Rule struct {
	ID       string   `yaml:"id,omitempty"`
	Name     string   `yaml:"name"`
	Priority int      `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

// Section represents a documentation section
type Section struct {
	ID       string   `yaml:"id,omitempty"`
	Name     string   `yaml:"name"`
	Priority int      `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

// Agent represents an AI agent configuration
type Agent struct {
	ID           string   `yaml:"id,omitempty"`
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Priority     int      `yaml:"priority,omitempty"`
	Tools        []string `yaml:"tools,omitempty"`
	Template     string   `yaml:"template,omitempty"`
	SystemPrompt string   `yaml:"system_prompt,omitempty"`
	Targets      []string `yaml:"targets,omitempty"`
}
