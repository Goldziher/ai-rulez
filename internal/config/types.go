package config

import "strings"

// Config represents the main configuration structure
type Config struct {
	Metadata   Metadata    `yaml:"metadata"`
	Extends    string      `yaml:"extends,omitempty"`
	Includes   []string    `yaml:"includes,omitempty"`
	Outputs    []Output    `yaml:"outputs"`
	Rules      []Rule      `yaml:"rules,omitempty"`
	Sections   []Section   `yaml:"sections,omitempty"`
	Agents     []Agent     `yaml:"agents,omitempty"`
	MCPServers []MCPServer `yaml:"mcp_servers,omitempty"`
	Commands   []Command   `yaml:"commands,omitempty"`
}

// Metadata contains project metadata
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// Output represents an output file configuration
type Output struct {
	Path         string         `yaml:"path"`
	Type         string         `yaml:"type,omitempty"`
	Template     TemplateConfig `yaml:"template,omitempty"`
	NamingScheme string         `yaml:"naming_scheme,omitempty"`
}

// GetTemplate parses the template configuration
func (o *Output) GetTemplate() (*Template, error) {
	return ParseTemplate(o.Template)
}

// IsDirectory checks if the output is a directory
func (o *Output) IsDirectory() bool {
	return strings.HasSuffix(o.Path, "/")
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
	Priority Priority `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

// Section represents a documentation section
type Section struct {
	ID       string   `yaml:"id,omitempty"`
	Name     string   `yaml:"name"`
	Priority Priority `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

// Agent represents an AI agent configuration
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

// GetTemplate parses the template configuration
func (a *Agent) GetTemplate() (*Template, error) {
	return ParseTemplate(a.Template)
}

// MCPServer represents a Model Context Protocol server configuration
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

// IsEnabled returns whether the MCP server is enabled (default: true)
func (m *MCPServer) IsEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// GetTransport returns the transport protocol with default
func (m *MCPServer) GetTransport() string {
	if m.Transport == "" {
		return "stdio"
	}
	return m.Transport
}

// Command represents a custom slash command configuration
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

// IsEnabled returns whether the command is enabled (default: true)
func (c *Command) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}
