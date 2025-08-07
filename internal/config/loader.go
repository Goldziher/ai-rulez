// Package config provides configuration loading and validation for ai_rules.
package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/Goldziher/ai-rulez/internal/remote"
	"gopkg.in/yaml.v3"
)

// LoadConfigWithIncludes loads a configuration file and resolves all includes.
func LoadConfigWithIncludes(filename string) (*Config, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, errors.FileRead(filename, err).
			WithContext("operation", "resolve absolute path").
			WithSuggestion("Check if the file path is valid and accessible")
	}

	loader := &configLoader{
		visited:      make(map[string]bool),
		baseDir:      filepath.Dir(absPath),
		remoteClient: remote.NewClient(nil), // Use default HTTP config
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
			return nil, err // Already a rich error from loadLocalOverrides
		}
	}

	return config, nil
}

// configLoader handles recursive include resolution with cycle detection.
type configLoader struct {
	visited      map[string]bool
	baseDir      string
	remoteClient *remote.Client
}

// loadConfig loads a config file and resolves includes recursively.
func (l *configLoader) loadConfig(filename string) (*Config, error) {
	// Determine if this is a URL or local file
	configKey := filename
	if l.isURL(filename) {
		configKey = filename // Use URL as key for cycle detection
	} else {
		absPath, err := filepath.Abs(filename)
		if err != nil {
			return nil, errors.FileRead(filename, err).
				WithContext("operation", "resolve absolute path").
				WithSuggestion("Check if the file path is valid and accessible")
		}
		configKey = absPath
	}

	// Check for circular includes
	if l.visited[configKey] {
		// Build include chain for better error context
		chain := make([]string, 0)
		for path := range l.visited {
			if l.visited[path] {
				chain = append(chain, path)
			}
		}
		chain = append(chain, configKey)
		return nil, errors.CircularInclude(chain)
	}
	l.visited[configKey] = true
	defer func() { l.visited[configKey] = false }()

	// Load the main config (local file or remote URL)
	var config *Config
	var err error
	if l.isURL(filename) {
		config, err = l.loadRemoteConfig(filename)
	} else {
		config, err = LoadConfig(filename)
	}
	if err != nil {
		return nil, errors.ConfigLoad(filename, err).
			WithContext("operation", "loading main config")
	}

	// Resolve includes
	var baseDir string
	if l.isURL(filename) {
		// For remote URLs, keep track of the base URL for relative includes
		baseDir = filename
	} else {
		baseDir = filepath.Dir(filename)
	}
	if err := l.resolveIncludes(config, baseDir); err != nil {
		return nil, err
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
	var allAgents []Agent
	// Add existing rules, sections, and agents first
	allRules = append(allRules, config.Rules...)
	allSections = append(allSections, config.Sections...)
	allAgents = append(allAgents, config.Agents...)

	// Process each include
	for _, includePath := range config.Includes {
		resolvedPath := l.resolvePath(includePath, baseDir)

		// Check existence only for local files
		if !l.isURL(resolvedPath) {
			if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
				return errors.New(errors.ErrorTypeConfigNotFound, "resolve include",
					fmt.Errorf("include file not found: %s", includePath)).
					WithPath(resolvedPath).
					WithContext("include_path", includePath).
					WithContext("resolved_path", resolvedPath).
					WithSuggestion("Check if the include file exists: %s", resolvedPath).
					WithSuggestion("Verify the relative path is correct relative to %s", baseDir).
					WithSuggestion("Use an absolute path if the relative path is unclear")
			}
		}

		includedConfig, err := l.loadConfig(resolvedPath)
		if err != nil {
			return err // Already a rich error from loadConfig
		}

		// Merge rules, sections, and agents from included config
		allRules = append(allRules, includedConfig.Rules...)
		allSections = append(allSections, includedConfig.Sections...)
		allAgents = append(allAgents, includedConfig.Agents...)
	}

	// Update config with merged rules, sections, and agents, clear includes
	config.Rules = MergeRules(allRules)
	config.Sections = MergeSections(allSections)
	config.Agents = MergeAgents(allAgents)
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

	// Ensure all agents have priority (default to 1)
	for i := range config.Agents {
		if config.Agents[i].Priority == 0 {
			config.Agents[i].Priority = 1
		}
	}

	return nil
}

// resolvePath resolves relative paths against the base directory or URL.
func (l *configLoader) resolvePath(includePath, baseDir string) string {
	// If includePath is already a URL, return as-is
	if l.isURL(includePath) {
		return includePath
	}

	// If includePath is absolute, return as-is
	if filepath.IsAbs(includePath) {
		return includePath
	}

	// If baseDir is a URL, resolve relative to the URL
	if l.isURL(baseDir) {
		baseURL, err := url.Parse(baseDir)
		if err != nil {
			// If we can't parse the base URL, just return the include path
			return includePath
		}
		relativeURL, err := url.Parse(includePath)
		if err != nil {
			// If we can't parse the relative path, just return it
			return includePath
		}
		resolved := baseURL.ResolveReference(relativeURL)
		return resolved.String()
	}

	// Normal file path resolution
	return filepath.Join(baseDir, includePath)
}

// isURL checks if a string is a valid HTTP/HTTPS URL.
func (*configLoader) isURL(path string) bool {
	parsedURL, err := url.Parse(path)
	if err != nil {
		return false
	}
	return parsedURL.Scheme == "http" || parsedURL.Scheme == "https"
}

// loadRemoteConfig fetches and parses a configuration from a remote URL.
func (l *configLoader) loadRemoteConfig(configURL string) (*Config, error) {
	ctx := context.Background()

	// Fetch the remote content
	content, err := l.remoteClient.Fetch(ctx, configURL)
	if err != nil {
		return nil, errors.RemoteConfigFetch(configURL, err)
	}

	// Parse the YAML content
	var config Config
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, errors.RemoteConfigParse(configURL, err)
	}

	return &config, nil
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

// MergeAgents combines multiple agent slices, with later agents taking precedence.
// Agents with IDs are matched by ID first, then by name for backward compatibility.
func MergeAgents(agentSets ...[]Agent) []Agent {
	agentMap := make(map[string]Agent) // key is ID if present, otherwise name
	var order []string

	for _, agents := range agentSets {
		for _, agent := range agents {
			// Use ID as key if present, otherwise use name
			key := agent.Name
			if agent.ID != "" {
				key = agent.ID
			}

			// Track order for consistent output
			if _, exists := agentMap[key]; !exists {
				order = append(order, key)
			}
			agentMap[key] = agent
		}
	}

	// Rebuild slice in order
	result := make([]Agent, 0, len(order))
	for _, key := range order {
		result = append(result, agentMap[key])
	}

	return result
}

// ValidateIncludes checks that all include paths are valid and accessible.
func ValidateIncludes(config *Config, baseDir string) error {
	loader := &configLoader{
		visited:      make(map[string]bool),
		baseDir:      baseDir,
		remoteClient: remote.NewClient(nil),
	}

	for _, includePath := range config.Includes {
		resolvedPath := loader.resolvePath(includePath, baseDir)

		// Check existence for local files
		if !loader.isURL(resolvedPath) {
			if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
				return errors.New(errors.ErrorTypeConfigNotFound, "validate include",
					fmt.Errorf("include file not found: %s", includePath)).
					WithPath(resolvedPath).
					WithContext("include_path", includePath).
					WithContext("base_dir", baseDir).
					WithSuggestion("Check if the include file exists: %s", resolvedPath).
					WithSuggestion("Verify the path is correct relative to %s", baseDir)
			}
		}

		// Check if it's a valid YAML file by attempting to load it
		var err error
		if loader.isURL(resolvedPath) {
			_, err = loader.loadRemoteConfig(resolvedPath)
		} else {
			_, err = LoadConfig(resolvedPath)
		}
		if err != nil {
			return errors.ConfigLoad(resolvedPath, err).
				WithContext("include_path", includePath).
				WithContext("operation", "validate include syntax")
		}
	}

	return nil
}

// ValidateOutputs checks that all outputs have valid file paths.
func ValidateOutputs(outputs []Output) error {
	if len(outputs) == 0 {
		return errors.ValidationRequired("outputs", "configuration").
			WithSuggestion("Add at least one output file in the 'outputs' section").
			WithSuggestion("Example: outputs: [{file: 'CLAUDE.md', template: 'default'}]")
	}

	for i, output := range outputs {
		if output.Path == "" && output.File == "" {
			return errors.ValidationRequired(fmt.Sprintf("outputs[%d].path or outputs[%d].file", i, i), "output configuration").
				WithContext("output_index", i).
				WithSuggestion("Specify a path or file for output[%d]", i).
				WithSuggestion("Example: path: 'CLAUDE.md' (preferred) or file: 'CLAUDE.md'")
		}
	}

	return nil
}

// loadLocalOverrides loads local override rules from .local.yaml file
func (l *configLoader) loadLocalOverrides(config *Config, filename string) error {
	// Load the local config file
	localConfig, err := l.loadConfig(filename)
	if err != nil {
		return err // Already a rich error from loadConfig
	}

	// Merge rules, sections, and agents using ID-based merging
	config.Rules = MergeRules(config.Rules, localConfig.Rules)
	config.Sections = MergeSections(config.Sections, localConfig.Sections)
	config.Agents = MergeAgents(config.Agents, localConfig.Agents)

	// Also merge user_rulez if present in local config
	if localConfig.UserRulez != nil {
		if config.UserRulez == nil {
			config.UserRulez = localConfig.UserRulez
		} else {
			config.UserRulez.Rules = MergeRules(config.UserRulez.Rules, localConfig.UserRulez.Rules)
			config.UserRulez.Sections = MergeSections(config.UserRulez.Sections, localConfig.UserRulez.Sections)
			config.UserRulez.Agents = MergeAgents(config.UserRulez.Agents, localConfig.UserRulez.Agents)
		}
	}

	return nil
}
