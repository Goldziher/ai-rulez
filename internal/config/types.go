package config

import (
	"strings"
)

type Config struct {
	Metadata   Metadata    `yaml:"metadata"`
	Extends    string      `yaml:"extends,omitempty"`
	Includes   []string    `yaml:"includes,omitempty"`
	Gitignore  *bool       `yaml:"gitignore,omitempty"`
	Outputs    []Output    `yaml:"outputs,omitempty"`
	Presets    []string    `yaml:"presets,omitempty"`
	Rules      []Rule      `yaml:"rules,omitempty"`
	Sections   []Section   `yaml:"sections,omitempty"`
	Agents     []Agent     `yaml:"agents,omitempty"`
	MCPServers []MCPServer `yaml:"mcp_servers,omitempty"`
	Commands   []Command   `yaml:"commands,omitempty"`
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
	ID       string   `yaml:"id,omitempty"`
	Name     string   `yaml:"name"`
	Priority Priority `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

type Section struct {
	ID       string   `yaml:"id,omitempty"`
	Name     string   `yaml:"name"`
	Priority Priority `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
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
