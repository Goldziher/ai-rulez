package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Metadata Metadata `yaml:"metadata"`
	Includes []string `yaml:"includes,omitempty"`
	Outputs  []Output `yaml:"outputs"`
	Rules    []Rule   `yaml:"rules,omitempty"`
}

// Metadata contains project metadata
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// Output defines where and how to generate rule files
type Output struct {
	Format   string `yaml:"format"`
	File     string `yaml:"file"`
	Template string `yaml:"template,omitempty"`
}

// Rule represents a single rule definition
type Rule struct {
	Name     string `yaml:"name"`
	Priority string `yaml:"priority,omitempty"`
	Content  string `yaml:"content"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filename, err)
	}

	return &config, nil
}

// SaveConfig saves configuration to a YAML file
func SaveConfig(config *Config, filename string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", filename, err)
	}

	return nil
}

// Validate checks the configuration for common errors
func (c *Config) Validate() error {
	if c.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if len(c.Outputs) == 0 {
		return fmt.Errorf("at least one output must be defined")
	}

	for i, output := range c.Outputs {
		if output.Format == "" {
			return fmt.Errorf("output[%d].format is required", i)
		}
		if output.File == "" {
			return fmt.Errorf("output[%d].file is required", i)
		}
	}

	return nil
}