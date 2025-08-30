package config

import (
	errorsStd "errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"gopkg.in/yaml.v3"
)

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

type UserRulez struct {
	Rules    []Rule    `yaml:"rules,omitempty"`
	Sections []Section `yaml:"sections,omitempty"`
	Agents   []Agent   `yaml:"agents,omitempty"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Output struct {
	File         string `yaml:"file,omitempty"`
	Path         string `yaml:"path,omitempty"`
	Type         string `yaml:"type,omitempty"`
	Template     string `yaml:"template,omitempty"`
	NamingScheme string `yaml:"naming_scheme,omitempty"`
}

func (o *Output) GetPath() string {
	if o.Path != "" {
		return o.Path
	}
	return o.File
}

func (o *Output) IsDirectory() bool {
	path := o.GetPath()
	return strings.HasSuffix(path, "/")
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
	Priority int      `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

type Section struct {
	ID       string   `yaml:"id,omitempty"`
	Title    string   `yaml:"title"`
	Priority int      `yaml:"priority,omitempty"`
	Content  string   `yaml:"content"`
	Targets  []string `yaml:"targets,omitempty"`
}

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

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.FileRead(filename, err)
	}

	if err := ValidateWithSchema(data); err != nil {
		var richErr *errors.RichError
		if errorsStd.As(err, &richErr) {
			if validationErrors, ok := richErr.Context["errors"].([]string); ok && len(validationErrors) > 0 {
				return nil, errors.SchemaValidation(filename, validationErrors)
			}
		}
		return nil, errors.SchemaValidation(filename, []string{err.Error()})
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.ConfigParse(filename, err)
	}

	setDefaultPriorities(&config)

	return &config, nil
}

func setDefaultPriorities(config *Config) {
	for i := range config.Rules {
		if config.Rules[i].Priority == 0 {
			config.Rules[i].Priority = 1
		}
	}

	for i := range config.Sections {
		if config.Sections[i].Priority == 0 {
			config.Sections[i].Priority = 1
		}
	}

	for i := range config.Agents {
		if config.Agents[i].Priority == 0 {
			config.Agents[i].Priority = 1
		}
	}

	if config.UserRulez != nil {
		for i := range config.UserRulez.Rules {
			if config.UserRulez.Rules[i].Priority == 0 {
				config.UserRulez.Rules[i].Priority = 1
			}
		}
		for i := range config.UserRulez.Sections {
			if config.UserRulez.Sections[i].Priority == 0 {
				config.UserRulez.Sections[i].Priority = 1
			}
		}
		for i := range config.UserRulez.Agents {
			if config.UserRulez.Agents[i].Priority == 0 {
				config.UserRulez.Agents[i].Priority = 1
			}
		}
	}
}

func SaveConfig(config *Config, filename string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return errors.New(errors.ErrorTypeConfig, "marshal config", err).
			WithPath(filename).
			WithSuggestion("Check if the config structure is valid")
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.FileWrite(dir, err).
			WithContext("operation", "create directory").
			WithSuggestion("Check if you have write permissions for the parent directory")
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return errors.FileWrite(filename, err)
	}

	return nil
}

func (c *Config) Validate() error {
	if c.Metadata.Name == "" {
		return errors.ValidationRequired("metadata.name", "config metadata").
			WithSuggestion("Add a name field to the metadata section").
			WithSuggestion("Example: metadata: {name: 'My Project'}")
	}

	return ValidateOutputs(c.Outputs)
}
