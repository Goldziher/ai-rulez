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
		remoteClient: remote.NewClient(nil),
	}

	config, err := loader.loadConfig(absPath)
	if err != nil {
		return nil, err
	}

	baseDir := filepath.Dir(absPath)

	configBaseName := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	localConfigPath := filepath.Join(baseDir, configBaseName+".local.yaml")
	if _, err := os.Stat(localConfigPath); err == nil {
		if err := loader.loadLocalOverrides(config, localConfigPath); err != nil {
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

func (l *configLoader) loadConfig(filename string) (*Config, error) {
	configKey := filename
	if l.isURL(filename) {
		configKey = filename
	} else {
		absPath, err := filepath.Abs(filename)
		if err != nil {
			return nil, errors.FileRead(filename, err).
				WithContext("operation", "resolve absolute path").
				WithSuggestion("Check if the file path is valid and accessible")
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
		return nil, errors.CircularInclude(chain)
	}
	l.visited[configKey] = true
	defer func() { l.visited[configKey] = false }()

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

	var baseDir string
	if l.isURL(filename) {
		baseDir = filename
	} else {
		baseDir = filepath.Dir(filename)
	}
	if err := l.resolveIncludes(config, baseDir); err != nil {
		return nil, err
	}

	return config, nil
}

func (l *configLoader) resolveIncludes(config *Config, baseDir string) error {
	if len(config.Includes) == 0 {
		return nil
	}

	var allRules []Rule
	var allSections []Section
	var allAgents []Agent
	allRules = append(allRules, config.Rules...)
	allSections = append(allSections, config.Sections...)
	allAgents = append(allAgents, config.Agents...)

	for _, includePath := range config.Includes {
		resolvedPath := l.resolvePath(includePath, baseDir)

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
			return err
		}

		allRules = append(allRules, includedConfig.Rules...)
		allSections = append(allSections, includedConfig.Sections...)
		allAgents = append(allAgents, includedConfig.Agents...)
	}

	config.Rules = MergeRules(allRules)
	config.Sections = MergeSections(allSections)
	config.Agents = MergeAgents(allAgents)
	config.Includes = nil

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

	return nil
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

func (l *configLoader) loadRemoteConfig(configURL string) (*Config, error) {
	ctx := context.Background()

	content, err := l.remoteClient.Fetch(ctx, configURL)
	if err != nil {
		return nil, errors.RemoteConfigFetch(configURL, err)
	}

	var config Config
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, errors.RemoteConfigParse(configURL, err)
	}

	return &config, nil
}

func MergeRules(ruleSets ...[]Rule) []Rule {
	ruleMap := make(map[string]Rule)
	var order []string

	for _, rules := range ruleSets {
		for _, rule := range rules {
			key := rule.Name
			if rule.ID != "" {
				key = rule.ID
			}

			if _, exists := ruleMap[key]; !exists {
				order = append(order, key)
			}
			ruleMap[key] = rule
		}
	}

	result := make([]Rule, 0, len(order))
	for _, key := range order {
		result = append(result, ruleMap[key])
	}

	return result
}

func MergeSections(sectionSets ...[]Section) []Section {
	sectionMap := make(map[string]Section)
	var order []string

	for _, sections := range sectionSets {
		for _, section := range sections {
			key := section.Title
			if section.ID != "" {
				key = section.ID
			}

			if _, exists := sectionMap[key]; !exists {
				order = append(order, key)
			}
			sectionMap[key] = section
		}
	}

	result := make([]Section, 0, len(order))
	for _, key := range order {
		result = append(result, sectionMap[key])
	}

	return result
}

func MergeAgents(agentSets ...[]Agent) []Agent {
	agentMap := make(map[string]Agent)
	var order []string

	for _, agents := range agentSets {
		for i := range agents {
			key := agents[i].Name
			if agents[i].ID != "" {
				key = agents[i].ID
			}

			if _, exists := agentMap[key]; !exists {
				order = append(order, key)
			}
			agentMap[key] = agents[i]
		}
	}

	result := make([]Agent, 0, len(order))
	for _, key := range order {
		result = append(result, agentMap[key])
	}

	return result
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
				return errors.New(errors.ErrorTypeConfigNotFound, "validate include",
					fmt.Errorf("include file not found: %s", includePath)).
					WithPath(resolvedPath).
					WithContext("include_path", includePath).
					WithContext("base_dir", baseDir).
					WithSuggestion("Check if the include file exists: %s", resolvedPath).
					WithSuggestion("Verify the path is correct relative to %s", baseDir)
			}
		}

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

func (l *configLoader) loadLocalOverrides(config *Config, filename string) error {
	localConfig, err := l.loadConfig(filename)
	if err != nil {
		return err
	}

	config.Rules = MergeRules(config.Rules, localConfig.Rules)
	config.Sections = MergeSections(config.Sections, localConfig.Sections)
	config.Agents = MergeAgents(config.Agents, localConfig.Agents)

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
