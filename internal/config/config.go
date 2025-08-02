package config

import (
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Metadata  Metadata   `yaml:"metadata"`
	Includes  []string   `yaml:"includes,omitempty"`
	Outputs   []Output   `yaml:"outputs"`
	Rules     []Rule     `yaml:"rules,omitempty"`
	Sections  []Section  `yaml:"sections,omitempty"`
	UserRulez *UserRulez `yaml:"user_rulez,omitempty"`
}

// UserRulez contains user-specific rules and sections
type UserRulez struct {
	Rules    []Rule    `yaml:"rules,omitempty"`
	Sections []Section `yaml:"sections,omitempty"`
}

// Metadata contains project metadata
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// Output defines where and how to generate rule files
type Output struct {
	File     string `yaml:"file"`
	Template string `yaml:"template,omitempty"`
}

// Rule represents a single rule definition
type Rule struct {
	ID       string `yaml:"id,omitempty"`
	Name     string `yaml:"name"`
	Priority int    `yaml:"priority,omitempty"`
	Content  string `yaml:"content"`
}

// Section represents an informative text section
type Section struct {
	ID       string `yaml:"id,omitempty"`
	Title    string `yaml:"title"`
	Priority int    `yaml:"priority,omitempty"`
	Content  string `yaml:"content"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.FileRead(filename, err)
	}

	// Validate against schema first
	if err := ValidateWithSchema(data); err != nil {
		return nil, errors.SchemaValidation(filename, []string{err.Error()})
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.ConfigParse(filename, err)
	}

	// Set default priority for rules
	for i := range config.Rules {
		if config.Rules[i].Priority == 0 {
			config.Rules[i].Priority = 1
		}
	}

	// Set default priority for sections
	for i := range config.Sections {
		if config.Sections[i].Priority == 0 {
			config.Sections[i].Priority = 1
		}
	}

	// Set default priority for user_rulez
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
	}

	return &config, nil
}

// SaveConfig saves configuration to a YAML file
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

// Validate checks the configuration for common errors
func (c *Config) Validate() error {
	if c.Metadata.Name == "" {
		return errors.ValidationRequired("metadata.name", "config metadata").
			WithSuggestion("Add a name field to the metadata section").
			WithSuggestion("Example: metadata: {name: 'My Project'}")
	}

	return ValidateOutputs(c.Outputs)
}
