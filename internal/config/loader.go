// Package config provides configuration loading and validation for ai_rules.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadConfigWithIncludes loads a configuration file and resolves all includes.
func LoadConfigWithIncludes(filename string) (*Config, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}

	loader := &configLoader{
		visited: make(map[string]bool),
		baseDir: filepath.Dir(absPath),
	}

	config, err := loader.loadConfig(absPath)
	if err != nil {
		return nil, err
	}

	// Load and merge additional config files
	baseDir := filepath.Dir(absPath)

	// Load .local.yaml files for ID-based overrides (loaded last for highest precedence)
	configBaseName := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	localConfigPath := filepath.Join(baseDir, configBaseName+".local.yaml")
	if _, err := os.Stat(localConfigPath); err == nil {
		if err := loader.loadLocalOverrides(config, localConfigPath); err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", filepath.Base(localConfigPath), err)
		}
	}

	return config, nil
}

// configLoader handles recursive include resolution with cycle detection.
type configLoader struct {
	visited map[string]bool
	baseDir string
}

// loadConfig loads a config file and resolves includes recursively.
func (l *configLoader) loadConfig(filename string) (*Config, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}

	// Check for circular includes
	if l.visited[absPath] {
		return nil, fmt.Errorf("circular include detected: %s", absPath)
	}
	l.visited[absPath] = true
	defer func() { l.visited[absPath] = false }()

	// Load the main config
	config, err := LoadConfig(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config %s: %w", absPath, err)
	}

	// Resolve includes
	if err := l.resolveIncludes(config, filepath.Dir(absPath)); err != nil {
		return nil, fmt.Errorf("failed to resolve includes in %s: %w", absPath, err)
	}

	return config, nil
}

// resolveIncludes processes all include paths and merges rules and sections.
func (l *configLoader) resolveIncludes(config *Config, baseDir string) error {
	if len(config.Includes) == 0 {
		return nil
	}

	var allRules []Rule
	var allSections []Section
	// Add existing rules and sections first
	allRules = append(allRules, config.Rules...)
	allSections = append(allSections, config.Sections...)

	// Process each include
	for _, includePath := range config.Includes {
		resolvedPath := l.resolvePath(includePath, baseDir)

		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			return fmt.Errorf("include file not found: %s (resolved to %s)", includePath, resolvedPath)
		}

		includedConfig, err := l.loadConfig(resolvedPath)
		if err != nil {
			return fmt.Errorf("failed to load include %s: %w", includePath, err)
		}

		// Merge rules and sections from included config
		allRules = append(allRules, includedConfig.Rules...)
		allSections = append(allSections, includedConfig.Sections...)
	}

	// Update config with merged rules and sections, clear includes
	config.Rules = MergeRules(allRules)
	config.Sections = MergeSections(allSections)
	config.Includes = nil

	// Ensure all rules have priority (default to 1)
	for i := range config.Rules {
		if config.Rules[i].Priority == 0 {
			config.Rules[i].Priority = 1
		}
	}

	// Ensure all sections have priority (default to 1)
	for i := range config.Sections {
		if config.Sections[i].Priority == 0 {
			config.Sections[i].Priority = 1
		}
	}

	return nil
}

// resolvePath resolves relative paths against the base directory.
func (*configLoader) resolvePath(includePath, baseDir string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}
	return filepath.Join(baseDir, includePath)
}

// MergeRules combines multiple rule slices, with later rules taking precedence.
// Rules with IDs are matched by ID first, then by name for backward compatibility.
func MergeRules(ruleSets ...[]Rule) []Rule {
	ruleMap := make(map[string]Rule) // key is ID if present, otherwise name
	var order []string

	for _, rules := range ruleSets {
		for _, rule := range rules {
			// Use ID as key if present, otherwise use name
			key := rule.Name
			if rule.ID != "" {
				key = rule.ID
			}

			// Track order for consistent output
			if _, exists := ruleMap[key]; !exists {
				order = append(order, key)
			}
			ruleMap[key] = rule
		}
	}

	// Rebuild slice in order
	result := make([]Rule, 0, len(order))
	for _, key := range order {
		result = append(result, ruleMap[key])
	}

	return result
}

// MergeSections combines multiple section slices, with later sections taking precedence.
// Sections with IDs are matched by ID first, then by title for backward compatibility.
func MergeSections(sectionSets ...[]Section) []Section {
	sectionMap := make(map[string]Section) // key is ID if present, otherwise title
	var order []string

	for _, sections := range sectionSets {
		for _, section := range sections {
			// Use ID as key if present, otherwise use title
			key := section.Title
			if section.ID != "" {
				key = section.ID
			}

			// Track order for consistent output
			if _, exists := sectionMap[key]; !exists {
				order = append(order, key)
			}
			sectionMap[key] = section
		}
	}

	// Rebuild slice in order
	result := make([]Section, 0, len(order))
	for _, key := range order {
		result = append(result, sectionMap[key])
	}

	return result
}

// ValidateIncludes checks that all include paths are valid and accessible.
func ValidateIncludes(config *Config, baseDir string) error {
	for _, includePath := range config.Includes {
		var resolvedPath string
		if filepath.IsAbs(includePath) {
			resolvedPath = includePath
		} else {
			resolvedPath = filepath.Join(baseDir, includePath)
		}

		// Check if file exists
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			return fmt.Errorf("include file not found: %s", includePath)
		}

		// Check if it's a valid YAML file by attempting to parse
		if _, err := LoadConfig(resolvedPath); err != nil {
			return fmt.Errorf("invalid include file %s: %w", includePath, err)
		}
	}

	return nil
}

// ValidateOutputs checks that all outputs have valid file paths.
func ValidateOutputs(outputs []Output) error {
	if len(outputs) == 0 {
		return errors.New("at least one output must be defined")
	}

	for i, output := range outputs {
		if output.File == "" {
			return fmt.Errorf("output[%d].file is required", i)
		}
	}

	return nil
}

// loadLocalOverrides loads local override rules from .local.yaml file
func (l *configLoader) loadLocalOverrides(config *Config, filename string) error {
	// Load the local config file
	localConfig, err := l.loadConfig(filename)
	if err != nil {
		return fmt.Errorf("failed to load local config: %w", err)
	}

	// Merge rules and sections using ID-based merging
	config.Rules = MergeRules(config.Rules, localConfig.Rules)
	config.Sections = MergeSections(config.Sections, localConfig.Sections)

	// Also merge user_rulez if present in local config
	if localConfig.UserRulez != nil {
		if config.UserRulez == nil {
			config.UserRulez = localConfig.UserRulez
		} else {
			config.UserRulez.Rules = MergeRules(config.UserRulez.Rules, localConfig.UserRulez.Rules)
			config.UserRulez.Sections = MergeSections(config.UserRulez.Sections, localConfig.UserRulez.Sections)
		}
	}

	return nil
}
