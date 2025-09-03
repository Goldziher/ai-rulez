package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/remote"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

func LoadConfigWithIncludes(ctx context.Context, filename string) (*Config, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, oops.
			With("path", filename).
			With("operation", "resolve absolute path").
			Hint("Check if the file path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}

	loader := &configLoader{
		visited:      make(map[string]bool),
		baseDir:      filepath.Dir(absPath),
		remoteClient: remote.NewClient(nil),
	}

	config, err := loader.loadConfig(ctx, absPath)
	if err != nil {
		return nil, err
	}

	// Check for local config file using our discovery logic
	localConfigPath, err := FindLocalConfigFile(absPath)
	if err != nil {
		return nil, oops.Wrapf(err, "find local config file")
	}

	if localConfigPath != "" {
		if err := loader.loadLocalOverrides(ctx, config, localConfigPath); err != nil {
			return nil, err
		}
	}

	return config, nil
}

type configLoader struct {
	visited      map[string]bool
	baseDir      string
	remoteClient *remote.Client
}

func (l *configLoader) loadConfig(ctx context.Context, filename string) (*Config, error) {
	return l.loadConfigInternal(ctx, filename, false)
}

func (l *configLoader) loadConfigInternal(ctx context.Context, filename string, isInclude bool) (*Config, error) {
	configKey := filename
	if l.isURL(filename) {
		configKey = filename
	} else {
		absPath, err := filepath.Abs(filename)
		if err != nil {
			return nil, oops.
				With("path", filename).
				With("operation", "resolve absolute path").
				Hint("Check if the file path is valid and accessible").
				Wrapf(err, "resolve absolute path")
		}
		configKey = absPath
	}

	if l.visited[configKey] {
		chain := make([]string, 0)
		for path := range l.visited {
			if l.visited[path] {
				chain = append(chain, path)
			}
		}
		chain = append(chain, configKey)
		return nil, oops.
			With("path", chain[len(chain)-1]).
			With("include_chain", chain).
			With("circular_file", chain[len(chain)-1]).
			Hint(fmt.Sprintf("Remove the circular reference in %s\nRestructure your includes to avoid circular dependencies\nUse a different include strategy (e.g., separate common rules)\nConsider merging the circular files into a single file", chain[len(chain)-1])).
			Errorf("circular include detected")
	}
	l.visited[configKey] = true
	defer func() { l.visited[configKey] = false }()

	var config *Config
	var err error
	switch {
	case l.isURL(filename):
		config, err = l.loadRemoteConfig(ctx, filename)
	case isInclude:
		config, err = l.loadIncludeFile(ctx, filename)
	default:
		config, err = LoadConfig(filename)
	}
	if err != nil {
		return nil, oops.
			With("path", filename).
			With("filename", filename).
			With("operation", "loading main config").
			Wrapf(err, "loading main config")
	}

	var baseDir string
	if l.isURL(filename) {
		baseDir = filename
	} else {
		baseDir = filepath.Dir(filename)
	}
	
	// Resolve extends first (inheritance)
	if err := l.resolveExtends(ctx, config, baseDir); err != nil {
		return nil, err
	}
	
	// Then resolve includes (additive merging)
	if err := l.resolveIncludes(ctx, config, baseDir); err != nil {
		return nil, err
	}

	return config, nil
}

func (l *configLoader) resolveIncludes(ctx context.Context, config *Config, baseDir string) error {
	if len(config.Includes) == 0 {
		return nil
	}

	allRules, allSections, allAgents, err := l.collectIncludedContent(ctx, config, baseDir)
	if err != nil {
		return err
	}

	config.Rules = allRules
	config.Sections = allSections
	config.Agents = allAgents
	config.Includes = nil

	setDefaultPriorities(config)

	return nil
}

func (l *configLoader) resolveExtends(ctx context.Context, config *Config, baseDir string) error {
	if config.Extends == "" {
		return nil
	}

	extendsPath := l.resolvePath(config.Extends, baseDir)
	
	// Load base configuration
	baseConfig, err := l.loadConfigInternal(ctx, extendsPath, true)
	if err != nil {
		return oops.
			With("extends_path", config.Extends).
			With("resolved_path", extendsPath).
			With("operation", "loading extended config").
			Hint("Check if the extended configuration file exists and is accessible\nVerify the extends path is correct\nFor remote URLs, ensure network connectivity").
			Wrapf(err, "loading extended config")
	}

	// Apply inheritance: base config is inherited, child config overrides
	extendedConfig := l.applyExtends(baseConfig, config)
	
	// Replace current config with extended version
	*config = *extendedConfig
	config.Extends = "" // Clear extends to prevent re-processing

	return nil
}

func (l *configLoader) applyExtends(base, child *Config) *Config {
	// Start with base config
	result := &Config{
		Metadata: base.Metadata,
		Extends:  "",
		Includes: nil, // Don't copy base includes - they were already resolved
		Outputs:  base.Outputs,
		Rules:    base.Rules,
		Sections: base.Sections,
		Agents:   base.Agents,
	}

	// Override with child config values (child takes precedence)
	if child.Metadata.Name != "" {
		result.Metadata.Name = child.Metadata.Name
	}
	if child.Metadata.Version != "" {
		result.Metadata.Version = child.Metadata.Version
	}
	if child.Metadata.Description != "" {
		result.Metadata.Description = child.Metadata.Description
	}

	// Only use child includes (base includes were already resolved)
	result.Includes = child.Includes
	
	// Append child outputs to base outputs
	result.Outputs = append(result.Outputs, child.Outputs...)

	// For rules, sections, and agents, merge using existing merge logic
	result.Rules = mergeRules(result.Rules, child.Rules)
	result.Sections = mergeSections(result.Sections, child.Sections)
	result.Agents = mergeAgents(result.Agents, child.Agents)

	return result
}

// loadIncludeFile loads an include file, handling cases where the include itself has extends or includes
func (l *configLoader) loadIncludeFile(ctx context.Context, filename string) (*Config, error) {
	// First, try to load as partial config to check if it has extends/includes
	partialConfig, err := LoadPartialConfig(filename)
	if err != nil {
		return nil, err
	}
	
	// If include file has extends or includes, process it fully
	if partialConfig.Extends != "" || len(partialConfig.Includes) > 0 {
		// Use LoadConfigWithIncludes to process the extends/includes
		return LoadConfigWithIncludes(ctx, filename)
	}
	
	// Simple include file, return the partial config
	return partialConfig, nil
}

func (l *configLoader) resolvePath(includePath, baseDir string) string {
	if l.isURL(includePath) {
		return includePath
	}

	if filepath.IsAbs(includePath) {
		return includePath
	}

	if l.isURL(baseDir) {
		baseURL, err := url.Parse(baseDir)
		if err != nil {
			return includePath
		}
		relativeURL, err := url.Parse(includePath)
		if err != nil {
			return includePath
		}
		resolved := baseURL.ResolveReference(relativeURL)
		return resolved.String()
	}

	return filepath.Join(baseDir, includePath)
}

func (*configLoader) isURL(path string) bool {
	parsedURL, err := url.Parse(path)
	if err != nil {
		return false
	}
	return parsedURL.Scheme == "http" || parsedURL.Scheme == "https"
}

func (l *configLoader) loadRemoteConfig(ctx context.Context, configURL string) (*Config, error) {
	content, err := l.remoteClient.Fetch(ctx, configURL)
	if err != nil {
		errorMsg := err.Error()
		hintMsg := fmt.Sprintf("Check if the URL is accessible: %s\nVerify your network connection\nEnsure the remote server is responding correctly\nCheck if authentication is required for this resource", configURL)

		if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded") {
			hintMsg += "\nThe request timed out - try again or check server response time"
		}
		if strings.Contains(errorMsg, "connection refused") {
			hintMsg += "\nConnection refused - check if the server is running"
		}
		if strings.Contains(errorMsg, "no such host") {
			hintMsg += "\nDNS resolution failed - check the hostname"
		}

		return nil, oops.
			With("path", configURL).
			With("url", configURL).
			Hint(hintMsg).
			Wrapf(err, "fetch remote config")
	}

	var config Config
	if err := yaml.Unmarshal(content, &config); err != nil {
		hintMsg := "Check the YAML syntax in the remote file\nEnsure the remote file follows ai-rulez configuration format\nVerify the remote file content type is text/yaml or application/yaml"

		return nil, oops.
			With("path", configURL).
			With("url", configURL).
			With("parse_error", err.Error()).
			Hint(hintMsg).
			Wrapf(err, "parse remote config")
	}

	return &config, nil
}

func ValidateIncludes(config *Config, baseDir string) error {
	loader := &configLoader{
		visited:      make(map[string]bool),
		baseDir:      baseDir,
		remoteClient: remote.NewClient(nil),
	}

	for _, includePath := range config.Includes {
		resolvedPath := loader.resolvePath(includePath, baseDir)

		if !loader.isURL(resolvedPath) {
			if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
				return oops.
					With("path", resolvedPath).
					With("include_path", includePath).
					With("base_dir", baseDir).
					Hint(fmt.Sprintf("Check if the include file exists: %s\nVerify the path is correct relative to %s", resolvedPath, baseDir)).
					Errorf("include file not found: %s", includePath)
			}
		}

		var err error
		if loader.isURL(resolvedPath) {
			_, err = loader.loadRemoteConfig(context.Background(), resolvedPath)
		} else {
			_, err = LoadPartialConfig(resolvedPath)
		}
		if err != nil {
			return oops.
				With("path", resolvedPath).
				With("filename", resolvedPath).
				With("include_path", includePath).
				With("operation", "validate include syntax").
				Wrapf(err, "validate include syntax")
		}
	}

	return nil
}

func ValidateOutputs(outputs []Output) error {
	if len(outputs) == 0 {
		return oops.
			With("field", "outputs").
			With("context", "configuration").
			Hint("Add the required field 'outputs' to your configuration\nAdd at least one output file in the 'outputs' section\nExample: outputs: [{file: 'CLAUDE.md', template: 'default'}]").
			Errorf("required field 'outputs' is missing")
	}

	for i, output := range outputs {
		if output.Path == "" {
			return oops.
				With("field", fmt.Sprintf("outputs[%d].path", i)).
				With("context", "output configuration").
				With("output_index", i).
				Hint(fmt.Sprintf("Add the required field 'outputs[%d].path' to your configuration\nSpecify a path for output[%d]\nExample: path: 'CLAUDE.md'", i, i)).
				Errorf("required field 'outputs[%d].path' is missing", i)
		}
	}

	return nil
}

func (l *configLoader) loadLocalOverrides(ctx context.Context, config *Config, filename string) error {
	localConfig, err := l.loadConfig(ctx, filename)
	if err != nil {
		return err
	}

	// Merge the configs using our new ID-based merging logic
	merged := MergeConfigs(config, localConfig)

	// Update the original config in place
	config.Rules = merged.Rules
	config.Sections = merged.Sections
	config.Agents = merged.Agents

	// Local configs can also add outputs and includes
	if len(localConfig.Outputs) > 0 {
		config.Outputs = append(config.Outputs, localConfig.Outputs...)
	}
	if len(localConfig.Includes) > 0 {
		config.Includes = append(config.Includes, localConfig.Includes...)
	}

	return nil
}
