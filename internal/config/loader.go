// Package config provides configuration loading and validation for ai_rules.
package config

import (
	"fmt"
	"os"
	"path/filepath"
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

	return loader.loadConfig(absPath)
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

// resolveIncludes processes all include paths and merges rules.
func (l *configLoader) resolveIncludes(config *Config, baseDir string) error {
	if len(config.Includes) == 0 {
		return nil
	}

	var allRules []Rule
	// Add existing rules first
	allRules = append(allRules, config.Rules...)

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

		// Merge rules from included config
		allRules = append(allRules, includedConfig.Rules...)
	}

	// Update config with merged rules and clear includes
	config.Rules = allRules
	config.Includes = nil

	return nil
}

// resolvePath resolves relative paths against the base directory.
func (l *configLoader) resolvePath(includePath, baseDir string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}
	return filepath.Join(baseDir, includePath)
}

// MergeRules combines multiple rule slices, with later rules taking precedence.
func MergeRules(ruleSets ...[]Rule) []Rule {
	ruleMap := make(map[string]Rule)
	var order []string

	for _, rules := range ruleSets {
		for _, rule := range rules {
			// Track order for consistent output
			if _, exists := ruleMap[rule.Name]; !exists {
				order = append(order, rule.Name)
			}
			ruleMap[rule.Name] = rule
		}
	}

	// Rebuild slice in order
	result := make([]Rule, 0, len(order))
	for _, name := range order {
		result = append(result, ruleMap[name])
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
		return fmt.Errorf("at least one output must be defined")
	}

	for i, output := range outputs {
		if output.File == "" {
			return fmt.Errorf("output[%d].file is required", i)
		}
	}

	return nil
}