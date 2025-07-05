package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfigWithoutProfiles loads configuration from a YAML file without profile merging
// This is used for testing to maintain backward compatibility
func LoadConfigWithoutProfiles(filename string) (*Config, error) {
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

	// Do NOT merge profiles - this is for testing
	return &config, nil
}

// LoadConfigWithIncludesWithoutProfiles loads a configuration file and resolves all includes without profile merging
// This is used for testing to maintain backward compatibility
func LoadConfigWithIncludesWithoutProfiles(filename string) (*Config, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}

	loader := &configLoaderNoProfile{
		visited: make(map[string]bool),
		baseDir: filepath.Dir(absPath),
	}

	config, err := loader.loadConfig(absPath)
	if err != nil {
		return nil, err
	}

	// Load and merge additional config files (without profiles)
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

// configLoaderNoProfile handles recursive include resolution with cycle detection without profiles
type configLoaderNoProfile struct {
	visited map[string]bool
	baseDir string
}

// loadConfig loads a config file and resolves includes recursively without profile merging
func (l *configLoaderNoProfile) loadConfig(filename string) (*Config, error) {
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

	// Load the main config WITHOUT profile merging
	config, err := LoadConfigWithoutProfiles(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config %s: %w", absPath, err)
	}

	// Resolve includes
	if err := l.resolveIncludes(config, filepath.Dir(absPath)); err != nil {
		return nil, fmt.Errorf("failed to resolve includes in %s: %w", absPath, err)
	}

	return config, nil
}

// resolveIncludes processes all include paths and merges rules and sections without profile merging
func (l *configLoaderNoProfile) resolveIncludes(config *Config, baseDir string) error {
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

// resolvePath resolves relative paths against the base directory
func (*configLoaderNoProfile) resolvePath(includePath, baseDir string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}
	return filepath.Join(baseDir, includePath)
}

// loadLocalOverrides loads local override rules from .local.yaml file (no-profile version)
func (l *configLoaderNoProfile) loadLocalOverrides(config *Config, filename string) error {
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
