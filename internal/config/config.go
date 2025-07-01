package config

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.yaml
var profilesFS embed.FS

// Config represents the main configuration structure
type Config struct {
	Metadata Metadata    `yaml:"metadata"`
	Profile  interface{} `yaml:"profile,omitempty"`
	Includes []string    `yaml:"includes,omitempty"`
	Outputs  []Output    `yaml:"outputs"`
	Rules    []Rule      `yaml:"rules,omitempty"`
	Sections []Section   `yaml:"sections,omitempty"`
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
	Name     string `yaml:"name"`
	Priority int    `yaml:"priority,omitempty"`
	Content  string `yaml:"content"`
}

// Section represents an informative text section
type Section struct {
	Title    string `yaml:"title"`
	Priority int    `yaml:"priority,omitempty"`
	Content  string `yaml:"content"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	// Validate against schema first
	if err := ValidateWithSchema(data); err != nil {
		return nil, fmt.Errorf("schema validation failed for %s: %w", filename, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filename, err)
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

	// Merge profiles
	if err := config.MergeWithProfiles(); err != nil {
		return nil, fmt.Errorf("failed to merge profiles: %w", err)
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
		return errors.New("metadata.name is required")
	}

	if len(c.Outputs) == 0 {
		return errors.New("at least one output must be defined")
	}

	for i, output := range c.Outputs {
		if output.File == "" {
			return fmt.Errorf("output[%d].file is required", i)
		}
	}

	return nil
}

// GetProfileNames parses the profile field and returns a list of profile names
func (c *Config) GetProfileNames() []string {
	if c.Profile == nil {
		return []string{"default"}
	}

	switch v := c.Profile.(type) {
	case string:
		if v == "" {
			return []string{"default"}
		}
		return []string{v}
	case []interface{}:
		names := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				names = append(names, str)
			}
		}
		if len(names) == 0 {
			return []string{"default"}
		}
		return names
	default:
		return []string{"default"}
	}
}

// LoadProfile loads a built-in profile by name
func LoadProfile(name string) (*Config, error) {
	filename := fmt.Sprintf("profiles/%s.yaml", name)
	data, err := profilesFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("profile '%s' not found: %w", name, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse profile '%s': %w", name, err)
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

	return &config, nil
}

// MergeWithProfiles merges the config with the specified profiles
func (c *Config) MergeWithProfiles() error {
	profileNames := c.GetProfileNames()

	// Load and merge profiles in order
	mergedRules := make(map[string]Rule)
	mergedSections := make(map[string]Section)

	for _, profileName := range profileNames {
		profile, err := LoadProfile(profileName)
		if err != nil {
			return fmt.Errorf("failed to load profile '%s': %w", profileName, err)
		}

		// Merge rules - later profiles override earlier ones with same name
		for _, rule := range profile.Rules {
			// Boost priority for profile rules to make them prominent
			rule.Priority += 10
			mergedRules[rule.Name] = rule
		}

		// Merge sections
		for _, section := range profile.Sections {
			section.Priority += 10
			mergedSections[section.Title] = section
		}
	}

	// Convert maps back to slices and prepend to existing rules
	profileRules := make([]Rule, 0, len(mergedRules))
	for _, rule := range mergedRules {
		profileRules = append(profileRules, rule)
	}

	profileSections := make([]Section, 0, len(mergedSections))
	for _, section := range mergedSections {
		profileSections = append(profileSections, section)
	}

	// Merge with existing rules - local rules override profile rules with same name
	finalRules := make(map[string]Rule)

	// Add profile rules first
	for _, rule := range profileRules {
		finalRules[rule.Name] = rule
	}

	// Add/override with local rules
	for _, rule := range c.Rules {
		if rule.Priority == 0 {
			rule.Priority = 1
		}
		finalRules[rule.Name] = rule
	}

	// Convert back to slice
	c.Rules = make([]Rule, 0, len(finalRules))
	for _, rule := range finalRules {
		c.Rules = append(c.Rules, rule)
	}

	// Same for sections
	finalSections := make(map[string]Section)

	for _, section := range profileSections {
		finalSections[section.Title] = section
	}

	for _, section := range c.Sections {
		if section.Priority == 0 {
			section.Priority = 1
		}
		finalSections[section.Title] = section
	}

	c.Sections = make([]Section, 0, len(finalSections))
	for _, section := range finalSections {
		c.Sections = append(c.Sections, section)
	}

	return nil
}

// ListAvailableProfiles returns the names of all available built-in profiles
func ListAvailableProfiles() ([]string, error) {
	entries, err := profilesFS.ReadDir("profiles")
	if err != nil {
		return nil, fmt.Errorf("failed to read profiles directory: %w", err)
	}

	var profiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			name := strings.TrimSuffix(entry.Name(), ".yaml")
			profiles = append(profiles, name)
		}
	}

	return profiles, nil
}
